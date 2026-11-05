// SPDX-License-Identifier: AGPL-3.0-only
package auth

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/drilonrecica/binnacle/internal/storage"
)

func TestSessionLifecycleAndHashedPersistence(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	manager := storage.New(filepath.Join(dir, "binnacle.db"), filepath.Join(dir, "run"))
	if err := manager.Open(ctx); err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	now := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
	sessions := NewSessions(manager.DB(), SessionConfig{IdleTimeout: time.Hour, AbsoluteLifetime: 24 * time.Hour})
	sessions.now = func() time.Time { return now }
	token, csrf, session, err := sessions.IssueWithCSRF(ctx, "usr_test")
	if err != nil {
		t.Fatal(err)
	}
	if token == "" || csrf == "" || !session.ExpiresAt.Equal(now.Add(time.Hour)) {
		t.Fatal("invalid issued session")
	}
	mutation := httptest.NewRequest("POST", "https://binnacle.test/api/v1/auth/logout", nil)
	mutation.Header.Set("Origin", "https://binnacle.test")
	mutation.Header.Set("X-CSRF-Token", csrf)
	mutation.AddCookie(&http.Cookie{Name: SessionCookieName, Value: token})
	mutation.AddCookie(&http.Cookie{Name: CSRFCookieName, Value: csrf})
	if !sessions.ValidCSRF(mutation) {
		t.Fatal("valid CSRF rejected")
	}
	mutation.Header.Set("Origin", "https://evil.test")
	if sessions.ValidCSRF(mutation) {
		t.Fatal("cross-origin CSRF accepted")
	}
	var stored string
	if err = manager.DB().QueryRowContext(ctx, "SELECT id_hash FROM sessions").Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if stored == token {
		t.Fatal("plaintext token stored")
	}
	// SEC-21: fingerprints were stored but never enforced; the columns are gone.
	var fingerprint sql.NullString
	if err = manager.DB().QueryRowContext(ctx, "SELECT user_agent_hash FROM sessions").Scan(&fingerprint); err == nil {
		t.Fatal("user_agent_hash column still exists")
	}
	now = now.Add(30 * time.Minute)
	if _, err = sessions.Authenticate(ctx, token); err != nil {
		t.Fatal(err)
	}
	now = now.Add(2 * time.Hour)
	if _, err = sessions.Authenticate(ctx, token); err == nil {
		t.Fatal("idle-expired session accepted")
	}
	now = now.Add(time.Second)
	removed, err := sessions.Cleanup(ctx, 500)
	if err != nil || removed != 1 {
		t.Fatalf("cleanup removed=%d err=%v", removed, err)
	}
}

func TestConcurrentSessionsAndLogoutAll(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	manager := storage.New(filepath.Join(dir, "binnacle.db"), filepath.Join(dir, "run"))
	if err := manager.Open(ctx); err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	sessions := NewSessions(manager.DB(), SessionConfig{IdleTimeout: time.Hour, AbsoluteLifetime: 24 * time.Hour})
	var wg sync.WaitGroup
	tokens := make(chan string, 8)
	errs := make(chan error, 8)
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			token, _, err := sessions.Issue(ctx, "usr_test")
			if err != nil {
				errs <- err
				return
			}
			tokens <- token
		}()
	}
	wg.Wait()
	close(tokens)
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
	if err := sessions.RevokeAll(ctx, "usr_test"); err != nil {
		t.Fatal(err)
	}
	for token := range tokens {
		if _, err := sessions.Authenticate(ctx, token); err == nil {
			t.Fatal("revoked session accepted")
		}
	}
}

func TestSessionCookieFlags(t *testing.T) {
	w := httptest.NewRecorder()
	SetSessionCookie(w, "token", true, time.Now().Add(time.Hour))
	cookie := w.Result().Cookies()[0]
	if !cookie.HttpOnly || !cookie.Secure || cookie.SameSite != 2 || cookie.Path != "/" {
		t.Fatalf("cookie=%+v", cookie)
	}
}

func TestCurrentSessionRevocation(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	manager := storage.New(filepath.Join(dir, "binnacle.db"), filepath.Join(dir, "run"))
	if err := manager.Open(ctx); err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	sessions := NewSessions(manager.DB(), SessionConfig{IdleTimeout: time.Hour, AbsoluteLifetime: 24 * time.Hour})
	token, _, err := sessions.Issue(ctx, "usr_test")
	if err != nil {
		t.Fatal(err)
	}
	if err = sessions.Revoke(ctx, token); err != nil {
		t.Fatal(err)
	}
	if _, err = sessions.Authenticate(ctx, token); err == nil {
		t.Fatal("revoked current session accepted")
	}
}

func TestSessionLifetimesAreCapped(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	manager := storage.New(filepath.Join(dir, "binnacle.db"), filepath.Join(dir, "run"))
	if err := manager.Open(ctx); err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	now := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
	// SEC-19: a configuration above the ceilings still yields capped sessions.
	sessions := NewSessions(manager.DB(), SessionConfig{IdleTimeout: 30 * 24 * time.Hour, AbsoluteLifetime: 365 * 24 * time.Hour})
	sessions.now = func() time.Time { return now }
	_, session, err := sessions.Issue(ctx, "usr_test")
	if err != nil {
		t.Fatal(err)
	}
	if !session.ExpiresAt.Equal(now.Add(MaxSessionIdleTimeout)) || !session.AbsoluteExpires.Equal(now.Add(MaxSessionAbsoluteLifetime)) {
		t.Fatalf("expires=%s absolute=%s", session.ExpiresAt, session.AbsoluteExpires)
	}
	_, _, proxied, err := sessions.IssueForProxy(ctx, "usr_test", "subject-1")
	if err != nil {
		t.Fatal(err)
	}
	if !proxied.AbsoluteExpires.Equal(now.Add(ProxySessionLifetime)) || !proxied.ExpiresAt.Equal(now.Add(ProxySessionLifetime)) {
		t.Fatalf("proxy expires=%s absolute=%s", proxied.ExpiresAt, proxied.AbsoluteExpires)
	}
	// A configured lifetime below the proxy ceiling still wins.
	sessions.SetConfig(SessionConfig{IdleTimeout: time.Hour, AbsoluteLifetime: 2 * time.Hour})
	_, _, short, err := sessions.IssueForProxy(ctx, "usr_test", "subject-1")
	if err != nil {
		t.Fatal(err)
	}
	if !short.AbsoluteExpires.Equal(now.Add(2 * time.Hour)) {
		t.Fatalf("short proxy absolute=%s", short.AbsoluteExpires)
	}
}

func TestCheckDoesNotCountAsActivity(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	manager := storage.New(filepath.Join(dir, "binnacle.db"), filepath.Join(dir, "run"))
	if err := manager.Open(ctx); err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	now := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
	sessions := NewSessions(manager.DB(), SessionConfig{IdleTimeout: time.Hour, AbsoluteLifetime: 24 * time.Hour})
	sessions.now = func() time.Time { return now }
	token, _, err := sessions.Issue(ctx, "usr_test")
	if err != nil {
		t.Fatal(err)
	}
	now = now.Add(50 * time.Minute)
	if err = sessions.Check(ctx, token); err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest("GET", "https://binnacle.test/api/v1/live", nil)
	r.AddCookie(&http.Cookie{Name: SessionCookieName, Value: token})
	if !sessions.Recheck(r) {
		t.Fatal("valid session failed recheck")
	}
	// Check did not slide the idle expiry, so the original hour still ends it.
	now = now.Add(11 * time.Minute)
	if err = sessions.Check(ctx, token); err == nil {
		t.Fatal("idle-expired session passed Check")
	}
	if sessions.Recheck(r) {
		t.Fatal("idle-expired session passed Recheck")
	}
	if _, err = sessions.Authenticate(ctx, token); err == nil {
		t.Fatal("idle-expired session authenticated")
	}
}

func TestRevokeOthersKeepsTheCurrentSession(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	manager := storage.New(filepath.Join(dir, "binnacle.db"), filepath.Join(dir, "run"))
	if err := manager.Open(ctx); err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	sessions := NewSessions(manager.DB(), SessionConfig{IdleTimeout: time.Hour, AbsoluteLifetime: 24 * time.Hour})
	current, _, err := sessions.Issue(ctx, "usr_test")
	if err != nil {
		t.Fatal(err)
	}
	other, _, err := sessions.Issue(ctx, "usr_test")
	if err != nil {
		t.Fatal(err)
	}
	stranger, _, err := sessions.Issue(ctx, "usr_other")
	if err != nil {
		t.Fatal(err)
	}
	if err = sessions.RevokeOthers(ctx, "usr_test", current); err != nil {
		t.Fatal(err)
	}
	if _, err = sessions.Authenticate(ctx, other); err == nil {
		t.Fatal("other session survived")
	}
	if _, err = sessions.Authenticate(ctx, current); err != nil {
		t.Fatalf("current session revoked: %v", err)
	}
	if _, err = sessions.Authenticate(ctx, stranger); err != nil {
		t.Fatalf("another user's session revoked: %v", err)
	}
}
