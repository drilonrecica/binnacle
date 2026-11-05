// SPDX-License-Identifier: AGPL-3.0-only
package storage

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// waitFor polls until condition is true or the deadline passes.
func waitFor(t *testing.T, d time.Duration, condition func() bool) bool {
	t.Helper()
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if condition() {
			return true
		}
		time.Sleep(20 * time.Millisecond)
	}
	return condition()
}

// STO-6: the app sets the retention policy after Start, so a worker whose
// first pass runs at boot with the zero policy and then sleeps an hour never
// deletes anything on a process that restarts hourly. SetRetention must wake
// the worker.
func TestSetRetentionWakesTheWorkerWithoutWaitingForTheHourlyTick(t *testing.T) {
	ctx := context.Background()
	m := openTestManager(t)
	m.retentionBootDelay = time.Hour
	if err := m.Start(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = m.Stop(ctx) })
	// Wait for the boot rollup pass so the 1h watermark is at the current hour.
	if !waitFor(t, 5*time.Second, func() bool { return m.Health().RollupDuration > 0 }) {
		t.Fatal("boot rollup pass did not run")
	}
	insertHostSamplesEveryMinute(t, m, rollupBase, rollupBase.Add(time.Hour), 1)
	m.SetRetention(RetentionCutoffs{Raw: time.Hour})
	if !waitFor(t, 5*time.Second, func() bool { return rawHostCount(t, m, time.Time{}) == 0 }) {
		t.Fatalf("raw rows after SetRetention=%d want 0", rawHostCount(t, m, time.Time{}))
	}
}

// insertHostSamplesEvery writes one host sample per step in [from, until) in
// one transaction, with a boot session id so the rows carry real payload.
func insertHostSamplesEvery(t *testing.T, m *Manager, from, until time.Time, step time.Duration) {
	t.Helper()
	tx, err := m.writer.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	for at := from; at.Before(until); at = at.Add(step) {
		if _, err := tx.ExecContext(context.Background(), "INSERT INTO host_samples_10s(ts,host_id,boot_session_id,cpu_busy_pct,load_1,memory_used_bytes,network_rx_bps,network_tx_bps) VALUES(?,'host','boot-0123456789abcdef0123456789abcdef',12.5,0.75,1234567890,1000.5,2000.25)", at.UnixMilli()); err != nil {
			t.Fatal(err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
}

func countRows(t *testing.T, m *Manager, table string, where string, args ...any) int {
	t.Helper()
	var n int
	query := "SELECT COUNT(*) FROM " + table
	if where != "" {
		query += " WHERE " + where
	}
	if err := m.db.QueryRowContext(context.Background(), query, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func checkpoint(t *testing.T, m *Manager) {
	t.Helper()
	if _, err := m.writer.Exec("PRAGMA wal_checkpoint(TRUNCATE)"); err != nil {
		t.Fatal(err)
	}
}

func mainFileSize(t *testing.T, m *Manager) int64 {
	t.Helper()
	info, err := os.Stat(m.Path)
	if err != nil {
		t.Fatal(err)
	}
	return info.Size()
}

// STO-5 / ADR 021 §7: the budget compares live data, (page_count -
// freelist_count) * page_size, not the size of the files, so deleted rows
// free budget before the space is reclaimed and the WAL cannot latch the
// emergency pause.
func TestBudgetMeasuresLivePagesNotFileSize(t *testing.T) {
	ctx := context.Background()
	m := openTestManager(t)
	insertHostSamplesEvery(t, m, rollupBase, rollupBase.Add(8*time.Hour), 10*time.Second)
	checkpoint(t, m)
	before, err := m.liveBytes(ctx)
	if err != nil {
		t.Fatal(err)
	}
	fileBefore := mainFileSize(t, m)
	if _, err := m.db.ExecContext(ctx, "DELETE FROM host_samples_10s"); err != nil {
		t.Fatal(err)
	}
	checkpoint(t, m)
	after, err := m.liveBytes(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if fileAfter := mainFileSize(t, m); fileAfter < fileBefore {
		t.Fatalf("main file shrank on DELETE: %d -> %d; the test premise is wrong", fileBefore, fileAfter)
	}
	if after >= before*3/4 {
		t.Fatalf("live bytes before=%d after=%d; deleted rows still count toward the budget", before, after)
	}
}

// ADR 021 §7: when the disk is nearly full the budget state is emergency
// regardless of the configured target, because SQLite needs disk to recover.
func TestFreeDiskFloorForcesEmergency(t *testing.T) {
	ctx := context.Background()
	m := openTestManager(t)
	m.SetBudgetTarget(1 << 40)
	m.freeDisk = func(string) (uint64, error) { return MinimumFreeSpace - 1, nil }
	m.evaluateAt(ctx, rollupBase, false)
	if b := m.Budget(); b.State != BudgetEmergency || !m.EmergencyPause() {
		t.Fatalf("budget=%+v paused=%v", b, m.EmergencyPause())
	}
}

// seedEightHours writes eight hours of host samples at the given cadence,
// rolls them up with now = rollupBase + 9h and returns that now.
func seedEightHours(t *testing.T, m *Manager, step time.Duration) time.Time {
	t.Helper()
	insertHostSamplesEvery(t, m, rollupBase, rollupBase.Add(8*time.Hour), step)
	now := rollupBase.Add(9 * time.Hour)
	if err := m.RollupOnce(context.Background(), now); err != nil {
		t.Fatal(err)
	}
	return now
}

func eventCount(t *testing.T, m *Manager, eventType string) int {
	return countRows(t, m, "events", "type=?", eventType)
}

// STO-5 / ADR 021 §7: at emergency the pass shortens retention, oldest raw
// rows down to half the raw retention first, then the 1m tier, and pauses
// raw persistence only when both steps left the budget at emergency.
func TestEmergencyShortensRawThenOneMinuteBeforePausing(t *testing.T) {
	ctx := context.Background()
	m := openTestManager(t)
	now := seedEightHours(t, m, time.Minute)
	m.SetRetention(RetentionCutoffs{Raw: 8 * time.Hour, OneMinute: 8 * time.Hour})
	m.SetBudgetTarget(1) // every byte is over budget, so nothing short of pausing helps
	m.evaluateAt(ctx, now, true)

	// Raw and 1m are kept from now-4h = rollupBase+5h; the 15m and 1h tiers
	// are never shortened.
	half := rollupBase.Add(5 * time.Hour)
	if got := rawHostCount(t, m, time.Time{}); got != 180 || rawHostCount(t, m, half) != 180 {
		t.Fatalf("raw rows=%d from +5h=%d want 180/180", got, rawHostCount(t, m, half))
	}
	if got := countRows(t, m, "host_rollups_1m", ""); got != 180 {
		t.Fatalf("1m rows=%d want 180", got)
	}
	if got := countRows(t, m, "host_rollups_15m", ""); got != 32 {
		t.Fatalf("15m rows=%d want 32", got)
	}
	b := m.Budget()
	if !b.Paused || !m.EmergencyPause() || len(b.Shortened) != 2 || b.Shortened[0] != "raw" || b.Shortened[1] != "1m" {
		t.Fatalf("budget=%+v", b)
	}
	if eventCount(t, m, "retention_shortened") == 0 || eventCount(t, m, "persistence_emergency") != 1 {
		t.Fatalf("shortened=%d emergency=%d", eventCount(t, m, "retention_shortened"), eventCount(t, m, "persistence_emergency"))
	}
}

// ADR 021 §7: shortening stops as soon as the budget leaves emergency; the
// 1m tier is untouched and persistence is not paused.
func TestEmergencyStopsShorteningOnceBelowEmergency(t *testing.T) {
	ctx := context.Background()
	m := openTestManager(t)
	now := seedEightHours(t, m, 10*time.Second)
	m.SetRetention(RetentionCutoffs{Raw: 8 * time.Hour, OneMinute: 8 * time.Hour})
	used, err := m.liveBytes(ctx)
	if err != nil {
		t.Fatal(err)
	}
	m.SetBudgetTarget(used * 100 / 99) // 99 % used: emergency
	m.evaluateAt(ctx, now, true)
	if got := rawHostCount(t, m, time.Time{}); got != 3*360 {
		t.Fatalf("raw rows=%d want %d (raw shortened to the last 3 complete hours)", got, 3*360)
	}
	// The normal 1m cutoff (now-8h, aligned) removes the first hour only.
	if got := countRows(t, m, "host_rollups_1m", ""); got != 420 {
		t.Fatalf("1m rows=%d want 420", got)
	}
	b := m.Budget()
	if b.Paused || m.EmergencyPause() || len(b.Shortened) != 1 || b.Shortened[0] != "raw" || b.State == BudgetEmergency {
		t.Fatalf("budget=%+v paused=%v", b, m.EmergencyPause())
	}
	if eventCount(t, m, "persistence_emergency") != 0 {
		t.Fatal("persistence was paused although shortening raw retention was enough")
	}
}

// STO-5: the pause clears below the critical threshold, not below emergency,
// so the state cannot flap around 98 %.
func TestEmergencyPauseClearsOnlyBelowCritical(t *testing.T) {
	ctx := context.Background()
	m := openTestManager(t)
	now := seedEightHours(t, m, time.Minute)
	m.SetBudgetTarget(1)
	m.evaluateAt(ctx, now, false)
	if !m.EmergencyPause() {
		t.Fatal("not paused at emergency")
	}
	used := m.Budget().UsedBytes
	m.SetBudgetTarget(used * 100 / 96) // 96 %: below emergency, above critical
	m.evaluateAt(ctx, now.Add(time.Minute), false)
	if b := m.Budget(); b.State != BudgetCritical || !m.EmergencyPause() {
		t.Fatalf("budget=%+v paused=%v; pause must hold between critical and emergency", b, m.EmergencyPause())
	}
	m.SetBudgetTarget(used * 2)
	m.evaluateAt(ctx, now.Add(2*time.Minute), false)
	if b := m.Budget(); b.State != BudgetOK || m.EmergencyPause() || b.Paused {
		t.Fatalf("budget=%+v paused=%v", b, m.EmergencyPause())
	}
	if eventCount(t, m, "persistence_resumed") != 1 {
		t.Fatalf("resumed events=%d", eventCount(t, m, "persistence_resumed"))
	}
}

// ADR 021 §5: a pass has a total duration budget; when it is exhausted the
// pass stops, reports it, and the next pass resumes the work.
func TestRetentionPassStopsAtItsBudgetAndReports(t *testing.T) {
	ctx := context.Background()
	m := openTestManager(t)
	now := seedEightHours(t, m, 10*time.Second)
	policy := RetentionCutoffs{Raw: time.Hour}
	m.retentionPassBudget = time.Nanosecond
	if err := m.applyRetention(ctx, now, policy, false); err != nil {
		t.Fatal(err)
	}
	if !m.Health().RetentionIncomplete || rawHostCount(t, m, time.Time{}) == 0 {
		t.Fatalf("incomplete=%v rows=%d; the pass did not stop at its budget", m.Health().RetentionIncomplete, rawHostCount(t, m, time.Time{}))
	}
	m.retentionPassBudget = 0
	if err := m.applyRetention(ctx, now, policy, false); err != nil {
		t.Fatal(err)
	}
	if m.Health().RetentionIncomplete || rawHostCount(t, m, time.Time{}) != 0 {
		t.Fatalf("incomplete=%v rows=%d after the resumed pass", m.Health().RetentionIncomplete, rawHostCount(t, m, time.Time{}))
	}
}

func pageCounts(t *testing.T, m *Manager) (pages, freelist int) {
	t.Helper()
	if err := m.db.QueryRowContext(context.Background(), "PRAGMA page_count").Scan(&pages); err != nil {
		t.Fatal(err)
	}
	if err := m.db.QueryRowContext(context.Background(), "PRAGMA freelist_count").Scan(&freelist); err != nil {
		t.Fatal(err)
	}
	return pages, freelist
}

// STO-5 / ADR 021 §7: space is reclaimed with incremental vacuum in batches
// and the pass ends with wal_checkpoint(TRUNCATE); a full VACUUM never runs.
func TestRetentionReclaimsFreelistAndTruncatesTheWAL(t *testing.T) {
	ctx := context.Background()
	m := openTestManager(t)
	now := seedEightHours(t, m, 10*time.Second)
	checkpoint(t, m)
	before := mainFileSize(t, m)
	if err := m.applyRetention(ctx, now, RetentionCutoffs{Raw: time.Hour}, false); err != nil {
		t.Fatal(err)
	}
	pages, freelist := pageCounts(t, m)
	if pages == 0 || float64(freelist)/float64(pages) >= 0.10 {
		t.Fatalf("pages=%d freelist=%d; the freelist was not reclaimed", pages, freelist)
	}
	if after := mainFileSize(t, m); after >= before {
		t.Fatalf("main file %d -> %d did not shrink", before, after)
	}
	if info, err := os.Stat(m.Path + "-wal"); err == nil && info.Size() != 0 {
		t.Fatalf("WAL is %d bytes after retention; it must be truncated", info.Size())
	}
	if m.Health().VacuumDuration <= 0 {
		t.Fatal("vacuum duration not reported")
	}
}

func autoVacuumMode(t *testing.T, path string) int {
	t.Helper()
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var mode int
	if err := db.QueryRow("PRAGMA auto_vacuum").Scan(&mode); err != nil {
		t.Fatal(err)
	}
	return mode
}

// legacyDatabase creates a database the way releases before ADR 021 did:
// auto_vacuum=none with a populated table.
func legacyDatabase(t *testing.T) (path, runtimeDir string) {
	t.Helper()
	dir := t.TempDir()
	path = filepath.Join(dir, "binnacle.db")
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("CREATE TABLE legacy(x TEXT); INSERT INTO legacy VALUES('kept')"); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if mode := autoVacuumMode(t, path); mode != 0 {
		t.Fatalf("legacy auto_vacuum=%d want 0", mode)
	}
	return path, filepath.Join(dir, "run")
}

// ADR 021 §8: an existing database is converted to incremental auto-vacuum
// once at start, when the disk has room for the copy, and keeps its data.
func TestOpenConvertsExistingDatabaseToIncrementalAutoVacuum(t *testing.T) {
	ctx := context.Background()
	path, runtimeDir := legacyDatabase(t)
	m := New(path, runtimeDir)
	if err := m.Open(ctx); err != nil {
		t.Fatal(err)
	}
	var kept string
	if err := m.db.QueryRowContext(ctx, "SELECT x FROM legacy").Scan(&kept); err != nil || kept != "kept" {
		t.Fatalf("legacy row=%q err=%v", kept, err)
	}
	if m.Health().VacuumPending {
		t.Fatal("vacuum reported pending after a conversion")
	}
	if err := m.Close(); err != nil {
		t.Fatal(err)
	}
	if mode := autoVacuumMode(t, path); mode != 2 {
		t.Fatalf("auto_vacuum=%d want 2 (incremental)", mode)
	}
}

// ADR 021 §8: without room for the copy the conversion is skipped, reported
// as pending, and retried at the next start.
func TestOpenReportsPendingVacuumWhenDiskIsTight(t *testing.T) {
	ctx := context.Background()
	path, runtimeDir := legacyDatabase(t)
	m := New(path, runtimeDir)
	m.freeDisk = func(string) (uint64, error) { return MinimumFreeSpace + 1024, nil }
	if err := m.Open(ctx); err != nil {
		t.Fatal(err)
	}
	if !m.Health().VacuumPending {
		t.Fatal("vacuum not reported pending")
	}
	if err := m.Close(); err != nil {
		t.Fatal(err)
	}
	if mode := autoVacuumMode(t, path); mode != 0 {
		t.Fatalf("auto_vacuum=%d; converted without disk room", mode)
	}
	next := New(path, runtimeDir)
	if err := next.Open(ctx); err != nil {
		t.Fatal(err)
	}
	if err := next.Close(); err != nil {
		t.Fatal(err)
	}
	if mode := autoVacuumMode(t, path); mode != 2 {
		t.Fatalf("auto_vacuum=%d after the retry want 2", mode)
	}
}

func TestNewDatabaseUsesIncrementalAutoVacuum(t *testing.T) {
	m := openTestManager(t)
	var mode int
	if err := m.db.QueryRowContext(context.Background(), "PRAGMA auto_vacuum").Scan(&mode); err != nil || mode != 2 {
		t.Fatalf("auto_vacuum=%d err=%v", mode, err)
	}
}

func exec(t *testing.T, m *Manager, query string, args ...any) {
	t.Helper()
	if _, err := m.db.ExecContext(context.Background(), query, args...); err != nil {
		t.Fatalf("%s: %v", query, err)
	}
}

func archiveResource(t *testing.T, m *Manager, id string, archivedAt time.Time) {
	t.Helper()
	if err := m.UpsertResource(context.Background(), Resource{ID: id, HostID: "host", StableKey: id, SourceKind: "compose", Name: id, Category: "service", Status: "healthy"}); err != nil {
		t.Fatal(err)
	}
	exec(t, m, "UPDATE resources SET status='archived',archived_at=?,last_seen_at=? WHERE id=?", archivedAt.UnixMilli(), archivedAt.UnixMilli(), id)
}

// STO-9 / ADR 021 §6: container instances whose stopped_at or destroyed_at
// (the later of them) is below the 1m retention cutoff are pruned; running
// instances never are.
func TestRetentionPrunesFinishedContainerInstances(t *testing.T) {
	ctx := context.Background()
	m := openTestManager(t)
	now := rollupBase.Add(9 * time.Hour)
	old, recent := now.Add(-48*time.Hour).UnixMilli(), now.Add(-time.Hour).UnixMilli()
	exec(t, m, "INSERT INTO container_instances(id,name,created_at,stopped_at) VALUES('ci_stopped_old','a',?,?)", old, old)
	exec(t, m, "INSERT INTO container_instances(id,name,created_at,destroyed_at) VALUES('ci_destroyed_old','b',?,?)", old, old)
	exec(t, m, "INSERT INTO container_instances(id,name,created_at,stopped_at) VALUES('ci_stopped_recent','c',?,?)", old, recent)
	exec(t, m, "INSERT INTO container_instances(id,name,created_at,stopped_at,destroyed_at) VALUES('ci_mixed','d',?,?,?)", old, old, recent)
	exec(t, m, "INSERT INTO container_instances(id,name,created_at) VALUES('ci_running','e',?)", old)
	if err := m.applyRetention(ctx, now, RetentionCutoffs{Raw: time.Hour, OneMinute: 24 * time.Hour}, false); err != nil {
		t.Fatal(err)
	}
	for id, want := range map[string]int{"ci_stopped_old": 0, "ci_destroyed_old": 0, "ci_stopped_recent": 1, "ci_mixed": 1, "ci_running": 1} {
		if got := countRows(t, m, "container_instances", "id=?", id); got != want {
			t.Errorf("%s present=%d want %d", id, got, want)
		}
	}
}

// STO-9 / ADR 021 §6: coolify_deployments and alert_evaluation_state follow
// the raw retention. alert_evaluation_state stores seconds, not milliseconds.
func TestRetentionPrunesCoolifyDeploymentsAndAlertState(t *testing.T) {
	ctx := context.Background()
	m := openTestManager(t)
	now := rollupBase.Add(9 * time.Hour)
	old, recent := now.Add(-49*time.Hour), now.Add(-time.Hour)
	exec(t, m, "INSERT INTO coolify_deployments(deployment_uuid,last_status,first_seen_at,updated_at) VALUES('d_old','finished',?,?)", old.UnixMilli(), old.UnixMilli())
	exec(t, m, "INSERT INTO coolify_deployments(deployment_uuid,last_status,first_seen_at,updated_at) VALUES('d_new','running',?,?)", recent.UnixMilli(), recent.UnixMilli())
	exec(t, m, "INSERT INTO alert_rules(id,family,name,severity,scope_type,trigger_seconds,recovery_seconds,created_at,updated_at) VALUES('rule','cpu','CPU','warning','global',60,60,0,0)")
	exec(t, m, "INSERT INTO alert_evaluation_state(dedup_key,rule_id,target_type,target_id,phase,phase_since,last_evaluated_at) VALUES('s_old','rule','host','host','healthy',?,?)", old.Unix(), old.Unix())
	exec(t, m, "INSERT INTO alert_evaluation_state(dedup_key,rule_id,target_type,target_id,phase,phase_since,last_evaluated_at) VALUES('s_new','rule','host','host2','firing',?,?)", recent.Unix(), recent.Unix())
	if err := m.applyRetention(ctx, now, RetentionCutoffs{Raw: 48 * time.Hour}, false); err != nil {
		t.Fatal(err)
	}
	if countRows(t, m, "coolify_deployments", "deployment_uuid='d_old'") != 0 || countRows(t, m, "coolify_deployments", "deployment_uuid='d_new'") != 1 {
		t.Fatalf("deployments old=%d new=%d", countRows(t, m, "coolify_deployments", "deployment_uuid='d_old'"), countRows(t, m, "coolify_deployments", "deployment_uuid='d_new'"))
	}
	if countRows(t, m, "alert_evaluation_state", "dedup_key='s_old'") != 0 || countRows(t, m, "alert_evaluation_state", "dedup_key='s_new'") != 1 {
		t.Fatalf("alert state old=%d new=%d", countRows(t, m, "alert_evaluation_state", "dedup_key='s_old'"), countRows(t, m, "alert_evaluation_state", "dedup_key='s_new'"))
	}
}

// STO-9 / ADR 021 §6: archived resources older than the 1h retention are
// deleted once nothing references them; a check, incident, rollup or sample
// keeps the row.
func TestRetentionDeletesArchivedResourcesWithoutReferences(t *testing.T) {
	ctx := context.Background()
	m := openTestManager(t)
	now := rollupBase.Add(9 * time.Hour)
	old := now.Add(-48 * time.Hour)
	for _, id := range []string{"res_gone", "res_checked", "res_rolled", "res_incident", "res_instances"} {
		archiveResource(t, m, id, old)
	}
	archiveResource(t, m, "res_fresh", now.Add(-time.Hour))
	exec(t, m, "INSERT INTO health_checks(id,resource_id,name,url,method,interval_seconds,timeout_seconds,expected_status_min,expected_status_max,created_at,updated_at) VALUES('chk','res_checked','c','http://x','GET',60,5,200,299,0,0)")
	// A 1h bucket inside the 1h window still references the resource.
	exec(t, m, "INSERT INTO resource_rollups_1h(ts,resource_id,sample_count) VALUES(?,'res_rolled',1)", now.Add(-12*time.Hour).UnixMilli())
	exec(t, m, "INSERT INTO incidents(id,group_key,status,severity,target_type,target_id,title,opened_at,updated_at) VALUES('inc','g','resolved','warning','resource','res_incident','t',?,?)", old.UnixMilli(), old.UnixMilli())
	exec(t, m, "INSERT INTO container_instances(id,resource_id,name,created_at) VALUES('ci_archived','res_instances','web',?)", old.UnixMilli())
	if err := m.applyRetention(ctx, now, RetentionCutoffs{Raw: time.Hour, OneHour: 24 * time.Hour}, false); err != nil {
		t.Fatal(err)
	}
	for id, want := range map[string]int{"res_gone": 0, "res_instances": 0, "res_checked": 1, "res_rolled": 1, "res_incident": 1, "res_fresh": 1} {
		if got := countRows(t, m, "resources", "id=?", id); got != want {
			t.Errorf("%s present=%d want %d", id, got, want)
		}
	}
	if countRows(t, m, "container_instances", "id='ci_archived'") != 0 {
		t.Error("the deleted resource's container instance was orphaned")
	}
}

// ADR 021 §6: with an unlimited 1h tier a count cap applies instead, and the
// oldest archived resources beyond it go through the existing deletion job so
// their history goes with them.
func TestArchivedResourceCapQueuesDeletionJobForTheOldest(t *testing.T) {
	ctx := context.Background()
	m := openTestManager(t)
	m.archivedResourceCap = 2
	now := rollupBase.Add(9 * time.Hour)
	for i, id := range []string{"res_oldest", "res_middle", "res_newest"} {
		archiveResource(t, m, id, now.Add(-time.Duration(3-i)*24*time.Hour))
	}
	policy := RetentionCutoffs{Raw: time.Hour, OneHour: 0}
	if err := m.applyRetention(ctx, now, policy, false); err != nil {
		t.Fatal(err)
	}
	var kind, resource, state, requestedBy string
	if err := m.db.QueryRowContext(ctx, "SELECT kind,resource_id,state,requested_by FROM history_deletion_jobs").Scan(&kind, &resource, &state, &requestedBy); err != nil {
		t.Fatalf("no deletion job queued: %v", err)
	}
	if kind != string(DeleteArchived) || resource != "res_oldest" || state != "queued" || requestedBy != "retention" {
		t.Fatalf("job kind=%s resource=%s state=%s by=%s", kind, resource, state, requestedBy)
	}
	if countRows(t, m, "resources", "") != 3 {
		t.Fatal("the cap deleted a resource directly instead of through the job")
	}
	// A second pass while the job is active queues nothing more.
	if err := m.applyRetention(ctx, now.Add(time.Hour), policy, false); err != nil {
		t.Fatal(err)
	}
	if got := countRows(t, m, "history_deletion_jobs", ""); got != 1 {
		t.Fatalf("jobs=%d want 1", got)
	}
	// Once the job ran the count is at the cap and nothing else is queued.
	exec(t, m, "UPDATE history_deletion_jobs SET state='completed'")
	exec(t, m, "DELETE FROM resources WHERE id='res_oldest'")
	if err := m.applyRetention(ctx, now.Add(2*time.Hour), policy, false); err != nil {
		t.Fatal(err)
	}
	if got := countRows(t, m, "history_deletion_jobs", ""); got != 1 {
		t.Fatalf("jobs=%d want 1 after the count fell to the cap", got)
	}
}

// STO-9: the archived list is paginated with the events cursor convention.
func TestArchivedResourcesPaginateByCursor(t *testing.T) {
	ctx := context.Background()
	m := openTestManager(t)
	now := rollupBase.Add(9 * time.Hour)
	for i, id := range []string{"res_a", "res_b", "res_c"} {
		archiveResource(t, m, id, now.Add(-time.Duration(3-i)*time.Hour))
	}
	page, err := m.ArchivedResources(ctx, ArchivedQuery{Limit: 2})
	if err != nil || len(page) != 2 || page[0].ID != "res_c" || page[1].ID != "res_b" {
		t.Fatalf("page=%+v err=%v", page, err)
	}
	rest, err := m.ArchivedResources(ctx, ArchivedQuery{Limit: 2, BeforeID: page[1].ID})
	if err != nil || len(rest) != 1 || rest[0].ID != "res_a" {
		t.Fatalf("rest=%+v err=%v", rest, err)
	}
	all, err := m.ArchivedResources(ctx, ArchivedQuery{})
	if err != nil || len(all) != 3 {
		t.Fatalf("default page=%d err=%v", len(all), err)
	}
}

// A sustained emergency re-runs the shortening pass every minute; the event
// is written once per change of the shortened set, not once per pass.
func TestRetentionShortenedEventIsWrittenOncePerTransition(t *testing.T) {
	ctx := context.Background()
	m := openTestManager(t)
	now := seedEightHours(t, m, time.Minute)
	m.SetRetention(RetentionCutoffs{Raw: 8 * time.Hour, OneMinute: 8 * time.Hour})
	m.SetBudgetTarget(1)
	for i := 0; i < 3; i++ {
		m.evaluateAt(ctx, now.Add(time.Duration(i)*time.Minute), false)
	}
	if got := eventCount(t, m, "retention_shortened"); got != 1 {
		t.Fatalf("retention_shortened events=%d want 1", got)
	}
	if got := eventCount(t, m, "persistence_emergency"); got != 1 {
		t.Fatalf("persistence_emergency events=%d want 1", got)
	}
}
