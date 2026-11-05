// SPDX-License-Identifier: AGPL-3.0-only
package coolify

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/drilonrecica/binnacle/internal/auth"
	"github.com/drilonrecica/binnacle/internal/storage"
)

const testMasterKey = "000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f"

type recordingCoolify struct {
	*httptest.Server
	mu     sync.Mutex
	tokens []string
}

func newRecordingCoolify(t *testing.T) *recordingCoolify {
	t.Helper()
	rec := &recordingCoolify{}
	rec.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec.mu.Lock()
		rec.tokens = append(rec.tokens, r.Header.Get("Authorization"))
		rec.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `[]`)
	}))
	t.Cleanup(rec.Close)
	return rec
}

func (r *recordingCoolify) sawToken(token string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, seen := range r.tokens {
		if seen == "Bearer "+token {
			return true
		}
	}
	return false
}

func (r *recordingCoolify) requests() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.tokens)
}

func testIntegration(t *testing.T) *Integration {
	t.Helper()
	ctx := context.Background()
	store := storage.New(filepath.Join(t.TempDir(), "binnacle.db"), filepath.Join(t.TempDir(), "runtime"))
	if err := store.Open(ctx); err != nil {
		t.Fatal(err)
	}
	secrets, err := auth.NewSecretStore(store.DB(), testMasterKey)
	if err != nil {
		t.Fatal(err)
	}
	integration := NewIntegration(secrets, ClientConfig{AllowInsecureHTTP: true, HTTPClient: &http.Client{Timeout: 5 * time.Second}})
	integration.SetDB(store.DB())
	if err = integration.Start(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = integration.Stop(ctx)
		_ = store.Close()
	})
	return integration
}

func TestConfigureRequiresTokenAgainWhenOriginChanges(t *testing.T) {
	ctx := context.Background()
	integration := testIntegration(t)
	first := newRecordingCoolify(t)
	second := newRecordingCoolify(t)
	if err := integration.Configure(ctx, first.URL, "token-first"); err != nil {
		t.Fatal(err)
	}
	if err := integration.Configure(ctx, first.URL+"/", ""); err != nil {
		t.Fatalf("same origin without token: %v", err)
	}
	err := integration.Configure(ctx, second.URL, "")
	if err == nil {
		t.Fatal("origin change without token was accepted")
	}
	if status := integration.Status(); status.URL != first.URL+"/" {
		t.Fatalf("stored URL changed to %q", status.URL)
	}
	if second.sawToken("token-first") {
		t.Fatal("stored token was sent to the new origin")
	}
	if err = integration.Configure(ctx, second.URL, "token-second"); err != nil {
		t.Fatalf("origin change with token: %v", err)
	}
	if status := integration.Status(); status.URL != second.URL || !status.TokenConfigured {
		t.Fatalf("status=%+v", status)
	}
}

func TestTestUsesOnlyTheSubmittedToken(t *testing.T) {
	ctx := context.Background()
	integration := testIntegration(t)
	configured := newRecordingCoolify(t)
	other := newRecordingCoolify(t)
	if err := integration.Configure(ctx, configured.URL, "token-stored"); err != nil {
		t.Fatal(err)
	}
	if err := integration.Test(ctx, other.URL, ""); err == nil {
		t.Fatal("test without a token reached a new URL")
	}
	if other.requests() != 0 || other.sawToken("token-stored") {
		t.Fatalf("stored token was sent to the new URL: %d requests", other.requests())
	}
	if err := integration.Test(ctx, configured.URL, ""); err == nil {
		t.Fatal("test without a token fell back to the stored token")
	}
	if err := integration.Test(ctx, other.URL, "token-typed"); err != nil {
		t.Fatalf("test with submitted token: %v", err)
	}
	if !other.sawToken("token-typed") || other.sawToken("token-stored") {
		t.Fatalf("tokens seen by the new URL: %v", other.tokens)
	}
}
