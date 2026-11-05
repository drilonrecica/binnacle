// SPDX-License-Identifier: AGPL-3.0-only
package estimator

import (
	"context"
	"math"
	"path/filepath"
	"testing"
	"time"

	"github.com/drilonrecica/binnacle/internal/metrics"
	"github.com/drilonrecica/binnacle/internal/storage"
)

// End to end through SQLite: six hours of a disk filling at 1 GiB/h from a
// synthetic history give the projection the rows imply.
func TestRunReadsStoredHistory(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	store := storage.New(filepath.Join(dir, "db"), filepath.Join(dir, "run"))
	if err := store.Open(ctx); err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := store.UpsertHost(ctx, "host", "host-identity", "test"); err != nil {
		t.Fatal(err)
	}
	const capacity = 100 * gib
	for at := now.Add(-6 * time.Hour); !at.After(now); at = at.Add(10 * time.Second) {
		used := 60*gib - now.Sub(at).Hours()*gib
		if _, err := store.DB().ExecContext(ctx, `INSERT INTO filesystem_samples_1m(ts,host_id,mount_key,mount_point,used_bytes,available_bytes) VALUES(?,?,?,?,?,?)`, at.UnixMilli(), "host", "root", "/", int64(used), int64(capacity-used)); err != nil {
			t.Fatal(err)
		}
	}
	engine := metrics.NewEngine(4)
	engine.PublishFilesystems([]metrics.FilesystemObservation{{MountKey: "root", UsedBytes: i64(int64(60 * gib)), AvailableBytes: i64(int64(40 * gib))}})
	e := &Estimator{Engine: engine, Source: store, Now: func() time.Time { return now }}
	if err := e.Run(ctx); err != nil {
		t.Fatal(err)
	}
	got := engine.Snapshot().Filesystems[0].Trend
	if got.State != metrics.TrendFilling || got.DaysToFull == nil || math.Abs(*got.DaysToFull-40.0/24) > 0.02 || got.Confidence < 0.95 {
		t.Fatalf("trend=%+v", got)
	}
}
