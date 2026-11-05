// SPDX-License-Identifier: AGPL-3.0-only
package storage

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/drilonrecica/binnacle/internal/metrics"
)

func TestPersistenceSchedulesCurrentSnapshot(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	manager := New(filepath.Join(dir, "binnacle.db"), filepath.Join(dir, "run"))
	if err := manager.Open(ctx); err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	engine := metrics.NewEngine(8)
	value := 12.5
	at := time.Now().UTC()
	engine.Publish(metrics.Snapshot{At: at, Host: metrics.HostObservation{At: at, CPUPercent: &value}})
	worker := NewPersistence(engine, manager, 10*time.Millisecond, 2)
	if err := worker.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer worker.Stop(ctx)
	deadline := time.Now().Add(time.Second)
	for {
		var count int
		if err := manager.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM host_samples_10s").Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count > 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("snapshot was not persisted")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// TestPersistenceStopDrainsQueue holds the single writer connection so that
// batches pile up in the queue, then stops persistence and releases the
// writer. Stop must write every queued batch before it returns, instead of
// cancelling the writes the way a cancelled start context used to (RUN-1).
func TestPersistenceStopDrainsQueue(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	manager := New(filepath.Join(dir, "binnacle.db"), filepath.Join(dir, "run"))
	if err := manager.Open(ctx); err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	hold, err := manager.Writer().BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	engine := metrics.NewEngine(8)
	value := 1.0
	base := time.Now().UTC().Truncate(time.Second).Add(-time.Hour)
	publish := func(i int) {
		at := base.Add(time.Duration(i) * 10 * time.Second)
		engine.Publish(metrics.Snapshot{At: at, Host: metrics.HostObservation{At: at, CPUPercent: &value}})
	}
	publish(0)
	worker := NewPersistence(engine, manager, 15*time.Millisecond, 16)
	// The start context is cancelled first, as a signal context is; the
	// drain must not depend on it.
	startCtx, cancelStart := context.WithCancel(ctx)
	if err := worker.Start(startCtx); err != nil {
		t.Fatal(err)
	}
	for i := 1; i < 6; i++ {
		time.Sleep(20 * time.Millisecond)
		publish(i)
	}
	deadline := time.Now().Add(2 * time.Second)
	for worker.Queue() < 3 {
		if time.Now().After(deadline) {
			t.Fatalf("queue depth %d, want at least 3 waiting batches", worker.Queue())
		}
		time.Sleep(5 * time.Millisecond)
	}
	cancelStart()
	stopped := make(chan error, 1)
	stopCtx, cancelStop := context.WithTimeout(ctx, PersistenceStopBudget)
	defer cancelStop()
	go func() { stopped <- worker.Stop(stopCtx) }()
	time.Sleep(50 * time.Millisecond)
	if err := hold.Rollback(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-stopped:
		if err != nil {
			t.Fatalf("Stop: %v", err)
		}
	case <-time.After(PersistenceStopBudget + time.Second):
		t.Fatal("Stop did not return")
	}
	if dropped := worker.DroppedCount(); dropped != 0 {
		t.Fatalf("dropped %d batches during a drain that had time to finish", dropped)
	}
	var count int
	if err := manager.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM host_samples_10s").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count < 3 {
		t.Fatalf("stored %d host samples, want the queued batches (at least 3) written before Stop returned", count)
	}
}

// TestPersistenceStopHonoursDeadline keeps the writer blocked past the stop
// deadline: Stop returns at the deadline and counts the unwritten batches.
func TestPersistenceStopHonoursDeadline(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	manager := New(filepath.Join(dir, "binnacle.db"), filepath.Join(dir, "run"))
	if err := manager.Open(ctx); err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	hold, err := manager.Writer().BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer hold.Rollback()
	engine := metrics.NewEngine(8)
	value := 1.0
	at := time.Now().UTC()
	engine.Publish(metrics.Snapshot{At: at, Host: metrics.HostObservation{At: at, CPUPercent: &value}})
	worker := NewPersistence(engine, manager, 10*time.Millisecond, 4)
	if err := worker.Start(ctx); err != nil {
		t.Fatal(err)
	}
	time.Sleep(50 * time.Millisecond)
	stopCtx, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
	defer cancel()
	started := time.Now()
	if err := worker.Stop(stopCtx); err == nil {
		t.Fatal("Stop returned nil although the drain hit its deadline")
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("Stop took %s, want it bounded by its 100ms deadline", elapsed)
	}
	if worker.DroppedCount() == 0 {
		t.Fatal("batches left at the deadline were not counted as dropped")
	}
}
