// SPDX-License-Identifier: AGPL-3.0-only
package auth

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/drilonrecica/binnacle/internal/storage"
)

func setupDatabase(t *testing.T) (*storage.Manager, *SetupService) {
	t.Helper()
	dir := t.TempDir()
	manager := storage.New(filepath.Join(dir, "binnacle.db"), filepath.Join(dir, "run"))
	if err := manager.Open(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = manager.Close() })
	return manager, NewSetupService(manager.DB())
}

func TestSetupPolicyAndPermanentDisable(t *testing.T) {
	ctx := context.Background()
	_, setup := setupDatabase(t)
	if _, err := setup.Initialize(ctx, ":8080", ""); err == nil {
		t.Fatal("public listener accepted setup without an operator token")
	}
	token := "0123456789abcdefghijklmnopqrstuvwxyz-SETUP"
	if generated, err := setup.Initialize(ctx, ":8080", token); err != nil || generated != "" {
		t.Fatalf("initialize generated=%q err=%v", generated, err)
	}
	if _, err := setup.Claim(ctx, token, "admin", "correct horse battery staple"); err != nil {
		t.Fatal(err)
	}
	if setup.Available(ctx) {
		t.Fatal("setup remained available after claim")
	}
	if err := setup.Verify(ctx, token); !errors.Is(err, ErrSetupToken) {
		t.Fatalf("replay err=%v", err)
	}
	if _, err := setup.Initialize(ctx, ":8080", token); err != nil {
		t.Fatal(err)
	}
	if setup.Available(ctx) {
		t.Fatal("startup silently re-enabled setup")
	}
}

func TestLocalGeneratedSetupTokenExpires(t *testing.T) {
	ctx := context.Background()
	_, setup := setupDatabase(t)
	now := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
	setup.now = func() time.Time { return now }
	setup.SetTokenFile(filepath.Join(t.TempDir(), SetupTokenFileName))
	generated, err := setup.Initialize(ctx, "127.0.0.1:8080", "")
	if err != nil || len(generated) < 32 {
		t.Fatalf("generated=%q err=%v", generated, err)
	}
	now = now.Add(24 * time.Hour)
	if setup.Available(ctx) || setup.Verify(ctx, generated) == nil {
		t.Fatal("expired setup token accepted")
	}
}

func TestConcurrentSetupClaimAllowsOneAdmin(t *testing.T) {
	ctx := context.Background()
	_, setup := setupDatabase(t)
	token := "0123456789abcdefghijklmnopqrstuvwxyz-RACE"
	if _, err := setup.Initialize(ctx, ":8080", token); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for _, username := range []string{"admin-one", "admin-two"} {
		wg.Add(1)
		go func(username string) {
			defer wg.Done()
			_, err := setup.Claim(ctx, token, username, "correct horse battery staple")
			errs <- err
		}(username)
	}
	wg.Wait()
	close(errs)
	successes := 0
	for err := range errs {
		if err == nil {
			successes++
		}
	}
	if successes != 1 {
		t.Fatalf("successful claims=%d", successes)
	}
}

func TestLoopbackSetupTokenIsWrittenToPrivateFile(t *testing.T) {
	ctx := context.Background()
	_, setup := setupDatabase(t)
	path := filepath.Join(t.TempDir(), "run", SetupTokenFileName)
	setup.SetTokenFile(path)
	generated, err := setup.Initialize(ctx, "127.0.0.1:8080", "")
	if err != nil || generated == "" {
		t.Fatalf("generated=%q err=%v", generated, err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if mode := info.Mode().Perm(); mode != 0o600 {
		t.Fatalf("token file mode=%o, want 600", mode)
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(contents)) != generated {
		t.Fatal("token file does not hold the generated token")
	}
	if err := setup.Verify(ctx, strings.TrimSpace(string(contents))); err != nil {
		t.Fatalf("token from file rejected: %v", err)
	}
	if _, err := setup.Claim(ctx, generated, "admin", "correct horse battery staple"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("token file survived the claim: %v", err)
	}
}

func TestLoopbackWithConfiguredTokenWritesNoFile(t *testing.T) {
	ctx := context.Background()
	_, setup := setupDatabase(t)
	path := filepath.Join(t.TempDir(), SetupTokenFileName)
	if err := os.WriteFile(path, []byte("stale\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	setup.SetTokenFile(path)
	if generated, err := setup.Initialize(ctx, "127.0.0.1:8080", "0123456789abcdefghijklmnopqrstuvwxyz-CONF"); err != nil || generated != "" {
		t.Fatalf("generated=%q err=%v", generated, err)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("stale token file kept with an operator token: %v", err)
	}
}

func TestExpiredUnclaimedSetupIsReArmedOnStart(t *testing.T) {
	ctx := context.Background()
	_, setup := setupDatabase(t)
	now := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
	setup.now = func() time.Time { return now }
	token := "0123456789abcdefghijklmnopqrstuvwxyz-REARM"
	if _, err := setup.Initialize(ctx, ":8080", token); err != nil {
		t.Fatal(err)
	}
	now = now.Add(48 * time.Hour)
	if setup.Available(ctx) {
		t.Fatal("expired token still available")
	}
	if _, err := setup.Initialize(ctx, ":8080", token); err != nil {
		t.Fatal(err)
	}
	if !setup.Available(ctx) || setup.Verify(ctx, token) != nil {
		t.Fatal("expired unclaimed setup was not re-armed with the configured token")
	}
	if _, err := setup.Initialize(ctx, ":8080", ""); err != nil {
		t.Fatalf("restart without the token while the earlier one is valid: %v", err)
	}
	now = now.Add(48 * time.Hour)
	if _, err := setup.Initialize(ctx, ":8080", ""); err == nil {
		t.Fatal("public listener re-armed without an operator token")
	}
}

func TestExpiredLoopbackSetupGetsFreshToken(t *testing.T) {
	ctx := context.Background()
	_, setup := setupDatabase(t)
	now := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
	setup.now = func() time.Time { return now }
	path := filepath.Join(t.TempDir(), SetupTokenFileName)
	setup.SetTokenFile(path)
	first, err := setup.Initialize(ctx, "127.0.0.1:8080", "")
	if err != nil {
		t.Fatal(err)
	}
	now = now.Add(25 * time.Hour)
	second, err := setup.Initialize(ctx, "127.0.0.1:8080", "")
	if err != nil || second == "" || second == first {
		t.Fatalf("second=%q err=%v", second, err)
	}
	contents, err := os.ReadFile(path)
	if err != nil || strings.TrimSpace(string(contents)) != second {
		t.Fatalf("token file not rewritten: %q err=%v", contents, err)
	}
	if !setup.Available(ctx) || setup.Verify(ctx, second) != nil || setup.Verify(ctx, first) == nil {
		t.Fatal("re-armed loopback setup does not accept only the fresh token")
	}
}

func TestClaimedSetupIsNeverReArmed(t *testing.T) {
	ctx := context.Background()
	_, setup := setupDatabase(t)
	if err := setup.Disable(ctx); err != nil {
		t.Fatal(err)
	}
	setup.SetTokenFile(filepath.Join(t.TempDir(), SetupTokenFileName))
	if generated, err := setup.Initialize(ctx, "127.0.0.1:8080", ""); err != nil || generated != "" {
		t.Fatalf("generated=%q err=%v", generated, err)
	}
	if setup.Available(ctx) {
		t.Fatal("disabled setup re-armed")
	}
}
