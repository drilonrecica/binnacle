// SPDX-License-Identifier: AGPL-3.0-only
package storage

import (
	"bytes"
	"context"
	"log/slog"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/drilonrecica/binnacle/internal/metrics"
)

// syncBuffer is a bytes.Buffer safe for a logger written from a worker.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}
func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func openObserved(t *testing.T) (*Manager, *syncBuffer) {
	t.Helper()
	dir := t.TempDir()
	m := New(filepath.Join(dir, "binnacle.db"), filepath.Join(dir, "run"))
	logs := &syncBuffer{}
	m.Logger = slog.New(slog.NewTextHandler(logs, nil))
	if err := m.Open(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := m.Close(); err != nil {
			t.Error(err)
		}
	})
	return m, logs
}

func dropTable(t *testing.T, m *Manager, table string) {
	t.Helper()
	if _, err := m.writer.Exec("DROP TABLE " + table); err != nil {
		t.Fatal(err)
	}
}

// RUN-7: a failed rollup pass is counted and timed, not only logged.
func TestRollupFailureIsCountedAndTimed(t *testing.T) {
	m, _ := openObserved(t)
	ctx := context.Background()
	if err := m.RollupOnce(ctx, time.Now()); err != nil {
		t.Fatal(err)
	}
	dropTable(t, m, "host_rollups_1m")
	if err := m.RollupOnce(ctx, time.Now()); err == nil {
		t.Fatal("rollup over a missing table succeeded")
	}
	h := m.Health()
	if h.RollupFailures != 1 || h.RollupLatency.Count != 2 {
		t.Fatalf("failures=%d latency=%+v, want 1 failure over 2 passes", h.RollupFailures, h.RollupLatency)
	}
}

// RUN-7: retention and vacuum passes are timed, and a failed retention pass
// is counted.
func TestRetentionFailureIsCountedAndPassesTimed(t *testing.T) {
	m, _ := openObserved(t)
	ctx := context.Background()
	policy := RetentionCutoffs{Raw: 24 * time.Hour, OneMinute: 7 * 24 * time.Hour, FifteenMinute: 30 * 24 * time.Hour, OneHour: 365 * 24 * time.Hour}
	if err := m.applyRetention(ctx, time.Now(), policy, false); err != nil {
		t.Fatal(err)
	}
	h := m.Health()
	if h.RetentionLatency.Count != 1 || h.VacuumLatency.Count != 1 || h.RetentionFailures != 0 {
		t.Fatalf("after a clean pass: %+v", h)
	}
	dropTable(t, m, "events")
	if err := m.applyRetention(ctx, time.Now(), policy, false); err == nil {
		t.Fatal("retention over a missing table succeeded")
	}
	if h := m.Health(); h.RetentionFailures != 1 || h.RetentionLatency.Count != 2 {
		t.Fatalf("after a failed pass: failures=%d latency=%+v", h.RetentionFailures, h.RetentionLatency)
	}
}

// RUN-7: a failed batch write is counted, timed and logged through the app
// logger, once per interval however often it repeats.
func TestPersistenceWriteFailureIsCountedTimedAndRateLimited(t *testing.T) {
	m, logs := openObserved(t)
	ctx := context.Background()
	dropTable(t, m, "host_samples_10s")
	engine := metrics.NewEngine(8)
	p := NewPersistence(engine, m, time.Hour, 4)
	value := 1.0
	for i := range 3 {
		at := time.Now().UTC().Add(time.Duration(i) * time.Second)
		engine.Publish(metrics.Snapshot{At: at, Host: metrics.HostObservation{At: at, CPUPercent: &value}})
		p.write(ctx, p.take(engine.PersistenceBatch()))
	}
	if got := p.WriteFailureCount(); got != 3 {
		t.Fatalf("write failures=%d, want 3", got)
	}
	if s := p.WriteLatencySummary(); s.Count != 3 || s.Max <= 0 {
		t.Fatalf("write latency=%+v", s)
	}
	if n := strings.Count(logs.String(), "persistence batch was not stored"); n != 1 {
		t.Fatalf("logged the write failure %d times, want 1 within the interval:\n%s", n, logs)
	}
}

// RUN-7: the deletion worker logs a failed job instead of discarding the
// error.
func TestDeletionWorkerLogsFailedJob(t *testing.T) {
	m, logs := openObserved(t)
	now := time.Now().UTC().UnixMilli()
	if _, err := m.writer.Exec("INSERT INTO history_deletion_jobs(id,kind,resource_id,before_ts,fence_ts,confirmation,state,requested_by,requested_at,total_rows) VALUES('job','all',NULL,NULL,?,'x','queued','test',?,0)", now, now); err != nil {
		t.Fatal(err)
	}
	dropTable(t, m, "container_instance_samples_10s")
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); m.runDeletionWorker(ctx) }()
	ok := waitFor(t, 2*time.Second, func() bool { return strings.Contains(logs.String(), "history deletion job failed") })
	cancel()
	<-done
	if !ok {
		t.Fatalf("failed deletion job was not logged:\n%s", logs)
	}
}
