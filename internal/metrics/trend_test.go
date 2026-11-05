// SPDX-License-Identifier: AGPL-3.0-only

package metrics

import (
	"testing"
	"time"
)

// P3-12: the engine attaches the latest estimates to every snapshot, keeps
// them across publishes, and reports unknown for a mount it has none for.
func TestEngineAttachesTrends(t *testing.T) {
	engine := NewEngine(4)
	used := int64(1)
	engine.PublishFilesystems([]FilesystemObservation{{MountKey: "root", UsedBytes: &used}, {MountKey: "new"}})
	engine.Publish(Snapshot{At: time.Now()})
	snap := engine.Snapshot()
	if snap.Host.MemoryTrend.State != TrendUnknown || snap.Host.MemoryTrend.WindowHours != DefaultTrendWindowHours {
		t.Fatalf("memory before estimates=%+v", snap.Host.MemoryTrend)
	}
	if got := snap.Filesystems[0].Trend; got.State != TrendUnknown {
		t.Fatalf("filesystem before estimates=%+v", got)
	}

	days := 3.5
	engine.SetTrends(48, Trend{State: TrendSteady, Confidence: 0.9, WindowHours: 48}, map[string]Trend{"root": {State: TrendFilling, DaysToFull: &days, Confidence: 0.8, WindowHours: 48}})
	snap = engine.Snapshot()
	if snap.Host.MemoryTrend.State != TrendSteady || snap.Filesystems[0].Trend.State != TrendFilling || *snap.Filesystems[0].Trend.DaysToFull != 3.5 {
		t.Fatalf("after SetTrends: memory=%+v fs=%+v", snap.Host.MemoryTrend, snap.Filesystems[0].Trend)
	}
	if got := snap.Filesystems[1].Trend; got.State != TrendUnknown || got.WindowHours != 48 {
		t.Fatalf("mount without an estimate=%+v", got)
	}

	// Collectors publish observations without trends; the engine keeps them.
	engine.PublishFilesystems([]FilesystemObservation{{MountKey: "root", UsedBytes: &used}})
	engine.Publish(Snapshot{At: time.Now()})
	snap = engine.Snapshot()
	if snap.Host.MemoryTrend.State != TrendSteady || snap.Filesystems[0].Trend.State != TrendFilling {
		t.Fatalf("after publish: memory=%+v fs=%+v", snap.Host.MemoryTrend, snap.Filesystems[0].Trend)
	}
	sub := engine.Subscribe()
	defer sub.Close()
	if live := <-sub.Snapshots; live.Filesystems[0].Trend.State != TrendFilling || live.Host.MemoryTrend.State != TrendSteady {
		t.Fatalf("live snapshot lacks trends: %+v", live)
	}
}
