// SPDX-License-Identifier: AGPL-3.0-only
package preferences

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/drilonrecica/binnacle/internal/storage"
)

func TestPutGetAndValidate(t *testing.T) {
	store := storage.New(t.TempDir()+"/test.db", t.TempDir())
	if err := store.Open(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if _, err := store.DB().Exec(`INSERT INTO users(id,username,password_hash,created_at,updated_at) VALUES('user-1','admin','hash',1,1)`); err != nil {
		t.Fatal(err)
	}
	repo := NewRepository(store.DB())
	repo.now = func() time.Time { return time.Unix(123, 0).UTC() }
	want := Value{SchemaVersion: 1, Theme: "dark", Density: "compact", PinnedResources: []string{"resource-2", "resource-1"}, LandingPage: "resources", ChartRange: "7d"}
	if _, err := repo.Put(context.Background(), "user-1", want); err != nil {
		t.Fatal(err)
	}
	got, exists, err := repo.Get(context.Background(), "user-1")
	if err != nil || !exists || got.Theme != want.Theme || len(got.PinnedResources) != 2 || got.UpdatedAt.Unix() != 123 {
		t.Fatalf("got=%+v exists=%v err=%v", got, exists, err)
	}
	invalid := want
	invalid.PinnedResources = make([]string, 13)
	if _, err = repo.Put(context.Background(), "user-1", invalid); err == nil {
		t.Fatal("accepted too many pins")
	}
	invalid = want
	invalid.LandingPage = "settings"
	if _, err = repo.Put(context.Background(), "user-1", invalid); err == nil {
		t.Fatal("accepted invalid landing page")
	}
}

func TestPutNormalizesLegacyLandingPages(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	store := storage.New(filepath.Join(dir, "db"), filepath.Join(dir, "run"))
	if err := store.Open(ctx); err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if _, err := store.DB().ExecContext(ctx, "INSERT INTO users(id,username,password_hash,created_at,updated_at) VALUES('u2','legacy','hash',0,0)"); err != nil {
		t.Fatal(err)
	}
	repo := NewRepository(store.DB())
	for legacy, want := range map[string]string{"watch": "overview", "server": "host", "events": "activity", "logs": "logs"} {
		saved, err := repo.Put(ctx, "u2", Value{Theme: "dark", Density: "compact", LandingPage: legacy, ChartRange: "1h"})
		if err != nil || saved.LandingPage != want {
			t.Fatalf("%s: saved=%+v err=%v", legacy, saved, err)
		}
		loaded, _, err := repo.Get(ctx, "u2")
		if err != nil || loaded.LandingPage != want {
			t.Fatalf("%s: loaded=%+v err=%v", legacy, loaded, err)
		}
	}
}
