// SPDX-License-Identifier: AGPL-3.0-only
package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/drilonrecica/binnacle/internal/auth"
	"github.com/drilonrecica/binnacle/internal/storage"
	"github.com/drilonrecica/binnacle/internal/sweep"
)

// fixedProber answers every domain with the same healthy result.
type fixedProber struct{ status int }

func (p fixedProber) Probe(_ context.Context, d sweep.Domain) sweep.Result {
	days := 60
	return sweep.Result{DomainID: d.ID, HTTPStatus: p.status, CertDaysLeft: &days, NSPresent: true, Resolves: true, SPF: "v=spf1 -all", DMARCPolicy: "reject", HSTS: true, CSP: true}
}

type sweepAPIFixture struct {
	server        *Server
	manager       *storage.Manager
	repo          *sweep.Repository
	session, csrf string
}

func newSweepAPIFixture(t *testing.T) *sweepAPIFixture {
	t.Helper()
	ctx := context.Background()
	dir := t.TempDir()
	manager := storage.New(filepath.Join(dir, "binnacle.db"), filepath.Join(dir, "run"))
	if err := manager.Open(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = manager.Close() })
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
	repo := sweep.NewRepository(manager.DB())
	scheduler := sweep.NewScheduler(repo, fixedProber{status: 200}, 1)
	server := New()
	server.EnableSweep(repo, scheduler, 2*time.Hour, sessions, sessions, auth.NewProtection(64, auth.TrustedProxies{}))
	return &sweepAPIFixture{server, manager, repo, session, csrf}
}

func (f *sweepAPIFixture) request(method, path, body string, csrf bool) *httptest.ResponseRecorder {
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

func decodeBody[T any](t *testing.T, response *httptest.ResponseRecorder) T {
	t.Helper()
	var v T
	if err := json.Unmarshal(response.Body.Bytes(), &v); err != nil {
		t.Fatalf("decode %s: %v", response.Body.String(), err)
	}
	return v
}

func TestSweepRoutesRequireAuthentication(t *testing.T) {
	server := New()
	server.EnableSweep(nil, nil, time.Hour, DemoAuthorizer(false), nil, auth.NewProtection(32, auth.TrustedProxies{}))
	for _, tt := range []struct{ method, path string }{{http.MethodGet, "/api/v1/sweep/domains"}, {http.MethodPost, "/api/v1/sweep/domains"}, {http.MethodGet, "/api/v1/sweep/domains/x"}, {http.MethodPatch, "/api/v1/sweep/domains/x"}, {http.MethodDelete, "/api/v1/sweep/domains/x"}, {http.MethodPost, "/api/v1/sweep/domains/x/run"}} {
		req := httptest.NewRequest(tt.method, "http://binnacle.test"+tt.path, nil)
		rec := httptest.NewRecorder()
		server.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("%s %s status=%d", tt.method, tt.path, rec.Code)
		}
	}
}

func TestSweepDomainLifecycle(t *testing.T) {
	f := newSweepAPIFixture(t)
	if response := f.request(http.MethodPost, "/api/v1/sweep/domains", `{"domain":"Example.COM."}`, false); response.Code != http.StatusForbidden {
		t.Fatalf("mutation without CSRF status=%d", response.Code)
	}
	response := f.request(http.MethodPost, "/api/v1/sweep/domains", `{"domain":"Example.COM."}`, true)
	if response.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", response.Code, response.Body.String())
	}
	created := decodeBody[sweep.Domain](t, response)
	if created.Domain != "example.com" || created.Interval != 2*time.Hour || !created.Enabled || created.ID == "" {
		t.Fatalf("created=%+v (want normalized domain, the configured default interval, enabled)", created)
	}
	if response = f.request(http.MethodPost, "/api/v1/sweep/domains", `{"domain":"example.com"}`, true); response.Code != http.StatusConflict {
		t.Fatalf("duplicate status=%d body=%s", response.Code, response.Body.String())
	}
	for _, body := range []string{`{"domain":"https://bad.example"}`, `{"domain":"ok.example","intervalSeconds":60}`, `{"domain":"ok.example","unknown":1}`, `{"domain":"localhost"}`} {
		if response = f.request(http.MethodPost, "/api/v1/sweep/domains", body, true); response.Code != http.StatusBadRequest {
			t.Fatalf("%s status=%d", body, response.Code)
		}
	}
	response = f.request(http.MethodPost, "/api/v1/sweep/domains", `{"domain":"shop.example.org","intervalSeconds":600,"enabled":false}`, true)
	if response.Code != http.StatusCreated {
		t.Fatalf("second create status=%d body=%s", response.Code, response.Body.String())
	}
	second := decodeBody[sweep.Domain](t, response)
	if second.Interval != 10*time.Minute || second.Enabled {
		t.Fatalf("second=%+v", second)
	}

	response = f.request(http.MethodGet, "/api/v1/sweep/domains", "", false)
	if response.Code != http.StatusOK {
		t.Fatalf("list status=%d", response.Code)
	}
	entries := decodeBody[[]sweep.Entry](t, response)
	if len(entries) != 2 || entries[0].Domain.Domain != "example.com" || entries[0].State != nil {
		t.Fatalf("entries=%+v", entries)
	}

	response = f.request(http.MethodPost, "/api/v1/sweep/domains/"+created.ID+"/run", "", true)
	if response.Code != http.StatusOK {
		t.Fatalf("run status=%d body=%s", response.Code, response.Body.String())
	}
	result := decodeBody[sweep.Result](t, response)
	if result.HTTPStatus != 200 || result.Fingerprint == "" {
		t.Fatalf("run result=%+v", result)
	}
	if response = f.request(http.MethodPost, "/api/v1/sweep/domains/"+second.ID+"/run", "", true); response.Code != http.StatusBadRequest {
		t.Fatalf("run of a disabled domain status=%d", response.Code)
	}
	if response = f.request(http.MethodPost, "/api/v1/sweep/domains/missing/run", "", true); response.Code != http.StatusNotFound {
		t.Fatalf("run of a missing domain status=%d", response.Code)
	}

	response = f.request(http.MethodGet, "/api/v1/sweep/domains/"+created.ID, "", false)
	if response.Code != http.StatusOK {
		t.Fatalf("get status=%d", response.Code)
	}
	entry := decodeBody[sweep.Entry](t, response)
	if entry.State == nil || entry.State.HTTPStatus != 200 || entry.State.CertDaysLeft == nil || *entry.State.CertDaysLeft != 60 || !entry.State.HSTS || entry.State.DMARCPolicy != "reject" {
		t.Fatalf("entry=%+v state=%+v", entry, entry.State)
	}
	if !entry.State.NextRunAt.Equal(entry.State.CheckedAt.Add(2 * time.Hour)) {
		t.Fatalf("next run=%s checked=%s", entry.State.NextRunAt, entry.State.CheckedAt)
	}

	response = f.request(http.MethodPatch, "/api/v1/sweep/domains/"+created.ID, `{"intervalSeconds":3600,"enabled":false}`, true)
	if response.Code != http.StatusOK {
		t.Fatalf("patch status=%d body=%s", response.Code, response.Body.String())
	}
	patched := decodeBody[sweep.Entry](t, response)
	if patched.Interval != time.Hour || patched.Enabled || patched.Domain.Domain != "example.com" || patched.State == nil {
		t.Fatalf("patched=%+v", patched)
	}
	if response = f.request(http.MethodPatch, "/api/v1/sweep/domains/"+created.ID, `{"domain":"shop.example.org"}`, true); response.Code != http.StatusConflict {
		t.Fatalf("rename onto an existing domain status=%d", response.Code)
	}
	if response = f.request(http.MethodPatch, "/api/v1/sweep/domains/missing", `{"enabled":true}`, true); response.Code != http.StatusNotFound {
		t.Fatalf("patch missing status=%d", response.Code)
	}

	if response = f.request(http.MethodDelete, "/api/v1/sweep/domains/"+created.ID, "", true); response.Code != http.StatusNoContent {
		t.Fatalf("delete status=%d", response.Code)
	}
	if response = f.request(http.MethodDelete, "/api/v1/sweep/domains/"+created.ID, "", true); response.Code != http.StatusNotFound {
		t.Fatalf("second delete status=%d", response.Code)
	}
	if response = f.request(http.MethodGet, "/api/v1/sweep/domains/"+created.ID, "", false); response.Code != http.StatusNotFound {
		t.Fatalf("get after delete status=%d", response.Code)
	}
	if response = f.request(http.MethodPut, "/api/v1/sweep/domains", "", true); response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("PUT status=%d", response.Code)
	}
}
