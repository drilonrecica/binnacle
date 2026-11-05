// SPDX-License-Identifier: AGPL-3.0-only
package storage

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// queryPlan returns the EXPLAIN QUERY PLAN detail lines of query.
func queryPlan(t *testing.T, m *Manager, query string, args ...any) []string {
	t.Helper()
	rows, err := m.Reader().QueryContext(context.Background(), "EXPLAIN QUERY PLAN "+query, args...)
	if err != nil {
		t.Fatalf("explain %s: %v", query, err)
	}
	defer rows.Close()
	var plan []string
	for rows.Next() {
		var id, parent, unused int
		var detail string
		if err := rows.Scan(&id, &parent, &unused, &detail); err != nil {
			t.Fatal(err)
		}
		plan = append(plan, detail)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return plan
}

type planCase struct {
	name  string
	query string
	args  []any
	table string
	// want lists the accepted SEARCH lines for table; the plan must contain
	// one of them.
	want []string
}

func planCases(t *testing.T) []planCase {
	t.Helper()
	from := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
	to := from.Add(time.Hour)
	from6h := to.Add(-6 * time.Hour)
	cases := []planCase{{
		name:  "sparklines",
		query: sparklineSQL([]Metric{MetricCPU, MetricMemory}),
		args:  []any{from6h.UnixMilli(), 360_000, from6h.UnixMilli(), to.UnixMilli()},
		table: "resource_samples_10s",
		// Statistics that show few resources make a skip-scan of the primary
		// key (one seek per resource) cheaper than the ts index; both search.
		want: []string{"USING INDEX resource_samples_10s_ts (ts>? AND ts<?)", "USING INDEX sqlite_autoindex_resource_samples_10s_1 (ANY(resource_id) AND ts>? AND ts<?)"},
	}}
	history := []struct {
		name        string
		q           MetricQuery
		res         Resolution
		table, want string
	}{
		{"host raw history", MetricQuery{Scope: "host"}, ResolutionRaw, "host_samples_10s", "USING INDEX sqlite_autoindex_host_samples_10s_1 (host_id=? AND ts>? AND ts<?)"},
		{"host 1m history", MetricQuery{Scope: "host"}, Resolution1m, "host_rollups_1m", "USING INTEGER PRIMARY KEY (rowid>? AND rowid<?)"},
		{"host 1h history", MetricQuery{Scope: "host"}, Resolution1h, "host_rollups_1h", "USING INTEGER PRIMARY KEY (rowid>? AND rowid<?)"},
		{"resource raw history", MetricQuery{Scope: "resource", ID: "res_a"}, ResolutionRaw, "resource_samples_10s", "USING INDEX sqlite_autoindex_resource_samples_10s_1 (resource_id=? AND ts>? AND ts<?)"},
		{"resource 15m history", MetricQuery{Scope: "resource", ID: "res_a"}, Resolution15m, "resource_rollups_15m", "USING INDEX sqlite_autoindex_resource_rollups_15m_1 (resource_id=? AND ts>? AND ts<?)"},
	}
	for _, h := range history {
		h.q.From, h.q.To = from, to
		query, args, err := metricPointsSQL(h.q, MetricCPU, h.res)
		if err != nil {
			t.Fatal(err)
		}
		cases = append(cases, planCase{name: h.name, query: query, args: args, table: h.table, want: []string{h.want}})
	}
	for _, table := range []string{"host_samples_10s", "resource_samples_10s", "container_instance_samples_10s"} {
		cases = append(cases, planCase{name: table + " retention", query: deleteBatchSQL(table, "ts<?"), args: []any{from.UnixMilli(), 500},
			table: table, want: []string{"USING COVERING INDEX " + table + "_ts (ts<?)"}})
	}
	cursor := from.Add(30 * time.Minute).UnixMilli()
	feed, feedArgs := eventQuerySQL(EventQuery{From: from, To: to, Severity: "warning", Types: []string{"container_restart"}, BeforeID: "evt"}, 100, &cursor)
	forResource, forResourceArgs := eventQuerySQL(EventQuery{From: from, To: to, ResourceID: "res_a"}, 100, nil)
	return append(cases,
		planCase{"event feed", feed, feedArgs, "events", []string{"USING INDEX events_ts (ts>? AND ts<?)"}},
		planCase{"resource event feed", forResource, forResourceArgs, "events", []string{"USING INDEX events_resource_ts (resource_id=? AND ts>? AND ts<?)"}},
		planCase{"gap persistence events", gapPersistenceSQL, []any{from.UnixMilli(), to.UnixMilli()}, "events", []string{"USING INDEX events_ts (ts>? AND ts<?)"}},
		planCase{"gap collector state", gapCollectorStateSQL, []any{"host", from.UnixMilli()}, "collector_state_events", []string{"USING INDEX collector_state_events_collector_ts (collector_name=? AND ts<?)"}},
		planCase{"gap collector down", gapCollectorDownSQL, []any{"host", from.UnixMilli(), to.UnixMilli()}, "collector_state_events", []string{"USING INDEX collector_state_events_collector_ts (collector_name=? AND ts>? AND ts<?)"}},
		planCase{"gap resource activity", gapResourceActivitySQL, []any{"res_a", to.UnixMilli()}, "resource_samples_10s", []string{"USING INDEX sqlite_autoindex_resource_samples_10s_1 (resource_id=? AND ts<?)"}},
	)
}

func assertPlans(t *testing.T, m *Manager) {
	t.Helper()
	for _, c := range planCases(t) {
		plan := queryPlan(t, m, c.query, c.args...)
		joined := strings.Join(plan, "\n")
		searched := false
		for _, want := range c.want {
			searched = searched || strings.Contains(joined, "SEARCH "+c.table+" "+want)
		}
		if !searched {
			t.Errorf("%s: plan does not search %s %q:\n%s", c.name, c.table, c.want, joined)
		}
		if strings.Contains(joined, "SCAN "+c.table) {
			t.Errorf("%s: plan scans %s:\n%s", c.name, c.table, joined)
		}
	}
}

// seedPlanData writes enough rows that ANALYZE has a distribution to record:
// a day of raw samples for one host and twenty resources.
func seedPlanData(t *testing.T, m *Manager) {
	t.Helper()
	ctx := context.Background()
	tx, err := m.db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	start := time.Date(2026, 7, 11, 0, 0, 0, 0, time.UTC).UnixMilli()
	for ts := start; ts < start+24*time.Hour.Milliseconds(); ts += 60_000 {
		if _, err := tx.ExecContext(ctx, "INSERT INTO host_samples_10s(ts,host_id,cpu_busy_pct) VALUES(?,?,1)", ts, hostID); err != nil {
			t.Fatal(err)
		}
		for r := range 20 {
			if _, err := tx.ExecContext(ctx, "INSERT INTO resource_samples_10s(ts,resource_id,cpu_host_pct,active_instance_count) VALUES(?,?,1,1)", ts, fmt.Sprintf("res_%02d", r)); err != nil {
				t.Fatal(err)
			}
		}
		if ts%(15*60_000) == 0 {
			for _, collector := range []string{"host", "docker"} {
				if _, err := tx.ExecContext(ctx, "INSERT INTO collector_state_events(id,ts,collector_name,new_state) VALUES(?,?,?,'healthy')", fmt.Sprintf("%s-%d", collector, ts), ts, collector); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := tx.ExecContext(ctx, "INSERT INTO events(id,ts,host_id,resource_id,type,severity,summary,source,created_at) VALUES(?,?,?,'res_01','container_restart','warning','restart','docker',?)", fmt.Sprintf("evt-%d", ts), ts, hostID, ts); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
}

// STO-7: sparklines, history, the gap classifier, the event feed and raw
// retention search an
// index instead of scanning their table, both with no planner statistics
// and after PRAGMA optimize stored some.
func TestHotQueriesSearchIndexes(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	m := New(filepath.Join(dir, "binnacle.db"), filepath.Join(dir, "run"))
	if err := m.Open(ctx); err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	assertPlans(t, m)

	seedPlanData(t, m)
	if err := m.optimize(ctx); err != nil {
		t.Fatal(err)
	}
	var stats int
	if err := m.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM sqlite_stat1 WHERE tbl IN ('resource_samples_10s','host_samples_10s')").Scan(&stats); err != nil || stats == 0 {
		t.Fatalf("PRAGMA optimize stored %d statistics rows err=%v", stats, err)
	}
	assertPlans(t, m)
}

// The storage workers run PRAGMA optimize after the boot delay and then on
// every interval, and stop with the manager.
func TestOptimizeWorkerRefreshesStatistics(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	m := New(filepath.Join(dir, "binnacle.db"), filepath.Join(dir, "run"))
	if err := m.Open(ctx); err != nil {
		t.Fatal(err)
	}
	seedPlanData(t, m)
	m.optimizeBootDelay, m.optimizeInterval = 10*time.Millisecond, time.Hour
	if err := m.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer m.Stop(ctx)
	analyzed := func() bool {
		var exists bool
		err := m.db.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM sqlite_master WHERE name='sqlite_stat1')").Scan(&exists)
		if err != nil || !exists {
			return false
		}
		var rows int
		return m.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM sqlite_stat1 WHERE tbl='resource_samples_10s'").Scan(&rows) == nil && rows > 0
	}
	if !waitFor(t, 5*time.Second, analyzed) {
		t.Fatal("the optimize worker stored no planner statistics")
	}
}
