// SPDX-License-Identifier: AGPL-3.0-only
package auth

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	"golang.org/x/crypto/argon2"
)

// legacyHash builds a hash the way releases before the 32 MiB change did, so
// the tests prove old credentials keep working after an upgrade.
func legacyHash(t *testing.T, password string, memoryKiB uint32) string {
	t.Helper()
	salt := []byte("0123456789abcdef")
	key := argon2.IDKey([]byte(password), salt, 3, memoryKiB, 4, 32)
	return fmt.Sprintf("$argon2id$v=19$m=%d,t=3,p=4$%s$%s", memoryKiB,
		base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(key))
}

func TestHashParametersUse32MiB(t *testing.T) {
	hash, err := HashPassword("correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(hash, "$m=32768,t=3,") {
		t.Fatalf("hash=%s", hash)
	}
}

func TestVerifyKeepsAcceptingLegacy64MiBHashes(t *testing.T) {
	hash := legacyHash(t, "correct horse battery staple", 64*1024)
	ok, err := VerifyPassword(hash, "correct horse battery staple")
	if err != nil || !ok {
		t.Fatalf("legacy hash ok=%v err=%v", ok, err)
	}
}

func TestVerifyRejectsHashesAboveMemoryBudget(t *testing.T) {
	hash := legacyHash(t, "correct horse battery staple", 65*1024)
	ok, err := VerifyPassword(hash, "correct horse battery staple")
	if err != nil || ok {
		t.Fatalf("oversized hash ok=%v err=%v", ok, err)
	}
}

func TestHashingReportsBusyWhenBudgetHeld(t *testing.T) {
	if !passwordHashing.acquire(2) {
		t.Fatal("budget already held")
	}
	defer passwordHashing.release(2)
	if _, err := HashPassword("correct horse battery staple"); !errors.Is(err, ErrHashingBusy) {
		t.Fatalf("hash err=%v", err)
	}
	hash := legacyHash(t, "correct horse battery staple", 32*1024)
	if ok, err := VerifyPassword(hash, "correct horse battery staple"); ok || !errors.Is(err, ErrHashingBusy) {
		t.Fatalf("verify ok=%v err=%v", ok, err)
	}
	passwordHashing.release(1)
	defer passwordHashing.acquire(1)
	if ok, err := VerifyPassword(hash, "correct horse battery staple"); !ok || err != nil {
		t.Fatalf("one free unit ok=%v err=%v", ok, err)
	}
	legacy := legacyHash(t, "correct horse battery staple", 64*1024)
	if ok, err := VerifyPassword(legacy, "correct horse battery staple"); ok || !errors.Is(err, ErrHashingBusy) {
		t.Fatalf("64 MiB hash with one unit ok=%v err=%v", ok, err)
	}
}

func TestMalformedHashesFailWithoutTouchingTheBudget(t *testing.T) {
	if !passwordHashing.acquire(2) {
		t.Fatal("budget already held")
	}
	defer passwordHashing.release(2)
	if ok, err := VerifyPassword("not-a-hash", "whatever password"); ok || err != nil {
		t.Fatalf("malformed ok=%v err=%v", ok, err)
	}
}

func TestConcurrentVerificationsStayWithinMemoryBudget(t *testing.T) {
	hash, err := HashPassword("correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	legacy := legacyHash(t, "correct horse battery staple", 64*1024)
	passwordHashing.mu.Lock()
	passwordHashing.peak = 0
	passwordHashing.mu.Unlock()
	var wg sync.WaitGroup
	results := make(chan error, 8)
	for i := 0; i < 8; i++ {
		encoded := hash
		if i%4 == 0 {
			encoded = legacy
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			ok, err := VerifyPassword(encoded, "correct horse battery staple")
			if err == nil && !ok {
				err = errors.New("valid password rejected")
			}
			results <- err
		}()
	}
	wg.Wait()
	close(results)
	for err := range results {
		if err != nil && !errors.Is(err, ErrHashingBusy) {
			t.Fatal(err)
		}
	}
	passwordHashing.mu.Lock()
	used, peak, limit := passwordHashing.used, passwordHashing.peak, passwordHashing.limit
	passwordHashing.mu.Unlock()
	if used != 0 || peak > limit || peak == 0 {
		t.Fatalf("used=%d peak=%d limit=%d", used, peak, limit)
	}
}

func TestAuthenticateReportsBusyForKnownAndUnknownUsers(t *testing.T) {
	ctx := context.Background()
	manager, _ := setupDatabase(t)
	credentials := NewCredentials(manager.DB())
	if _, err := credentials.CreateAdmin(ctx, "admin", "correct horse battery staple"); err != nil {
		t.Fatal(err)
	}
	if !passwordHashing.acquire(2) {
		t.Fatal("budget already held")
	}
	defer passwordHashing.release(2)
	if _, err := credentials.Authenticate(ctx, "admin", "correct horse battery staple"); !errors.Is(err, ErrHashingBusy) {
		t.Fatalf("known user err=%v", err)
	}
	if _, err := credentials.Authenticate(ctx, "nobody", "correct horse battery staple"); !errors.Is(err, ErrHashingBusy) {
		t.Fatalf("unknown user err=%v", err)
	}
}

func TestExistsOnlyReportsRealAccounts(t *testing.T) {
	ctx := context.Background()
	manager, _ := setupDatabase(t)
	credentials := NewCredentials(manager.DB())
	if _, err := credentials.CreateAdmin(ctx, "admin", "correct horse battery staple"); err != nil {
		t.Fatal(err)
	}
	if !credentials.Exists(ctx, " Admin ") {
		t.Fatal("real account not found")
	}
	if credentials.Exists(ctx, "nobody") || credentials.Exists(ctx, "@bad") {
		t.Fatal("unknown or invalid name reported as existing")
	}
}

func TestClaimChecksTokenBeforeHashing(t *testing.T) {
	ctx := context.Background()
	_, setup := setupDatabase(t)
	token := "0123456789abcdefghijklmnopqrstuvwxyz-ORDER"
	if _, err := setup.Initialize(ctx, ":8080", token); err != nil {
		t.Fatal(err)
	}
	if !passwordHashing.acquire(2) {
		t.Fatal("budget already held")
	}
	defer passwordHashing.release(2)
	if _, err := setup.Claim(ctx, "wrong-token", "admin", "correct horse battery staple"); !errors.Is(err, ErrSetupToken) {
		t.Fatalf("wrong token err=%v", err)
	}
	if _, err := setup.Claim(ctx, token, "admin", "correct horse battery staple"); !errors.Is(err, ErrHashingBusy) {
		t.Fatalf("valid token while busy err=%v", err)
	}
	if setup.Available(ctx) != true {
		t.Fatal("busy claim consumed the setup token")
	}
}

func TestClaimWithoutSetupStateIsUnavailable(t *testing.T) {
	ctx := context.Background()
	_, setup := setupDatabase(t)
	if _, err := setup.Claim(ctx, "any-token", "admin", "correct horse battery staple"); !errors.Is(err, ErrSetupUnavailable) {
		t.Fatalf("err=%v", err)
	}
}
