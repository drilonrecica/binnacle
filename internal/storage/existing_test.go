// SPDX-License-Identifier: AGPL-3.0-only

package storage

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestOpenExistingNeverCreatesADatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing.db")
	if _, _, err := OpenExisting(context.Background(), path); err == nil {
		t.Fatal("missing database opened")
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("database file was created: %v", err)
	}
}

func TestOpenExistingWritesBesideARunningManager(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	path, runtime := filepath.Join(dir, "binnacle.db"), filepath.Join(dir, "run")
	manager := New(path, runtime)
	if err := manager.Open(ctx); err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	before, err := os.ReadDir(runtime)
	if err != nil {
		t.Fatal(err)
	}

	db, closeDB, err := OpenExisting(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	// Both processes write: the manager's writer and the second connection.
	if _, err = manager.DB().ExecContext(ctx, "INSERT INTO settings(key,value_json,updated_at) VALUES('from-manager','1',1)"); err != nil {
		t.Fatal(err)
	}
	if _, err = db.ExecContext(ctx, "INSERT INTO settings(key,value_json,updated_at) VALUES('from-command','1',1)"); err != nil {
		t.Fatal(err)
	}
	if err = closeDB(); err != nil {
		t.Fatal(err)
	}

	var rows int
	if err = manager.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM settings WHERE key IN ('from-manager','from-command')").Scan(&rows); err != nil || rows != 2 {
		t.Fatalf("rows=%d err=%v", rows, err)
	}
	after, err := os.ReadDir(runtime)
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != len(before) {
		t.Fatalf("runtime directory changed: before=%d after=%d entries", len(before), len(after))
	}
	if _, err = os.Stat(manager.openMarkerPath()); err != nil {
		t.Fatalf("the manager's open marker was disturbed: %v", err)
	}
}

func TestOpenExistingRefusesAnotherSchemaVersion(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	path := filepath.Join(dir, "binnacle.db")
	manager := New(path, filepath.Join(dir, "run"))
	if err := manager.Open(ctx); err != nil {
		t.Fatal(err)
	}
	latest, err := latestMigration()
	if err != nil {
		t.Fatal(err)
	}
	if _, err = manager.DB().ExecContext(ctx, "DELETE FROM schema_migrations WHERE version=?", latest); err != nil {
		t.Fatal(err)
	}
	if err = manager.Close(); err != nil {
		t.Fatal(err)
	}
	if _, _, err = OpenExisting(ctx, path); !errors.Is(err, ErrSchemaVersion) {
		t.Fatalf("err=%v", err)
	}

	// A file SQLite can open but Binnacle never migrated is version 0.
	empty := filepath.Join(dir, "empty.db")
	if err = os.WriteFile(empty, nil, 0600); err != nil {
		t.Fatal(err)
	}
	if _, _, err = OpenExisting(ctx, empty); !errors.Is(err, ErrSchemaVersion) {
		t.Fatalf("empty database err=%v", err)
	}
}
