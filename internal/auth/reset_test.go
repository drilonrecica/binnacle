// SPDX-License-Identifier: AGPL-3.0-only
package auth

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/drilonrecica/binnacle/internal/storage"
)

const (
	resetOldPassword = "correct horse battery staple"
	resetNewPassword = "a brand new password value"
)

type resetFixture struct {
	db          *storage.DB
	credentials *Credentials
	sessions    *Sessions
	secrets     *SecretStore
	user        User
}

func newResetFixture(t *testing.T) resetFixture {
	t.Helper()
	ctx := context.Background()
	dir := t.TempDir()
	manager := storage.New(filepath.Join(dir, "db"), filepath.Join(dir, "run"))
	if err := manager.Open(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := manager.Close(); err != nil {
			t.Error(err)
		}
	})
	credentials := NewCredentials(manager.DB())
	user, err := credentials.CreateAdmin(ctx, "Skipper", resetOldPassword)
	if err != nil {
		t.Fatal(err)
	}
	secrets, err := NewSecretStore(manager.DB(), "000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f")
	if err != nil {
		t.Fatal(err)
	}
	sessions := NewSessions(manager.DB(), SessionConfig{IdleTimeout: time.Hour, AbsoluteLifetime: 24 * time.Hour})
	return resetFixture{db: manager.DB(), credentials: credentials, sessions: sessions, secrets: secrets, user: user}
}

// enrollMFA turns TOTP on for the fixture's admin.
func (f resetFixture) enrollMFA(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	mfa := NewMFA(f.db, f.credentials, f.secrets, f.sessions)
	fixed := time.Unix(1_800_000_000, 0).UTC()
	mfa.now = func() time.Time { return fixed }
	if _, err := mfa.Begin(ctx, f.user, resetOldPassword); err != nil {
		t.Fatal(err)
	}
	mfa.mu.Lock()
	seed := append([]byte(nil), mfa.pending[f.user.ID].seed...)
	mfa.mu.Unlock()
	if _, err := mfa.Confirm(ctx, f.user.ID, TOTP(seed, fixed), ""); err != nil {
		t.Fatal(err)
	}
}

func (f resetFixture) count(t *testing.T, query string, args ...any) int {
	t.Helper()
	var n int
	if err := f.db.QueryRowContext(context.Background(), query, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestResetAdminPasswordReplacesPasswordAndRevokesSessions(t *testing.T) {
	ctx := context.Background()
	f := newResetFixture(t)
	token, _, err := f.sessions.Issue(ctx, f.user.ID)
	if err != nil {
		t.Fatal(err)
	}
	tokens := NewAPITokenRepository(f.db)
	_, apiToken, err := tokens.Create(ctx, f.user.ID, "automation", []APIScope{ScopeMetricsRead}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.db.ExecContext(ctx, `INSERT INTO user_preferences(user_id,schema_version,theme,density,pinned_resources_json,landing_page,chart_range,updated_at) VALUES(?,1,'dark','compact','[]','overview','24h',1)`, f.user.ID); err != nil {
		t.Fatal(err)
	}

	result, err := ResetAdminPassword(ctx, f.db, resetNewPassword, false)
	if err != nil {
		t.Fatal(err)
	}
	if result.User.ID != f.user.ID || result.User.Username != "skipper" || result.MFAEnabled {
		t.Fatalf("result=%+v", result)
	}
	if got, err := f.credentials.Authenticate(ctx, "skipper", resetNewPassword); err != nil || got.ID != f.user.ID {
		t.Fatalf("new password: user=%+v err=%v", got, err)
	}
	if _, err = f.credentials.Authenticate(ctx, "skipper", resetOldPassword); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("old password err=%v", err)
	}
	if err = f.sessions.Check(ctx, token); err == nil {
		t.Fatal("session survived the reset")
	}
	if err = tokens.Authenticate(ctx, apiToken, ScopeMetricsRead); err != nil {
		t.Fatalf("API token was not kept: %v", err)
	}
	if n := f.count(t, "SELECT COUNT(*) FROM user_preferences WHERE user_id=?", f.user.ID); n != 1 {
		t.Fatalf("preferences rows=%d", n)
	}
}

func TestResetAdminPasswordLeavesMFAUnlessAsked(t *testing.T) {
	ctx := context.Background()
	f := newResetFixture(t)
	f.enrollMFA(t)
	secretsBefore := f.count(t, "SELECT COUNT(*) FROM encrypted_secrets")
	if secretsBefore == 0 {
		t.Fatal("enrollment stored no encrypted seed")
	}

	result, err := ResetAdminPassword(ctx, f.db, resetNewPassword, false)
	if err != nil {
		t.Fatal(err)
	}
	if !result.MFAEnabled {
		t.Fatal("result reports MFA off although it was not disabled")
	}
	if n := f.count(t, "SELECT totp_enabled FROM users WHERE id=?", f.user.ID); n != 1 {
		t.Fatalf("totp_enabled=%d", n)
	}
	if n := f.count(t, "SELECT COUNT(*) FROM recovery_codes WHERE user_id=?", f.user.ID); n != 10 {
		t.Fatalf("recovery codes=%d", n)
	}

	result, err = ResetAdminPassword(ctx, f.db, resetNewPassword, true)
	if err != nil {
		t.Fatal(err)
	}
	if result.MFAEnabled {
		t.Fatal("result reports MFA on after it was disabled")
	}
	if n := f.count(t, "SELECT COUNT(*) FROM users WHERE id=? AND totp_enabled=0 AND totp_secret_key IS NULL AND totp_last_step=0", f.user.ID); n != 1 {
		t.Fatal("TOTP columns were not cleared")
	}
	if n := f.count(t, "SELECT COUNT(*) FROM recovery_codes WHERE user_id=?", f.user.ID); n != 0 {
		t.Fatalf("recovery codes=%d", n)
	}
	if n := f.count(t, "SELECT COUNT(*) FROM encrypted_secrets"); n != secretsBefore-1 {
		t.Fatalf("encrypted secrets=%d, want %d", n, secretsBefore-1)
	}
	// The advanced-auth gate may now be turned off again.
	if err = ValidateAdvancedAuthState(ctx, f.db, false); err != nil {
		t.Fatal(err)
	}
}

func TestResetAdminPasswordChangesNothingOnFailure(t *testing.T) {
	ctx := context.Background()
	f := newResetFixture(t)
	token, _, err := f.sessions.Issue(ctx, f.user.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = ResetAdminPassword(ctx, f.db, "too short", false); err == nil {
		t.Fatal("short password accepted")
	}
	if _, err = f.credentials.Authenticate(ctx, "skipper", resetOldPassword); err != nil {
		t.Fatalf("old password stopped working: %v", err)
	}
	if err = f.sessions.Check(ctx, token); err != nil {
		t.Fatalf("session was revoked by a failed reset: %v", err)
	}
}

func TestResetAdminPasswordWithoutAdmin(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	manager := storage.New(filepath.Join(dir, "db"), filepath.Join(dir, "run"))
	if err := manager.Open(ctx); err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	if _, err := ResetAdminPassword(ctx, manager.DB(), resetNewPassword, true); !errors.Is(err, ErrNoAdmin) {
		t.Fatalf("err=%v", err)
	}
}

func TestGeneratePasswordIsValidAndFresh(t *testing.T) {
	first, err := GeneratePassword()
	if err != nil {
		t.Fatal(err)
	}
	second, err := GeneratePassword()
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != 24 || first == second {
		t.Fatalf("first=%q second=%q", first, second)
	}
	if err = ValidatePassword(first); err != nil {
		t.Fatal(err)
	}
}
