// SPDX-License-Identifier: AGPL-3.0-only

package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/drilonrecica/binnacle/internal/auth"
	"github.com/drilonrecica/binnacle/internal/metrics"
	"github.com/drilonrecica/binnacle/internal/storage"
)

func TestDisabledFeatureRoutesAreNotRegistered(t *testing.T) {
	proxy, err := auth.NewProxyAuthenticator(auth.ProxyAuthConfig{Mode: auth.LocalAuth, IdentityHeader: "X-Forwarded-User"}, auth.TrustedProxies{})
	if err != nil {
		t.Fatal(err)
	}
	server := New()
	server.EnableAuthMethods(proxy, false)
	for _, path := range []string{
		"/api/v1/auth/external-session",
		"/api/v1/auth/mfa",
		"/api/v1/auth/mfa/enroll",
		"/api/v1/auth/mfa/confirm",
		"/api/v1/auth/mfa/disable",
		"/api/v1/api-tokens",
		"/api/v1/api-tokens/tok_1",
		"/api/v1/exports/metrics.csv",
		"/api/v1/exports/events.json",
		"/api/v1/exports/incidents.json",
		"/api/v1/exports/resources.json",
	} {
		request := httptest.NewRequest(http.MethodGet, "http://binnacle.test"+path, nil)
		response := httptest.NewRecorder()
		server.Handler().ServeHTTP(response, request)
		if response.Code != http.StatusNotFound {
			t.Errorf("%s status=%d body=%s", path, response.Code, response.Body.String())
		}
	}

	request := httptest.NewRequest(http.MethodGet, "http://binnacle.test/api/v1/auth/methods", nil)
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK || response.Body.String() == "" {
		t.Fatalf("methods status=%d body=%s", response.Code, response.Body.String())
	}
	if body := response.Body.String(); body != "{\"local\":true,\"mfaAvailable\":false,\"mode\":\"local\",\"proxy\":false,\"proxyAvailable\":false}\n" {
		t.Fatalf("methods body=%s", body)
	}
}

func TestSessionOnlyReadsRejectExistingBearerToken(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	manager := storage.New(filepath.Join(dir, "binnacle.db"), filepath.Join(dir, "run"))
	if err := manager.Open(ctx); err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	credentials := auth.NewCredentials(manager.DB())
	user, err := credentials.CreateAdmin(ctx, "admin", "correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	repository := auth.NewAPITokenRepository(manager.DB())
	_, plaintext, err := repository.Create(ctx, user.ID, "existing", []auth.APIScope{auth.ScopeServerRead}, nil)
	if err != nil {
		t.Fatal(err)
	}
	sessions := auth.NewSessions(manager.DB(), auth.SessionConfig{IdleTimeout: time.Hour, AbsoluteLifetime: 24 * time.Hour})
	server := New()
	server.EnableCurrent(metrics.NewEngine(1), sessions)
	request := httptest.NewRequest(http.MethodGet, "http://binnacle.test/api/v1/server", nil)
	request.Header.Set("Authorization", "Bearer "+plaintext)
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}
