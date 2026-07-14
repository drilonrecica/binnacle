// SPDX-License-Identifier: AGPL-3.0-only
package auth

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/drilonrecica/binnacle/internal/storage"
)

func TestAPITokenScopesExpiryAndRevocation(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	manager := storage.New(filepath.Join(dir, "db"), filepath.Join(dir, "run"))
	if err := manager.Open(ctx); err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	credentials := NewCredentials(manager.DB())
	user, err := credentials.CreateAdmin(ctx, "admin", "correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	repo := NewAPITokenRepository(manager.DB())
	now := time.Unix(1_800_000_000, 0).UTC()
	repo.now = func() time.Time { return now }
	expiry := now.Add(24 * time.Hour)
	token, plaintext, err := repo.Create(ctx, user.ID, "automation", []APIScope{ScopeMetricsRead, ScopeResourcesRead}, &expiry)
	if err != nil {
		t.Fatal(err)
	}
	if plaintext == "" || token.Prefix == "" {
		t.Fatal("plaintext token was not returned")
	}
	if err = repo.Authenticate(ctx, plaintext, ScopeMetricsRead); err != nil {
		t.Fatal(err)
	}
	if err = repo.Authenticate(ctx, plaintext, ScopeEventsRead); err != ErrAPIScopeInsufficient {
		t.Fatalf("wrong-scope error=%v", err)
	}
	listed, err := repo.List(ctx, user.ID)
	if err != nil || len(listed) != 1 || listed[0].LastUsedAt == nil {
		t.Fatalf("listed=%+v err=%v", listed, err)
	}
	if err = repo.Revoke(ctx, user.ID, token.ID); err != nil {
		t.Fatal(err)
	}
	if err = repo.Authenticate(ctx, plaintext, ScopeMetricsRead); err != ErrAPITokenInvalid {
		t.Fatalf("revoked error=%v", err)
	}
}

func TestAPITokenValidation(t *testing.T) {
	if _, err := normalizeScopes([]APIScope{ScopeMetricsRead, ScopeMetricsRead}); err == nil {
		t.Fatal("duplicate scope accepted")
	}
	if _, err := normalizeScopes([]APIScope{"settings:write"}); err == nil {
		t.Fatal("unknown scope accepted")
	}
}
