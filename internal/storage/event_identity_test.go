// SPDX-License-Identifier: AGPL-3.0-only
package storage

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/drilonrecica/binnacle/internal/metrics"
)

func countEvents(t *testing.T, m *Manager, where string, args ...any) int {
	t.Helper()
	var n int
	if err := m.db.QueryRowContext(context.Background(), "SELECT COUNT(*) FROM events WHERE "+where, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// EVT-1: a restarted process starts a new engine whose first event used to
// get id 1 again and was dropped by INSERT OR IGNORE.
func TestEventsAfterRestartAreStored(t *testing.T) {
	ctx := context.Background()
	m := openTestManager(t)
	now := time.Now().UTC()
	for boot := range 2 {
		engine := metrics.NewEngine(8)
		engine.Publish(metrics.Snapshot{At: now.Add(time.Duration(boot) * time.Second)}, metrics.Event{At: now, Type: "container_restart", Message: "restarted"})
		if err := m.WriteBatch(ctx, engine.PersistenceBatch()); err != nil {
			t.Fatal(err)
		}
	}
	if n := countEvents(t, m, "type='container_restart'"); n != 2 {
		t.Fatalf("stored %d events, want 2", n)
	}
}

// EVT-2: storage events of the same type in the same millisecond used to
// share an id, so only the first was stored.
func TestStorageEventsNeverCollide(t *testing.T) {
	ctx := context.Background()
	m := openTestManager(t)
	now := time.Now()
	for range 2 {
		m.writeStorageEvent(ctx, now, "persistence_emergency", "critical", "paused")
		m.writeStorageEvent(ctx, now, "persistence_resumed", "info", "resumed")
	}
	if n := countEvents(t, m, "type IN ('persistence_emergency','persistence_resumed')"); n != 4 {
		t.Fatalf("stored %d storage events, want 4", n)
	}
}

// EVT-6: a new boot identity ends the previous boot session and records a
// host_reboot event dated at the kernel boot time; a process restart within
// the same boot does not.
func TestBootChangeRecordsHostReboot(t *testing.T) {
	ctx := context.Background()
	m := openTestManager(t)
	if err := m.UpsertHost(ctx, hostID, "identity", "Server"); err != nil {
		t.Fatal(err)
	}
	start := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
	uptime := 30.0
	write := func(manager *Manager, seq int, boot string, at time.Time) {
		t.Helper()
		snap := metrics.Snapshot{Sequence: metrics.Sequence(seq), At: at, BootIdentity: metrics.BootIdentity(boot), Host: metrics.HostObservation{At: at, UptimeSeconds: &uptime}}
		if err := manager.WriteBatch(ctx, metrics.PersistenceBatch{Snapshot: snap}); err != nil {
			t.Fatal(err)
		}
	}
	write(m, 1, "boot-a", start)
	write(m, 2, "boot-a", start.Add(10*time.Second))
	// A restart in the same boot: a fresh manager on the same database.
	restarted := New(m.Path, filepath.Join(t.TempDir(), "run"))
	if err := restarted.Open(ctx); err != nil {
		t.Fatal(err)
	}
	write(restarted, 1, "boot-a", start.Add(20*time.Second))
	if n := countEvents(t, m, "type='host_reboot'"); n != 0 {
		t.Fatalf("restart within one boot recorded %d reboots", n)
	}
	rebootAt := start.Add(10 * time.Minute)
	write(restarted, 2, "boot-b", rebootAt)
	restarted.Close()
	var ts int64
	if err := m.db.QueryRowContext(ctx, "SELECT ts FROM events WHERE type='host_reboot'").Scan(&ts); err != nil {
		t.Fatal(err)
	}
	if want := rebootAt.Add(-30 * time.Second).UnixMilli(); ts != want {
		t.Fatalf("host_reboot at %d, want boot time %d", ts, want)
	}
	var open, ended int
	if err := m.db.QueryRowContext(ctx, "SELECT COUNT(*) FILTER (WHERE ended_at IS NULL), COUNT(*) FILTER (WHERE ended_at IS NOT NULL) FROM boot_sessions WHERE host_id=?", hostID).Scan(&open, &ended); err != nil {
		t.Fatal(err)
	}
	if open != 1 || ended != 1 {
		t.Fatalf("boot sessions open=%d ended=%d, want 1 and 1", open, ended)
	}
	// The gap the reboot left is classified as host_reboot.
	gaps := []Gap{{From: start.Add(20 * time.Second), To: rebootAt}}
	if err := m.classifyGaps(ctx, MetricQuery{Scope: "host"}, gaps); err != nil {
		t.Fatal(err)
	}
	if gaps[0].Reason != "host_reboot" {
		t.Fatalf("reboot gap reason %q", gaps[0].Reason)
	}
}

// Without a host row the boot session cannot be stored; the batch must still
// be written and the boot retried later.
func TestBootWithoutHostRowKeepsBatch(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	m := New(filepath.Join(dir, "binnacle.db"), filepath.Join(dir, "run"))
	if err := m.Open(ctx); err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	now := time.Now().UTC()
	snap := metrics.Snapshot{Sequence: 1, At: now, BootIdentity: "boot-a", Host: metrics.HostObservation{At: now}}
	if err := m.WriteBatch(ctx, metrics.PersistenceBatch{Snapshot: snap}); err != nil {
		t.Fatal(err)
	}
	if m.lastBoot != "" {
		t.Fatalf("boot marked open without a host row")
	}
}

// EVT-6: lost batches leave their events for the next batch and one
// persistence_gap event spanning all of them.
func TestLostBatchesCarryEventsAndOneGapEvent(t *testing.T) {
	p := &Persistence{}
	base := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
	batch := func(offset time.Duration, types ...string) metrics.PersistenceBatch {
		b := metrics.PersistenceBatch{Snapshot: metrics.Snapshot{Sequence: 1, At: base.Add(offset)}}
		for _, kind := range types {
			b.Events = append(b.Events, metrics.Event{ID: metrics.NewEventID(), Type: kind})
		}
		return b
	}
	p.lose(queued{batch: batch(0, "a")}, gapEmergencyPause)
	first := p.take(batch(10*time.Second, "b"))
	// The batch carrying the gap is lost as well: its gap is merged, not
	// stored twice.
	p.lose(first, gapWriteFailed)
	item := p.take(batch(20*time.Second, "c"))
	var kinds []string
	for _, event := range item.batch.Events {
		kinds = append(kinds, event.Type)
	}
	if got := strings.Join(kinds, ","); got != "a,b,c,persistence_gap" {
		t.Fatalf("events=%s", got)
	}
	gap := item.batch.Events[3]
	if !gap.At.Equal(base) || item.gap.Batches != 2 || !item.gap.To.Equal(base.Add(10*time.Second)) {
		t.Fatalf("gap event at %s, gap %+v", gap.At, item.gap)
	}
	for _, want := range []string{`"batches":2`, gapEmergencyPause, gapWriteFailed} {
		if !strings.Contains(gap.Details, want) {
			t.Fatalf("gap details %s missing %s", gap.Details, want)
		}
	}
	if next := p.take(batch(30 * time.Second)); len(next.batch.Events) != 0 || next.gap != nil {
		t.Fatalf("carry not cleared: %+v", next)
	}
}

func TestCarriedEventsAreBoundedAndCounted(t *testing.T) {
	p := &Persistence{}
	b := metrics.PersistenceBatch{Snapshot: metrics.Snapshot{Sequence: 1, At: time.Now()}}
	for range maxCarriedEvents + 5 {
		b.Events = append(b.Events, metrics.Event{ID: metrics.NewEventID(), Type: "x"})
	}
	p.lose(queued{batch: b}, gapQueueFull)
	if got := p.EventsLostCount(); got != 5 {
		t.Fatalf("EventsLost=%d, want 5", got)
	}
	if item := p.take(b); len(item.batch.Events) != maxCarriedEvents+len(b.Events)+1 {
		t.Fatalf("carried %d events", len(item.batch.Events))
	}
}

// A dropped batch is reported by a persistence_gap event that the history
// API turns into a persistence_failure gap.
func TestEmergencyPauseRecordsPersistenceGap(t *testing.T) {
	ctx := context.Background()
	m := openTestManager(t)
	m.mu.Lock()
	m.emergencyPause = true
	m.mu.Unlock()
	engine := metrics.NewEngine(8)
	at := time.Now().UTC()
	engine.Publish(metrics.Snapshot{At: at}, metrics.Event{At: at, Type: "container_oom", Message: "oom"})
	worker := NewPersistence(engine, m, 10*time.Millisecond, 2)
	if err := worker.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer worker.Stop(ctx)
	if !waitFor(t, 2*time.Second, func() bool { return worker.DroppedCount() > 0 }) {
		t.Fatal("no batch dropped during the pause")
	}
	m.mu.Lock()
	m.emergencyPause = false
	m.mu.Unlock()
	if !waitFor(t, 2*time.Second, func() bool { return countEvents(t, m, "type='persistence_gap'") == 1 }) {
		t.Fatal("no persistence_gap event stored after the pause")
	}
	if n := countEvents(t, m, "type='container_oom'"); n != 1 {
		t.Fatalf("event of the paused batch stored %d times", n)
	}
	var ts int64
	if err := m.db.QueryRowContext(ctx, "SELECT ts FROM events WHERE type='persistence_gap'").Scan(&ts); err != nil {
		t.Fatal(err)
	}
	gaps := []Gap{{From: time.UnixMilli(ts).Add(-time.Second), To: time.UnixMilli(ts).Add(time.Minute)}}
	if err := m.classifyGaps(ctx, MetricQuery{Scope: "host"}, gaps); err != nil {
		t.Fatal(err)
	}
	if gaps[0].Reason != "persistence_failure" {
		t.Fatalf("reason %q", gaps[0].Reason)
	}
}

// EVT-6: the feed pages by ts and insertion order. Ids are compared as text
// no more, which ordered pre-ULID "10" before "9".
func TestEventFeedCursorUsesInsertionOrder(t *testing.T) {
	ctx := context.Background()
	m := openTestManager(t)
	now := time.Now().UTC().Truncate(time.Millisecond)
	ids := []string{"9", "10", metrics.NewEventID()}
	for _, id := range ids {
		if _, err := m.db.ExecContext(ctx, "INSERT INTO events(id,ts,type,severity,summary,source,created_at) VALUES(?,?,'deployment','info','x','docker',?)", id, now.UnixMilli(), now.UnixMilli()); err != nil {
			t.Fatal(err)
		}
	}
	var got []string
	before := ""
	for range len(ids) + 1 {
		page, err := m.QueryEvents(ctx, EventQuery{From: now.Add(-time.Minute), To: now.Add(time.Minute), Limit: 1, BeforeID: before})
		if err != nil {
			t.Fatal(err)
		}
		if len(page) == 0 {
			break
		}
		got = append(got, page[0].ID)
		before = page[0].ID
	}
	if want := ids[2] + "," + ids[1] + "," + ids[0]; strings.Join(got, ",") != want {
		t.Fatalf("pages %v, want newest insertion first %s", got, want)
	}
}

// EVT-6: a short range whose start is older than the raw retention is read
// from the finest tier that still has it.
func TestResolutionFollowsRetention(t *testing.T) {
	now := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
	r := RetentionCutoffs{Raw: 24 * time.Hour, OneMinute: 7 * 24 * time.Hour, FifteenMinute: 90 * 24 * time.Hour}
	cases := []struct {
		from, to time.Time
		r        RetentionCutoffs
		want     Resolution
	}{
		{now.Add(-time.Hour), now, r, ResolutionRaw},
		{now.Add(-48 * time.Hour), now.Add(-47 * time.Hour), r, Resolution1m},
		{now.Add(-30 * 24 * time.Hour), now.Add(-30*24*time.Hour + time.Hour), r, Resolution15m},
		{now.Add(-200 * 24 * time.Hour), now.Add(-200*24*time.Hour + time.Hour), r, Resolution1h},
		{now.Add(-48 * time.Hour), now.Add(-47 * time.Hour), RetentionCutoffs{}, ResolutionRaw},
		// The range length still sets the finest tier allowed.
		{now.Add(-20 * 24 * time.Hour), now, r, Resolution1h},
		// A tier shortened at emergency keeps half.
		{now.Add(-18 * time.Hour), now.Add(-17 * time.Hour), effectiveRetention(r, []string{"raw"}), Resolution1m},
	}
	for _, tc := range cases {
		if got := resolutionFor(tc.from, tc.to, now, tc.r); got != tc.want {
			t.Errorf("resolutionFor(%s..%s)=%s, want %s", tc.from, tc.to, got, tc.want)
		}
	}
}

func TestGapClassificationReportsQueryErrors(t *testing.T) {
	m := openTestManager(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	gaps := []Gap{{From: time.Now().Add(-time.Hour), To: time.Now()}}
	if err := m.classifyGaps(ctx, MetricQuery{Scope: "resource", ID: "res"}, gaps); err == nil {
		t.Fatal("classification error was swallowed")
	}
}
