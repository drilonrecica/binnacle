// SPDX-License-Identifier: AGPL-3.0-only
package storage

import (
	"context"
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func TestDatabaseFilesArePrivateRegardlessOfUmask(t *testing.T) {
	old := syscall.Umask(0o022)
	t.Cleanup(func() { syscall.Umask(old) })
	dir := t.TempDir()
	path := filepath.Join(dir, "binnacle.db")
	manager := New(path, filepath.Join(dir, "run"))
	if err := manager.Open(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = manager.Close() })
	if _, err := manager.DB().Exec("CREATE TABLE IF NOT EXISTS mode_probe(id INTEGER)"); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{path, path + "-wal"} {
		info, err := os.Stat(name)
		if err != nil {
			t.Fatal(err)
		}
		if mode := info.Mode().Perm(); mode != 0o600 {
			t.Fatalf("%s mode=%o, want 600", filepath.Base(name), mode)
		}
	}
}

func TestExistingWorldReadableDatabaseIsTightened(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "binnacle.db")
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	manager := New(path, filepath.Join(dir, "run"))
	if err := manager.Open(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = manager.Close() })
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if mode := info.Mode().Perm(); mode != 0o600 {
		t.Fatalf("database mode=%o, want 600", mode)
	}
}
