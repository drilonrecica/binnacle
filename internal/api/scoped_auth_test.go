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
	"github.com/drilonrecica/binnacle/internal/storage"
)

func TestScopedAuthorizationAndInvalidBearerIsolation(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	manager := storage.New(filepath.Join(dir, "db"), filepath.Join(dir, "run"))
	if err := manager.Open(ctx); err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	credentials := auth.NewCredentials(manager.DB())
	user, _ := credentials.CreateAdmin(ctx, "admin", "correct horse battery staple")
	sessions := auth.NewSessions(manager.DB(), auth.SessionConfig{IdleTimeout: time.Hour, AbsoluteLifetime: time.Hour})
	sessionToken, _, err := sessions.Issue(ctx, user.ID)
	if err != nil {
		t.Fatal(err)
	}
	tokens := auth.NewAPITokenRepository(manager.DB())
	_, metricsToken, err := tokens.Create(ctx, user.ID, "metrics", []auth.APIScope{auth.ScopeMetricsRead}, nil)
	if err != nil {
		t.Fatal(err)
	}
	authorizer := ScopedAuthorizer{Sessions: sessions, Tokens: tokens, Scope: auth.ScopeEventsRead}
	request := httptest.NewRequest(http.MethodGet, "http://binnacle.test/api/v1/events", nil)
	request.Header.Set("Authorization", "Bearer "+metricsToken)
	decision := authorizer.Decision(request)
	if decision.Status != 403 || decision.Allowed {
		t.Fatalf("wrong-scope decision=%+v", decision)
	}
	request = httptest.NewRequest(http.MethodGet, "http://binnacle.test/api/v1/events", nil)
	request.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: sessionToken})
	request.Header.Set("Authorization", "Bearer invalid")
	decision = authorizer.Decision(request)
	if decision.Status != 401 || decision.Allowed {
		t.Fatalf("invalid bearer fell back to cookie: %+v", decision)
	}
	request.Header.Del("Authorization")
	if decision = authorizer.Decision(request); !decision.Allowed {
		t.Fatalf("browser session rejected: %+v", decision)
	}
}
