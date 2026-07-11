// SPDX-License-Identifier: AGPL-3.0-only
package storage

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestScopedDeletionPreservesConfigurationAndSupportsRetry(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	m := New(filepath.Join(dir, "talos.db"), filepath.Join(dir, "run"))
	if err := m.Open(ctx); err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	now := time.Now().UTC()
	old := now.Add(-2 * time.Hour).UnixMilli()
	fresh := now.UnixMilli()
	if _, err := m.db.ExecContext(ctx, "INSERT INTO users(id,username,password_hash,created_at,updated_at) VALUES('usr_test','admin','hash',?,?)", fresh, fresh); err != nil {
		t.Fatal(err)
	}
	if _, err := m.db.ExecContext(ctx, "INSERT INTO settings(key,value_json,updated_at) VALUES('retention','{}',?)", fresh); err != nil {
		t.Fatal(err)
	}
	if _, err := m.db.ExecContext(ctx, "INSERT INTO host_samples_10s(ts,host_id,cpu_busy_pct) VALUES(?,'host',1),(?,'host',2)", old, fresh); err != nil {
		t.Fatal(err)
	}
	preview, err := m.PreviewDeletion(ctx, DeletionRequest{Kind: DeleteBefore, Before: now.Add(-time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	if preview.TotalRows != 1 || preview.Scope.Kind != DeleteBefore {
		t.Fatalf("preview=%+v", preview)
	}
	if _, err = m.CreateDeletion(ctx, preview.Token, "wrong", "usr_test"); err == nil {
		t.Fatal("wrong confirmation accepted")
	}
	job, err := m.CreateDeletion(ctx, preview.Token, preview.Confirmation, "usr_test")
	if err != nil {
		t.Fatal(err)
	}
	if err = m.RunDeletion(ctx, job.ID); err != nil {
		t.Fatal(err)
	}
	var samples, users, settings, actor int
	_ = m.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM host_samples_10s").Scan(&samples)
	_ = m.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM users").Scan(&users)
	_ = m.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM settings").Scan(&settings)
	_ = m.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM history_deletion_jobs WHERE requested_by='usr_test'").Scan(&actor)
	if samples != 1 || users != 1 || settings != 1 || actor != 1 {
		t.Fatalf("samples=%d users=%d settings=%d actor=%d", samples, users, settings, actor)
	}
	second, err := m.PreviewDeletion(ctx, DeletionRequest{Kind: DeleteAll})
	if err != nil {
		t.Fatal(err)
	}
	retry, err := m.CreateDeletion(ctx, second.Token, second.Confirmation, "usr_test")
	if err != nil {
		t.Fatal(err)
	}
	if err = m.CancelDeletion(ctx, retry.ID); err != nil {
		t.Fatal(err)
	}
	if err = m.RetryDeletion(ctx, retry.ID); err != nil {
		t.Fatal(err)
	}
}
