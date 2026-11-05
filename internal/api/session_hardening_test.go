// SPDX-License-Identifier: AGPL-3.0-only
package api

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/drilonrecica/binnacle/internal/auth"
	"github.com/drilonrecica/binnacle/internal/diagnostics"
	"github.com/drilonrecica/binnacle/internal/dockerapi"
	"github.com/drilonrecica/binnacle/internal/metrics"
	"github.com/drilonrecica/binnacle/internal/storage"
)

func openSessions(t *testing.T) (*storage.Manager, *auth.Sessions) {
	t.Helper()
	ctx := context.Background()
	dir := t.TempDir()
	manager := storage.New(filepath.Join(dir, "binnacle.db"), filepath.Join(dir, "run"))
	if err := manager.Open(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { manager.Close() })
	return manager, auth.NewSessions(manager.DB(), auth.SessionConfig{IdleTimeout: time.Hour, AbsoluteLifetime: 24 * time.Hour})
}

// serveStream runs a streaming handler in the background and returns a
// function that waits for it to finish, or fails the test after a timeout.
func serveStream(t *testing.T, handler http.Handler, request *http.Request) (*httptest.ResponseRecorder, func(within time.Duration) bool) {
	t.Helper()
	recorder := httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		defer close(done)
		handler.ServeHTTP(recorder, request)
	}()
	return recorder, func(within time.Duration) bool {
		select {
		case <-done:
			return true
		case <-time.After(within):
			return false
		}
	}
}

func shortHeartbeat(t *testing.T) {
	t.Helper()
	previous := streamHeartbeat
	streamHeartbeat = 10 * time.Millisecond
	t.Cleanup(func() { streamHeartbeat = previous })
}

// SEC-12: the live stream re-checks the session on every heartbeat and
// closes once it is revoked, instead of running until the client disconnects.
func TestLiveStreamClosesWhenSessionIsRevoked(t *testing.T) {
	shortHeartbeat(t)
	ctx := context.Background()
	_, sessions := openSessions(t)
	token, _, err := sessions.Issue(ctx, "usr_test")
	if err != nil {
		t.Fatal(err)
	}
	server := New()
	server.EnableLive(metrics.NewEngine(8), sessions, auth.NewProtection(16, auth.TrustedProxies{}))
	streamCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	request := httptest.NewRequest(http.MethodGet, "http://binnacle.test/api/v1/live", nil).WithContext(streamCtx)
	request.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: token})
	recorder, finished := serveStream(t, server.Handler(), request)
	if finished(100 * time.Millisecond) {
		t.Fatalf("stream ended while the session was valid: status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if err = sessions.Revoke(ctx, token); err != nil {
		t.Fatal(err)
	}
	if !finished(2 * time.Second) {
		t.Fatal("stream kept running after the session was revoked")
	}
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), ": heartbeat") {
		t.Fatalf("status=%d body=%q", recorder.Code, recorder.Body.String())
	}
}

// followLogs blocks like a Docker follow until the context ends, after one
// initial line, so the handler's own cancellation is what stops it.
type followLogs struct{}

func (followLogs) ReadLogs(ctx context.Context, _ string, options dockerapi.LogOptions, emit func(string, string) error) error {
	if err := emit("stdout", "2026-07-11T12:00:00Z service started"); err != nil {
		return err
	}
	if !options.Follow {
		return nil
	}
	<-ctx.Done()
	return ctx.Err()
}

func TestLogFollowClosesWhenSessionIsRevoked(t *testing.T) {
	shortHeartbeat(t)
	ctx := context.Background()
	_, sessions := openSessions(t)
	token, _, err := sessions.Issue(ctx, "usr_test")
	if err != nil {
		t.Fatal(err)
	}
	service, err := diagnostics.NewLogService(followLogs{}, 5000, 1<<20, nil)
	if err != nil {
		t.Fatal(err)
	}
	server := New()
	server.EnableLogs(service, metrics.NewEngine(8), sessions)
	streamCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	request := httptest.NewRequest(http.MethodGet, "http://binnacle.test/api/v1/logs?container=web&follow=true", nil).WithContext(streamCtx)
	request.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: token})
	recorder, finished := serveStream(t, server.Handler(), request)
	if finished(100 * time.Millisecond) {
		t.Fatalf("follow ended while the session was valid: status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if err = sessions.Revoke(ctx, token); err != nil {
		t.Fatal(err)
	}
	if !finished(2 * time.Second) {
		t.Fatal("follow kept running after the session was revoked")
	}
	body := recorder.Body.String()
	if recorder.Code != http.StatusOK || !strings.Contains(body, "event: log") || !strings.Contains(body, ": heartbeat") || strings.Contains(body, "event: end") {
		t.Fatalf("status=%d body=%q", recorder.Code, body)
	}
}

func TestPasswordChangeRequiresCurrentPasswordAndRevokesOtherSessions(t *testing.T) {
	ctx := context.Background()
	manager, sessions := openSessions(t)
	credentials := auth.NewCredentials(manager.DB())
	user, err := credentials.CreateAdmin(ctx, "admin", "correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	server := New()
	server.EnableAuth(credentials, sessions, auth.NewProtection(128, auth.TrustedProxies{}))
	current, csrf, _, err := sessions.IssueWithCSRF(ctx, user.ID)
	if err != nil {
		t.Fatal(err)
	}
	other, _, err := sessions.Issue(ctx, user.ID)
	if err != nil {
		t.Fatal(err)
	}
	post := func(body string, withCSRF bool) *httptest.ResponseRecorder {
		request := httptest.NewRequest(http.MethodPost, "http://binnacle.test/api/v1/auth/password", bytes.NewBufferString(body))
		request.RemoteAddr = "192.0.2.10:1234"
		request.Header.Set("Origin", "http://binnacle.test")
		request.Header.Set("Content-Type", "application/json")
		request.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: current})
		if withCSRF {
			request.Header.Set("X-CSRF-Token", csrf)
			request.AddCookie(&http.Cookie{Name: auth.CSRFCookieName, Value: csrf})
		}
		response := httptest.NewRecorder()
		server.Handler().ServeHTTP(response, request)
		return response
	}
	if response := post(`{"currentPassword":"correct horse battery staple","newPassword":"a brand new passphrase"}`, false); response.Code != http.StatusForbidden {
		t.Fatalf("without CSRF status=%d body=%s", response.Code, response.Body.String())
	}
	if response := post(`{"currentPassword":"wrong password value","newPassword":"a brand new passphrase"}`, true); response.Code != http.StatusUnauthorized {
		t.Fatalf("wrong current status=%d body=%s", response.Code, response.Body.String())
	}
	if response := post(`{"currentPassword":"correct horse battery staple","newPassword":"short"}`, true); response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "password_policy") {
		t.Fatalf("weak password status=%d body=%s", response.Code, response.Body.String())
	}
	if _, err = sessions.Authenticate(ctx, other); err != nil {
		t.Fatalf("other session revoked before a successful change: %v", err)
	}
	if response := post(`{"currentPassword":"correct horse battery staple","newPassword":"a brand new passphrase"}`, true); response.Code != http.StatusNoContent {
		t.Fatalf("change status=%d body=%s", response.Code, response.Body.String())
	}
	if _, err = credentials.Authenticate(ctx, "admin", "a brand new passphrase"); err != nil {
		t.Fatalf("new password rejected: %v", err)
	}
	if _, err = sessions.Authenticate(ctx, other); err == nil {
		t.Fatal("other session survived the password change")
	}
	if _, err = sessions.Authenticate(ctx, current); err != nil {
		t.Fatalf("current session revoked by the password change: %v", err)
	}
}
