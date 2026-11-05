// SPDX-License-Identifier: AGPL-3.0-only
package alerts_test

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/drilonrecica/binnacle/internal/alerts"
	"github.com/drilonrecica/binnacle/internal/metrics"
	"github.com/drilonrecica/binnacle/internal/storage"
)

// failHostCPU makes every write of the host CPU family's state fail, the way a
// broken rule or a storage error would.
func failHostCPU(t *testing.T, store *storage.Manager) {
	t.Helper()
	if _, err := store.DB().Exec(`CREATE TRIGGER fail_host_cpu BEFORE INSERT ON alert_evaluation_state WHEN NEW.dedup_key LIKE 'host_cpu_warning:%' BEGIN SELECT RAISE(ABORT,'injected host cpu failure'); END`); err != nil {
		t.Fatal(err)
	}
}

func persistenceFailing(t *testing.T, store *storage.Manager, at time.Time) {
	t.Helper()
	if _, err := store.DB().Exec(`INSERT INTO events(id,ts,type,severity,summary,source,created_at)VALUES('persistence-test',?,'persistence_emergency','critical','Persistence stopped','storage',?)`, at.UnixMilli(), at.UnixMilli()); err != nil {
		t.Fatal(err)
	}
}

// EVT-5: an error in one alert family is reported and the other families are
// still evaluated. Before the fix the first error returned from Evaluate and
// every later family, here persistence, was skipped on every tick.
func TestEvaluatorEvaluatesFamiliesIndependently(t *testing.T) {
	ctx := context.Background()
	store, repo := healthStore(t)
	engine := metrics.NewEngine(4)
	now := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
	evaluator := alerts.NewEvaluator(repo, engine)
	evaluator.Now = func() time.Time { return now }
	failHostCPU(t, store)
	persistenceFailing(t, store, now.Add(-time.Second))
	hostCPU(engine, 95)

	err := evaluator.Evaluate(ctx)
	if err == nil || !strings.Contains(err.Error(), "host_cpu_warning") || !strings.Contains(err.Error(), "injected host cpu failure") {
		t.Fatalf("Evaluate error=%v, want the host CPU failure", err)
	}
	var phase string
	if err := store.DB().QueryRowContext(ctx, `SELECT phase FROM alert_evaluation_state WHERE dedup_key='persistence_failure:persistence'`).Scan(&phase); err != nil {
		t.Fatalf("persistence family was not evaluated after the host CPU failure: %v", err)
	}
	if phase == "healthy" {
		t.Fatalf("persistence phase=%s, want it to leave healthy", phase)
	}
}

type lockedBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.b.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.b.String()
}

// EVT-5: the background loop logs evaluation errors instead of discarding them.
func TestEvaluatorLogsErrors(t *testing.T) {
	store, repo := healthStore(t)
	engine := metrics.NewEngine(4)
	var logs lockedBuffer
	evaluator := alerts.NewEvaluator(repo, engine)
	evaluator.Interval = 10 * time.Millisecond
	evaluator.Logger = slog.New(slog.NewTextHandler(&logs, nil))
	failHostCPU(t, store)
	hostCPU(engine, 95)
	if err := evaluator.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer evaluator.Stop(context.Background())
	deadline := time.Now().Add(5 * time.Second)
	for !strings.Contains(logs.String(), "injected host cpu failure") {
		if time.Now().After(deadline) {
			t.Fatalf("evaluation error was not logged; logs=%q", logs.String())
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !strings.Contains(logs.String(), "alert evaluation failed") {
		t.Fatalf("logs=%q", logs.String())
	}
}
