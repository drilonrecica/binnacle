// SPDX-License-Identifier: AGPL-3.0-only
package auth

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/drilonrecica/binnacle/internal/storage"
)

func TestTOTPRFC6238SHA1VectorsAtSixDigits(t *testing.T) {
	seed := []byte("12345678901234567890")
	vectors := []struct {
		unix int64
		code string
	}{{59, "287082"}, {1111111109, "081804"}, {1111111111, "050471"}, {1234567890, "005924"}, {2000000000, "279037"}, {20000000000, "353130"}}
	for _, vector := range vectors {
		if got := TOTP(seed, time.Unix(vector.unix, 0)); got != vector.code {
			t.Errorf("TOTP(%d)=%s want %s", vector.unix, got, vector.code)
		}
	}
}

func TestTOTPClockTolerance(t *testing.T) {
	seed := []byte("01234567890123456789")
	at := time.Unix(1_800_000_000, 0)
	if !VerifyTOTP(seed, TOTP(seed, at.Add(-30*time.Second)), at) || !VerifyTOTP(seed, TOTP(seed, at.Add(30*time.Second)), at) {
		t.Fatal("adjacent clock step rejected")
	}
	if VerifyTOTP(seed, TOTP(seed, at.Add(60*time.Second)), at) {
		t.Fatal("code beyond tolerance accepted")
	}
}

func TestMFAEnrollmentAndAtomicRecoveryConsumption(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	store := storage.New(filepath.Join(dir, "db"), filepath.Join(dir, "runtime"))
	if err := store.Open(ctx); err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	credentials := NewCredentials(store.DB())
	user, err := credentials.CreateAdmin(ctx, "admin", "correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	sessions := NewSessions(store.DB(), SessionConfig{IdleTimeout: time.Hour, AbsoluteLifetime: 24 * time.Hour})
	secrets, err := NewSecretStore(store.DB(), "000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f")
	if err != nil {
		t.Fatal(err)
	}
	mfa := NewMFA(store.DB(), credentials, secrets, sessions)
	fixed := time.Unix(1_800_000_000, 0).UTC()
	mfa.now = func() time.Time { return fixed }
	enrollment, err := mfa.Begin(ctx, user, "correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	if enrollment.Seed == "" || enrollment.URI == "" {
		t.Fatalf("enrollment=%+v", enrollment)
	}
	mfa.mu.Lock()
	seed := append([]byte(nil), mfa.pending[user.ID].seed...)
	mfa.mu.Unlock()
	codes, err := mfa.Confirm(ctx, user.ID, TOTP(seed, fixed))
	if err != nil {
		t.Fatal(err)
	}
	if len(codes) != 10 {
		t.Fatalf("recovery codes=%d", len(codes))
	}
	if err = mfa.Verify(ctx, user.ID, codes[0]); err != nil {
		t.Fatal(err)
	}
	if err = mfa.Verify(ctx, user.ID, codes[0]); err == nil {
		t.Fatal("recovery code was reused")
	}
	// The confirmation code was consumed by Confirm; the next step is fresh.
	fixed = fixed.Add(TOTPPeriod)
	if err = mfa.Verify(ctx, user.ID, TOTP(seed, fixed)); err != nil {
		t.Fatal(err)
	}
	var stored int
	if err = store.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM recovery_codes WHERE user_id=? AND used_at IS NOT NULL", user.ID).Scan(&stored); err != nil || stored != 1 {
		t.Fatalf("used recovery count=%d err=%v", stored, err)
	}
}

func TestMFAEnrollmentRequiresMasterKeyAndPassword(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	store := storage.New(filepath.Join(dir, "db"), filepath.Join(dir, "runtime"))
	if err := store.Open(ctx); err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	credentials := NewCredentials(store.DB())
	user, _ := credentials.CreateAdmin(ctx, "admin", "correct horse battery staple")
	sessions := NewSessions(store.DB(), SessionConfig{IdleTimeout: time.Hour, AbsoluteLifetime: time.Hour})
	secrets, _ := NewSecretStore(store.DB(), "")
	mfa := NewMFA(store.DB(), credentials, secrets, sessions)
	if _, err := mfa.Begin(ctx, user, "correct horse battery staple"); err != ErrMasterKeyMissing {
		t.Fatalf("error=%v", err)
	}
}

func TestAdvancedAuthDisabledRefusesStoredTOTPEnrollment(t *testing.T) {
	ctx := context.Background()
	manager := storage.New(filepath.Join(t.TempDir(), "binnacle.db"), filepath.Join(t.TempDir(), "run"))
	if err := manager.Open(ctx); err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	credentials := NewCredentials(manager.DB())
	user, err := credentials.CreateAdmin(ctx, "admin", "correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = manager.DB().ExecContext(ctx, "UPDATE users SET totp_enabled=1 WHERE id=?", user.ID); err != nil {
		t.Fatal(err)
	}
	if err = ValidateAdvancedAuthState(ctx, manager.DB(), false); err == nil {
		t.Fatal("disabled advanced auth accepted stored TOTP enrollment")
	}
	if err = ValidateAdvancedAuthState(ctx, manager.DB(), true); err != nil {
		t.Fatalf("enabled advanced auth rejected stored enrollment: %v", err)
	}
}

func TestTOTPCodeIsAcceptedOnlyOnce(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	store := storage.New(filepath.Join(dir, "db"), filepath.Join(dir, "runtime"))
	if err := store.Open(ctx); err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	credentials := NewCredentials(store.DB())
	user, err := credentials.CreateAdmin(ctx, "admin", "correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	sessions := NewSessions(store.DB(), SessionConfig{IdleTimeout: time.Hour, AbsoluteLifetime: 24 * time.Hour})
	secrets, err := NewSecretStore(store.DB(), "000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f")
	if err != nil {
		t.Fatal(err)
	}
	mfa := NewMFA(store.DB(), credentials, secrets, sessions)
	now := time.Unix(1_800_000_000, 0).UTC()
	mfa.now = func() time.Time { return now }
	if _, err = mfa.Begin(ctx, user, "correct horse battery staple"); err != nil {
		t.Fatal(err)
	}
	mfa.mu.Lock()
	seed := append([]byte(nil), mfa.pending[user.ID].seed...)
	mfa.mu.Unlock()
	if _, err = mfa.Confirm(ctx, user.ID, TOTP(seed, now)); err != nil {
		t.Fatal(err)
	}
	// SEC-8: the code that confirmed enrollment is already spent.
	if err = mfa.Verify(ctx, user.ID, TOTP(seed, now)); err == nil {
		t.Fatal("confirmation code was replayed at login")
	}
	now = now.Add(TOTPPeriod)
	current := TOTP(seed, now)
	if err = mfa.Verify(ctx, user.ID, current); err != nil {
		t.Fatalf("fresh step rejected: %v", err)
	}
	if err = mfa.Verify(ctx, user.ID, current); err == nil {
		t.Fatal("code accepted twice within its window")
	}
	// The previous step is still inside clock tolerance but behind the
	// accepted one, so a shoulder-surfed earlier code is useless.
	if err = mfa.Verify(ctx, user.ID, TOTP(seed, now.Add(-TOTPPeriod))); err == nil {
		t.Fatal("older step accepted after a newer one")
	}
	// The next step can be used slightly early (clock tolerance) and then
	// becomes the high-water mark.
	if err = mfa.Verify(ctx, user.ID, TOTP(seed, now.Add(TOTPPeriod))); err != nil {
		t.Fatalf("next step within tolerance rejected: %v", err)
	}
	now = now.Add(TOTPPeriod)
	if err = mfa.Verify(ctx, user.ID, TOTP(seed, now)); err == nil {
		t.Fatal("step accepted after it was already used early")
	}
	var last int64
	if err = store.DB().QueryRowContext(ctx, "SELECT totp_last_step FROM users WHERE id=?", user.ID).Scan(&last); err != nil || last != now.Unix()/30 {
		t.Fatalf("stored step=%d err=%v want %d", last, err, now.Unix()/30)
	}
}

func TestMatchTOTPReportsTheStep(t *testing.T) {
	seed := []byte("01234567890123456789")
	at := time.Unix(1_800_000_000, 0)
	for delta := int64(-1); delta <= 1; delta++ {
		step, ok := MatchTOTP(seed, TOTP(seed, at.Add(time.Duration(delta)*TOTPPeriod)), at)
		if !ok || step != at.Unix()/30+delta {
			t.Fatalf("delta=%d step=%d ok=%v", delta, step, ok)
		}
	}
	if _, ok := MatchTOTP(seed, "12345", at); ok {
		t.Fatal("short code matched")
	}
	if _, ok := MatchTOTP(seed, "12345a", at); ok {
		t.Fatal("non-numeric code matched")
	}
}
