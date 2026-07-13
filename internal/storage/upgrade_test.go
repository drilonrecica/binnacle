// SPDX-License-Identifier: AGPL-3.0-only

package storage_test

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
	"testing"

	"github.com/drilonrecica/binnacle/internal/alerts"
	"github.com/drilonrecica/binnacle/internal/storage"
	"github.com/drilonrecica/binnacle/migrations"
	_ "github.com/mattn/go-sqlite3"
)

func TestUpgradeSchema15PreservesResourcesAndAddsContext(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "binnacle.db")
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.ExecContext(ctx, "PRAGMA foreign_keys=ON; CREATE TABLE schema_migrations (version INTEGER PRIMARY KEY, applied_at TEXT NOT NULL)"); err != nil {
		t.Fatal(err)
	}
	entries, err := fs.Glob(migrations.FS(), "*.sql")
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(entries)
	for _, entry := range entries {
		var version int
		if _, err = fmt.Sscanf(filepath.Base(entry), "%03d_", &version); err != nil {
			t.Fatal(err)
		}
		if version > 15 {
			break
		}
		body, readErr := migrations.FS().ReadFile(entry)
		if readErr != nil {
			t.Fatal(readErr)
		}
		if _, err = db.ExecContext(ctx, string(body)); err != nil {
			t.Fatalf("apply migration %d: %v", version, err)
		}
		if _, err = db.ExecContext(ctx, "INSERT INTO schema_migrations(version, applied_at) VALUES(?, datetime('now'))", version); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = db.ExecContext(ctx, "INSERT INTO hosts(id,identity_hash,name,updated_at) VALUES('host-1','identity-1','existing','2026-01-01T00:00:00Z')"); err != nil {
		t.Fatal(err)
	}
	if _, err = db.ExecContext(ctx, `INSERT INTO resources(id,host_id,stable_key,source_kind,name,category,status,first_seen_at,last_seen_at)
		VALUES('resource-1','host-1','stable-1','docker','existing','service','running',1,1)`); err != nil {
		t.Fatal(err)
	}
	if err = db.Close(); err != nil {
		t.Fatal(err)
	}

	manager := storage.New(dbPath, filepath.Join(dir, "runtime"))
	if err = manager.Open(ctx); err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	if version, versionErr := manager.SchemaVersion(ctx); versionErr != nil || version != 16 {
		t.Fatalf("schema version=%d err=%v", version, versionErr)
	}
	var name string
	if err = manager.DB().QueryRowContext(ctx, "SELECT name FROM resources WHERE id='resource-1'").Scan(&name); err != nil || name != "existing" {
		t.Fatalf("existing data was not preserved: name=%q err=%v", name, err)
	}
	var resourceContext string
	if err = manager.DB().QueryRowContext(ctx, "SELECT context FROM resources WHERE id='resource-1'").Scan(&resourceContext); err != nil || resourceContext != "" {
		t.Fatalf("existing resource context=%q err=%v", resourceContext, err)
	}

	repo := alerts.NewRepository(manager.DB())
	if err = repo.SeedDefaults(ctx); err != nil {
		t.Fatal(err)
	}
	if rules, rulesErr := repo.Rules(ctx); rulesErr != nil || len(rules) != len(alerts.DefaultRules()) {
		t.Fatalf("default rules=%d want=%d err=%v", len(rules), len(alerts.DefaultRules()), rulesErr)
	}
	if _, err = manager.DB().ExecContext(ctx, `INSERT INTO health_checks(id,resource_id,name,url,method,interval_seconds,timeout_seconds,expected_status_min,expected_status_max,created_at,updated_at)
		VALUES('bad','missing','bad','https://example.test','GET',30,5,200,299,1,1)`); err == nil {
		t.Fatal("foreign key constraint accepted a missing resource")
	}
	if _, err = manager.DB().ExecContext(ctx, `INSERT INTO health_checks(id,resource_id,name,url,method,interval_seconds,timeout_seconds,expected_status_min,expected_status_max,created_at,updated_at)
		VALUES('bad','resource-1','bad','https://example.test','POST',30,5,200,299,1,1)`); err == nil {
		t.Fatal("method constraint accepted POST")
	}
}
