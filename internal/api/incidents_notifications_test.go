// SPDX-License-Identifier: AGPL-3.0-only

package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/drilonrecica/binnacle/internal/auth"
	"github.com/drilonrecica/binnacle/internal/notifications"
	"github.com/drilonrecica/binnacle/internal/outbound"
	"github.com/drilonrecica/binnacle/internal/storage"
)

type apiResolver struct{}

func (apiResolver) LookupNetIP(context.Context, string, string) ([]netip.Addr, error) {
	return []netip.Addr{netip.MustParseAddr("192.0.2.10")}, nil
}

type notificationAPIFixture struct {
	server        *Server
	manager       *storage.Manager
	repo          *notifications.Repository
	secrets       *auth.SecretStore
	session, csrf string
}

func newNotificationAPIFixture(t *testing.T, withKey bool) *notificationAPIFixture {
	t.Helper()
	ctx := context.Background()
	dir := t.TempDir()
	manager := storage.New(filepath.Join(dir, "binnacle.db"), filepath.Join(dir, "run"))
	if err := manager.Open(ctx); err != nil {
		t.Fatal(err)
	}
	credentials := auth.NewCredentials(manager.DB())
	user, err := credentials.CreateAdmin(ctx, "admin", "correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	sessions := auth.NewSessions(manager.DB(), auth.SessionConfig{IdleTimeout: time.Hour, AbsoluteLifetime: 24 * time.Hour})
	session, csrf, _, err := sessions.IssueWithCSRF(ctx, user.ID)
	if err != nil {
		t.Fatal(err)
	}
	key := ""
	if withKey {
		key = "0123456789abcdef0123456789abcdef"
	}
	secretStore, err := auth.NewSecretStore(manager.DB(), key)
	if err != nil {
		t.Fatal(err)
	}
	repo := notifications.NewRepository(manager.DB(), secretStore)
	worker := notifications.NewWorker(repo, notifications.Config{})
	worker.Policy = outbound.Policy{Resolver: apiResolver{}, Dial: func(context.Context, string, string) (net.Conn, error) { return nil, context.DeadlineExceeded }}
	server := New()
	server.EnableIncidentsNotifications(repo, worker, sessions, sessions, auth.NewProtection(64, auth.TrustedProxies{}))
	return &notificationAPIFixture{server, manager, repo, secretStore, session, csrf}
}
func (f *notificationAPIFixture) request(method, path, body string, csrf bool) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, "http://binnacle.test"+path, bytes.NewBufferString(body))
	request.RemoteAddr = "192.0.2.10:1234"
	request.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: f.session})
	request.Header.Set("Origin", "http://binnacle.test")
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	if csrf {
		request.Header.Set("X-CSRF-Token", f.csrf)
		request.AddCookie(&http.Cookie{Name: auth.CSRFCookieName, Value: f.csrf})
	}
	response := httptest.NewRecorder()
	f.server.Handler().ServeHTTP(response, request)
	return response
}

func TestNotificationAPIAuthenticationCSRFSecretsAndRetry(t *testing.T) {
	f := newNotificationAPIFixture(t, true)
	defer f.manager.Close()
	ctx := context.Background()
	unauthenticated := httptest.NewRequest(http.MethodGet, "http://binnacle.test/api/v1/incidents", nil)
	unauthenticated.RemoteAddr = "192.0.2.10:1234"
	unauthenticatedResponse := httptest.NewRecorder()
	f.server.Handler().ServeHTTP(unauthenticatedResponse, unauthenticated)
	if unauthenticatedResponse.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated status=%d", unauthenticatedResponse.Code)
	}
	now := time.Now().Unix()
	for _, id := range []string{"inc-1", "inc-2"} {
		_, err := f.manager.DB().Exec(`INSERT INTO incidents(id,group_key,status,severity,target_type,target_id,title,opened_at,updated_at,version)VALUES(?,?,'open','warning','resource',?,'Test incident',?,?,1)`, id, "resource:"+id, id, now, now)
		if err != nil {
			t.Fatal(err)
		}
	}
	response := f.request(http.MethodGet, "/api/v1/incidents?limit=1", "", false)
	if response.Code != 200 {
		t.Fatalf("incident list status=%d body=%s", response.Code, response.Body.String())
	}
	var incidents []notifications.Incident
	if json.Unmarshal(response.Body.Bytes(), &incidents) != nil || len(incidents) != 1 {
		t.Fatalf("pagination body=%s", response.Body.String())
	}
	response = f.request(http.MethodGet, "/api/v1/incidents/inc-1", "", false)
	if response.Code != 200 || !strings.Contains(response.Body.String(), `"title":"Test incident"`) {
		t.Fatalf("detail status=%d body=%s", response.Code, response.Body.String())
	}
	body := `{"name":"Operations","kind":"webhook","url":"https://webhook.test/hook","bearerToken":"top-secret","signingSecret":"sign-secret","enabled":true,"notifyResolved":true}`
	response = f.request(http.MethodPost, "/api/v1/notification-channels", body, false)
	if response.Code != http.StatusForbidden {
		t.Fatalf("missing CSRF status=%d", response.Code)
	}
	response = f.request(http.MethodPost, "/api/v1/notification-channels", body, true)
	if response.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", response.Code, response.Body.String())
	}
	if strings.Contains(response.Body.String(), "webhook.test") || strings.Contains(response.Body.String(), "top-secret") || !strings.Contains(response.Body.String(), `"secretConfigured":true`) {
		t.Fatalf("unsafe channel response=%s", response.Body.String())
	}
	var channel notifications.Channel
	if err := json.Unmarshal(response.Body.Bytes(), &channel); err != nil {
		t.Fatal(err)
	}
	response = f.request(http.MethodPatch, "/api/v1/notification-channels/"+channel.ID, `{"name":"Renamed","bearerToken":""}`, true)
	if response.Code != 200 {
		t.Fatalf("patch status=%d body=%s", response.Code, response.Body.String())
	}
	encrypted, err := f.secrets.Get(ctx, "notification.channel."+channel.ID)
	if err != nil {
		t.Fatal(err)
	}
	var stored notifications.ChannelSecrets
	if json.Unmarshal(encrypted, &stored) != nil || stored.URL != "https://webhook.test/hook" || stored.BearerToken != "" || stored.SigningSecret != "sign-secret" {
		t.Fatalf("secret patch semantics=%+v", stored)
	}
	response = f.request(http.MethodPost, "/api/v1/notification-channels/"+channel.ID+"/test", "", true)
	if response.Code != http.StatusAccepted {
		t.Fatalf("test status=%d body=%s", response.Code, response.Body.String())
	}
	var testBody map[string]string
	_ = json.Unmarshal(response.Body.Bytes(), &testBody)
	deliveryID := testBody["deliveryId"]
	var key string
	if err = f.manager.DB().QueryRow(`SELECT idempotency_key FROM notification_deliveries WHERE id=?`, deliveryID).Scan(&key); err != nil {
		t.Fatal(err)
	}
	_, _ = f.manager.DB().Exec(`UPDATE notification_deliveries SET status='permanent_failure',attempt_count=7,completed_at=? WHERE id=?`, now, deliveryID)
	response = f.request(http.MethodPost, "/api/v1/notification-deliveries/"+deliveryID+"/retry", "", true)
	if response.Code != http.StatusAccepted {
		t.Fatalf("retry status=%d body=%s", response.Code, response.Body.String())
	}
	var attempts int
	var afterKey string
	if err = f.manager.DB().QueryRow(`SELECT attempt_count,idempotency_key FROM notification_deliveries WHERE id=?`, deliveryID).Scan(&attempts, &afterKey); err != nil || attempts != 0 || afterKey != key {
		t.Fatalf("retry attempts=%d key=%s err=%v", attempts, afterKey, err)
	}
}

func TestNotificationAPIMasterKeyAndRateLimit(t *testing.T) {
	missing := newNotificationAPIFixture(t, false)
	defer missing.manager.Close()
	response := missing.request(http.MethodPost, "/api/v1/notification-channels", `{"name":"Hook","kind":"webhook","url":"https://webhook.test"}`, true)
	if response.Code != 400 || !strings.Contains(response.Body.String(), "master_key_missing") {
		t.Fatalf("missing key status=%d body=%s", response.Code, response.Body.String())
	}
	ctx := context.Background()
	dir := t.TempDir()
	manager := storage.New(filepath.Join(dir, "db"), filepath.Join(dir, "run"))
	if err := manager.Open(ctx); err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	secrets, _ := auth.NewSecretStore(manager.DB(), "")
	repo := notifications.NewRepository(manager.DB(), secrets)
	worker := notifications.NewWorker(repo, notifications.Config{})
	server := New()
	server.EnableIncidentsNotifications(repo, worker, DemoAuthorizer(true), nil, auth.NewProtection(4, auth.TrustedProxies{}))
	limited := false
	for i := 0; i < 121; i++ {
		request := httptest.NewRequest(http.MethodGet, "http://binnacle.test/api/v1/incidents", nil)
		request.RemoteAddr = "198.51.100.20:1234"
		rec := httptest.NewRecorder()
		server.Handler().ServeHTTP(rec, request)
		if rec.Code == 429 {
			limited = true
			break
		}
	}
	if !limited {
		t.Fatal("notification API did not enforce rate limit")
	}
}
