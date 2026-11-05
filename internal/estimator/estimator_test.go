// SPDX-License-Identifier: AGPL-3.0-only
package estimator

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/drilonrecica/binnacle/internal/metrics"
	"github.com/drilonrecica/binnacle/internal/storage"
)

type fakeSource struct {
	fs     map[string][]storage.UsageSample
	memory []storage.UsageSample
	fail   string
	from   time.Time
	calls  atomic.Int32
}

func (f *fakeSource) FilesystemUsage(_ context.Context, key string, from, _ time.Time) ([]storage.UsageSample, error) {
	f.calls.Add(1)
	f.from = from
	if key == f.fail {
		return nil, errors.New("disk read failed")
	}
	return f.fs[key], nil
}

func (f *fakeSource) MemoryUsage(context.Context, time.Time, time.Time) ([]storage.UsageSample, error) {
	return f.memory, nil
}

func i64(v int64) *int64 { return &v }

func publish(engine *metrics.Engine) {
	engine.PublishFilesystems([]metrics.FilesystemObservation{
		{MountKey: "root", UsedBytes: i64(int64(60 * gib)), AvailableBytes: i64(int64(40 * gib))},
		{MountKey: "broken", UsedBytes: i64(1), AvailableBytes: i64(1)},
		{MountKey: "fresh", UsedBytes: i64(1), AvailableBytes: i64(1)},
	})
	engine.Publish(metrics.Snapshot{At: now, Host: metrics.HostObservation{MemoryUsedBytes: i64(int64(4 * gib)), MemoryTotalBytes: i64(int64(16 * gib))}})
}

func TestRunPublishesTrends(t *testing.T) {
	engine := metrics.NewEngine(4)
	publish(engine)
	source := &fakeSource{
		fs: map[string][]storage.UsageSample{
			"root": history(24*time.Hour, func(h float64) float64 { return 60*gib - h*gib }),
		},
		memory: history(24*time.Hour, func(float64) float64 { return 4 * gib }),
		fail:   "broken",
	}
	window := 3 // out of range: the default applies
	e := &Estimator{Engine: engine, Source: source, Now: func() time.Time { return now }, WindowHours: func() int { return window }}
	err := e.Run(context.Background())
	if err == nil || !strings.Contains(err.Error(), "broken") {
		t.Fatalf("err=%v, want the failed mount reported", err)
	}
	if !source.from.Equal(now.Add(-24 * time.Hour)) {
		t.Fatalf("history from=%v, want the 24 h default window", source.from)
	}
	snap := engine.Snapshot()
	byKey := map[string]metrics.Trend{}
	for _, fs := range snap.Filesystems {
		byKey[fs.MountKey] = fs.Trend
	}
	if got := byKey["root"]; got.State != metrics.TrendFilling || got.DaysToFull == nil || got.WindowHours != 24 {
		t.Fatalf("root=%+v", got)
	}
	for _, key := range []string{"broken", "fresh"} {
		if got := byKey[key]; got.State != metrics.TrendUnknown || got.Confidence != 0 {
			t.Fatalf("%s=%+v, want unknown", key, got)
		}
	}
	if got := snap.Host.MemoryTrend; got.State != metrics.TrendSteady {
		t.Fatalf("memory=%+v", got)
	}

	window = 72
	if err := e.Run(context.Background()); err == nil {
		t.Fatal("broken mount stopped failing")
	}
	if !source.from.Equal(now.Add(-72*time.Hour)) || engine.Snapshot().Host.MemoryTrend.WindowHours != 72 {
		t.Fatalf("window not applied: from=%v memory=%+v", source.from, engine.Snapshot().Host.MemoryTrend)
	}
}

// Without the memory total there is nothing to fill: memory stays unknown.
func TestRunWithoutMemoryTotal(t *testing.T) {
	engine := metrics.NewEngine(4)
	engine.Publish(metrics.Snapshot{At: now})
	e := &Estimator{Engine: engine, Source: &fakeSource{memory: history(24*time.Hour, func(float64) float64 { return gib })}, Now: func() time.Time { return now }}
	if err := e.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := engine.Snapshot().Host.MemoryTrend; got.State != metrics.TrendUnknown {
		t.Fatalf("memory=%+v", got)
	}
}

// The loop waits for its delay, drops a wake sent before Start, runs on Wake,
// and stops with its context.
func TestLoopRunsOnDelayAndWake(t *testing.T) {
	engine := metrics.NewEngine(4)
	publish(engine)
	source := &fakeSource{}
	e := &Estimator{Engine: engine, Source: source, Delay: time.Hour, Interval: time.Hour}
	e.Wake()
	if err := e.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer e.Stop(context.Background())
	time.Sleep(50 * time.Millisecond)
	if n := source.calls.Load(); n != 0 {
		t.Fatalf("ran %d times before the delay", n)
	}
	e.Wake()
	deadline := time.Now().Add(2 * time.Second)
	for source.calls.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if source.calls.Load() == 0 {
		t.Fatal("Wake did not run the estimator")
	}
	if err := e.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := (&Estimator{}).Start(context.Background()); err == nil {
		t.Fatal("started without dependencies")
	}
}
