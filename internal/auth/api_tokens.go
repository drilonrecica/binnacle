// SPDX-License-Identifier: AGPL-3.0-only
package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

type APIScope string

const (
	ScopeServerRead    APIScope = "server:read"
	ScopeResourcesRead APIScope = "resources:read"
	ScopeMetricsRead   APIScope = "metrics:read"
	ScopeEventsRead    APIScope = "events:read"
	ScopeIncidentsRead APIScope = "incidents:read"
)

var validScopes = map[APIScope]bool{ScopeServerRead: true, ScopeResourcesRead: true, ScopeMetricsRead: true, ScopeEventsRead: true, ScopeIncidentsRead: true}
var (
	ErrAPITokenInvalid      = errors.New("API token is invalid")
	ErrAPIScopeInsufficient = errors.New("API token scope is insufficient")
)

type APIToken struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	Prefix     string     `json:"prefix"`
	Scopes     []APIScope `json:"scopes"`
	CreatedAt  time.Time  `json:"createdAt"`
	ExpiresAt  *time.Time `json:"expiresAt,omitempty"`
	LastUsedAt *time.Time `json:"lastUsedAt,omitempty"`
	RevokedAt  *time.Time `json:"revokedAt,omitempty"`
}
type APITokenRepository struct {
	db  *sql.DB
	now func() time.Time
}

func NewAPITokenRepository(db *sql.DB) *APITokenRepository {
	return &APITokenRepository{db: db, now: func() time.Time { return time.Now().UTC() }}
}
func (r *APITokenRepository) SetDB(db *sql.DB) { r.db = db }
func (r *APITokenRepository) Create(ctx context.Context, userID, name string, scopes []APIScope, expiresAt *time.Time) (APIToken, string, error) {
	name = strings.TrimSpace(name)
	if len(name) < 1 || len(name) > 64 {
		return APIToken{}, "", errors.New("token name must be 1 to 64 characters")
	}
	scopes, err := normalizeScopes(scopes)
	if err != nil {
		return APIToken{}, "", err
	}
	now := r.now()
	if expiresAt != nil {
		value := expiresAt.UTC()
		if value.Before(now.Add(time.Hour)) || value.After(now.Add(365*24*time.Hour)) {
			return APIToken{}, "", errors.New("token expiry must be between one hour and one year")
		}
		expiresAt = &value
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return APIToken{}, "", err
	}
	defer tx.Rollback()
	var active int
	if err = tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM api_tokens WHERE user_id=? AND revoked_at IS NULL AND (expires_at IS NULL OR expires_at>?)", userID, now.UnixMilli()).Scan(&active); err != nil {
		return APIToken{}, "", err
	}
	if active >= 32 {
		return APIToken{}, "", errors.New("active API token limit reached")
	}
	idBytes, secret := make([]byte, 8), make([]byte, 32)
	if _, err = rand.Read(idBytes); err != nil {
		return APIToken{}, "", err
	}
	if _, err = rand.Read(secret); err != nil {
		return APIToken{}, "", err
	}
	id := "tok_" + hex.EncodeToString(idBytes)
	plaintext := "bnk_" + hex.EncodeToString(idBytes) + "_" + base64.RawURLEncoding.EncodeToString(secret)
	prefix := plaintext[:12]
	hash := apiTokenHash(plaintext)
	raw, _ := json.Marshal(scopes)
	var expiry any
	if expiresAt != nil {
		expiry = expiresAt.UnixMilli()
	}
	_, err = tx.ExecContext(ctx, "INSERT INTO api_tokens(id,user_id,name,prefix,token_hash,scopes_json,created_at,expires_at) VALUES(?,?,?,?,?,?,?,?)", id, userID, name, prefix, hash, string(raw), now.UnixMilli(), expiry)
	if err != nil {
		return APIToken{}, "", err
	}
	if err = tx.Commit(); err != nil {
		return APIToken{}, "", err
	}
	return APIToken{ID: id, Name: name, Prefix: prefix, Scopes: scopes, CreatedAt: now, ExpiresAt: expiresAt}, plaintext, nil
}
func (r *APITokenRepository) List(ctx context.Context, userID string) ([]APIToken, error) {
	rows, err := r.db.QueryContext(ctx, "SELECT id,name,prefix,scopes_json,created_at,expires_at,last_used_at,revoked_at FROM api_tokens WHERE user_id=? ORDER BY created_at DESC", userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	values := []APIToken{}
	for rows.Next() {
		var value APIToken
		var raw string
		var created int64
		var expiry, last, revoked sql.NullInt64
		if err = rows.Scan(&value.ID, &value.Name, &value.Prefix, &raw, &created, &expiry, &last, &revoked); err != nil {
			return nil, err
		}
		if json.Unmarshal([]byte(raw), &value.Scopes) != nil {
			return nil, errors.New("invalid stored API token scopes")
		}
		value.CreatedAt = time.UnixMilli(created).UTC()
		value.ExpiresAt = timePtr(expiry)
		value.LastUsedAt = timePtr(last)
		value.RevokedAt = timePtr(revoked)
		values = append(values, value)
	}
	return values, rows.Err()
}
func (r *APITokenRepository) Revoke(ctx context.Context, userID, id string) error {
	result, err := r.db.ExecContext(ctx, "UPDATE api_tokens SET revoked_at=COALESCE(revoked_at,?) WHERE id=? AND user_id=?", r.now().UnixMilli(), id, userID)
	if err != nil {
		return err
	}
	affected, _ := result.RowsAffected()
	if affected != 1 {
		return sql.ErrNoRows
	}
	return nil
}
func (r *APITokenRepository) Authenticate(ctx context.Context, plaintext string, required APIScope) error {
	if r == nil || r.db == nil || !strings.HasPrefix(plaintext, "bnk_") || len(plaintext) > 128 {
		return ErrAPITokenInvalid
	}
	var scopesRaw string
	var expires, revoked, last sql.NullInt64
	err := r.db.QueryRowContext(ctx, "SELECT scopes_json,expires_at,revoked_at,last_used_at FROM api_tokens WHERE token_hash=?", apiTokenHash(plaintext)).Scan(&scopesRaw, &expires, &revoked, &last)
	if err != nil || revoked.Valid || (expires.Valid && expires.Int64 <= r.now().UnixMilli()) {
		return ErrAPITokenInvalid
	}
	var scopes []APIScope
	if json.Unmarshal([]byte(scopesRaw), &scopes) != nil {
		return ErrAPITokenInvalid
	}
	allowed := false
	for _, scope := range scopes {
		allowed = allowed || scope == required
	}
	if !allowed {
		return ErrAPIScopeInsufficient
	}
	now := r.now()
	if !last.Valid || last.Int64 < now.Add(-time.Hour).UnixMilli() {
		_, _ = r.db.ExecContext(ctx, "UPDATE api_tokens SET last_used_at=? WHERE token_hash=? AND (last_used_at IS NULL OR last_used_at<?)", now.UnixMilli(), apiTokenHash(plaintext), now.Add(-time.Hour).UnixMilli())
	}
	return nil
}
func normalizeScopes(scopes []APIScope) ([]APIScope, error) {
	if len(scopes) < 1 || len(scopes) > len(validScopes) {
		return nil, errors.New("one to five scopes are required")
	}
	seen := map[APIScope]bool{}
	for _, scope := range scopes {
		if !validScopes[scope] || seen[scope] {
			return nil, fmt.Errorf("invalid API token scope %q", scope)
		}
		seen[scope] = true
	}
	result := append([]APIScope(nil), scopes...)
	sort.Slice(result, func(i, j int) bool { return result[i] < result[j] })
	return result, nil
}
func apiTokenHash(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}
func timePtr(value sql.NullInt64) *time.Time {
	if !value.Valid {
		return nil
	}
	at := time.UnixMilli(value.Int64).UTC()
	return &at
}
