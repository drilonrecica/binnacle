// SPDX-License-Identifier: AGPL-3.0-only
package coolify

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net"
	"net/url"
	"strings"
	"sync"

	"github.com/drilonrecica/binnacle/internal/auth"
	"github.com/drilonrecica/binnacle/internal/metrics"
	"github.com/drilonrecica/binnacle/internal/outbound"
)

const coolifyTokenSecret = "coolify.api_token"

var (
	// ErrTokenRequired is returned when a request needs the API token typed again:
	// a connection test, or a save that moves the URL to another origin.
	ErrTokenRequired = errors.New("Coolify API token is required")
	// ErrTokenBoundToOrigin is returned when a save keeps the stored token but the
	// URL points at a different scheme, host or port than the one it was saved for.
	ErrTokenBoundToOrigin = errors.New("Coolify API token must be entered again when the URL moves to another host")
)

type Integration struct {
	db                       *sql.DB
	secrets                  *auth.SecretStore
	env                      ClientConfig
	environmentAuthoritative bool
	mu                       sync.RWMutex
	ctx                      context.Context
	cancel                   context.CancelFunc
	enricher                 *Enricher
	url                      string
	tokenConfigured          bool
}
type IntegrationStatus struct {
	Enabled                  bool       `json:"enabled"`
	URL                      string     `json:"url,omitempty"`
	TokenConfigured          bool       `json:"tokenConfigured"`
	EnvironmentAuthoritative bool       `json:"environmentAuthoritative"`
	Collector                SyncStatus `json:"collector"`
}

func NewIntegration(secrets *auth.SecretStore, env ClientConfig) *Integration {
	return &Integration{secrets: secrets, env: env, environmentAuthoritative: env.BaseURL != "" || env.Token != ""}
}
func (i *Integration) SetDB(db *sql.DB) { i.db = db }
func (i *Integration) Start(ctx context.Context) error {
	i.mu.Lock()
	i.ctx = ctx
	i.mu.Unlock()
	config, configured, err := i.effectiveConfig(ctx)
	if err != nil {
		return err
	}
	if configured {
		return i.activate(config)
	}
	return nil
}
func (i *Integration) Stop(context.Context) error {
	i.mu.Lock()
	if i.cancel != nil {
		i.cancel()
	}
	i.cancel = nil
	i.mu.Unlock()
	return nil
}
func (i *Integration) effectiveConfig(ctx context.Context) (ClientConfig, bool, error) {
	config := i.env
	if i.environmentAuthoritative {
		// The operator chose this URL at deployment time, so a private
		// Coolify address is an explicit decision rather than a UI input.
		config.AllowPrivateTargets = true
		return config, config.BaseURL != "" && config.Token != "", nil
	}
	var raw string
	err := i.db.QueryRowContext(ctx, "SELECT value_json FROM settings WHERE key='coolify.url'").Scan(&raw)
	if err == nil {
		_ = json.Unmarshal([]byte(raw), &config.BaseURL)
	} else if !errors.Is(err, sql.ErrNoRows) {
		return config, false, err
	}
	token, err := i.secrets.Get(ctx, coolifyTokenSecret)
	if err == nil {
		config.Token = string(token)
	} else if !errors.Is(err, auth.ErrSecretNotFound) && !errors.Is(err, auth.ErrMasterKeyMissing) {
		return config, false, err
	}
	return config, config.BaseURL != "" && config.Token != "", nil
}
func (i *Integration) activate(config ClientConfig) error {
	client, err := NewAPIClient(config)
	if err != nil {
		return err
	}
	enricher := NewEnricher(client)
	enricher.SetDB(i.db)
	i.mu.Lock()
	if i.cancel != nil {
		i.cancel()
	}
	ctx, cancel := context.WithCancel(i.ctx)
	i.cancel = cancel
	i.enricher = enricher
	i.url = config.BaseURL
	i.tokenConfigured = config.Token != ""
	i.mu.Unlock()
	return enricher.Start(ctx)
}

// clientConfig binds a URL and token to the deployment's transport settings.
func (i *Integration) clientConfig(url, token string) ClientConfig {
	return ClientConfig{BaseURL: url, Token: token, AllowInsecureHTTP: i.env.AllowInsecureHTTP, AllowPrivateTargets: i.env.AllowPrivateTargets, HTTPClient: i.env.HTTPClient}
}

// testConfig is the configuration a connection test runs with: the
// environment pair when it is authoritative, otherwise the submitted URL and
// token bound to the deployment's transport settings.
func (i *Integration) testConfig(url, token string) ClientConfig {
	if i.environmentAuthoritative {
		config := i.env
		config.AllowPrivateTargets = true
		return config
	}
	config := i.clientConfig(url, token)
	if config.BaseURL == "" {
		i.mu.RLock()
		config.BaseURL = i.url
		i.mu.RUnlock()
	}
	return config
}

// storedURL is the URL the stored token was saved with, or "" when none is saved.
func (i *Integration) storedURL(ctx context.Context) (string, error) {
	var raw, stored string
	err := i.db.QueryRowContext(ctx, "SELECT value_json FROM settings WHERE key='coolify.url'").Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	_ = json.Unmarshal([]byte(raw), &stored)
	return stored, nil
}

// origin reduces a URL to scheme://host[:port] with default ports dropped, so
// that path or trailing-slash edits keep the stored token while any change of
// scheme, host or port requires it again.
func origin(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Hostname() == "" {
		return ""
	}
	scheme := strings.ToLower(u.Scheme)
	host := strings.ToLower(u.Hostname())
	port := u.Port()
	if (scheme == "https" && port == "443") || (scheme == "http" && port == "80") {
		port = ""
	}
	if port != "" {
		host = net.JoinHostPort(host, port)
	}
	return scheme + "://" + host
}

func (i *Integration) Configure(ctx context.Context, url, token string) error {
	if i.environmentAuthoritative {
		return errors.New("environment configuration is authoritative")
	}
	if token != "" && !i.secrets.Available() {
		return auth.ErrMasterKeyMissing
	}
	config := i.clientConfig(url, token)
	if token == "" {
		stored, err := i.storedURL(ctx)
		if err != nil {
			return err
		}
		if stored == "" || origin(stored) == "" || origin(stored) != origin(url) {
			return ErrTokenBoundToOrigin
		}
		existing, err := i.secrets.Get(ctx, coolifyTokenSecret)
		if err != nil {
			return ErrTokenRequired
		}
		config.Token = string(existing)
	}
	if _, err := NewAPIClient(config); err != nil {
		return err
	}
	raw, _ := json.Marshal(url)
	tx, err := i.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, `INSERT INTO settings(key,value_json,updated_at,updated_by) VALUES('coolify.url',?,unixepoch('subsec')*1000,'admin') ON CONFLICT(key) DO UPDATE SET value_json=excluded.value_json,updated_at=excluded.updated_at,updated_by=excluded.updated_by`, string(raw))
	if err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	if token != "" {
		if err = i.secrets.Put(ctx, coolifyTokenSecret, []byte(token)); err != nil {
			return err
		}
	}
	return i.activate(config)
}

// Test connects with exactly the submitted token. The stored token is never
// used here, so a test can not carry it to a URL it was not saved for. With
// environment-authoritative configuration the environment pair is tested.
func (i *Integration) Test(ctx context.Context, url, token string) error {
	if !i.environmentAuthoritative && strings.TrimSpace(token) == "" {
		return ErrTokenRequired
	}
	client, err := NewAPIClient(i.testConfig(url, token))
	if err != nil {
		return err
	}
	if _, err = client.Metadata(ctx); errors.Is(err, outbound.ErrBlocked) {
		return ErrPrivateTarget
	}
	return err
}
func (i *Integration) Status() IntegrationStatus {
	i.mu.RLock()
	defer i.mu.RUnlock()
	status := IntegrationStatus{Enabled: i.enricher != nil, URL: i.url, TokenConfigured: i.tokenConfigured, EnvironmentAuthoritative: i.environmentAuthoritative, Collector: SyncStatus{State: "unknown"}}
	if i.enricher != nil {
		status.Collector = i.enricher.Status()
	}
	return status
}
func (i *Integration) Decorate(ctx context.Context, snapshot metrics.Snapshot) metrics.Snapshot {
	i.mu.RLock()
	enricher := i.enricher
	i.mu.RUnlock()
	if enricher == nil {
		return snapshot
	}
	return enricher.Decorate(ctx, snapshot)
}
