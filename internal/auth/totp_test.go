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
	secrets, err := NewSecretStore(store.DB(), "01234567890123456789012345678901")
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
