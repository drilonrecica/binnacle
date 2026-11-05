// SPDX-License-Identifier: AGPL-3.0-only
package storage

import (
	"context"
	"fmt"
	"strings"
	"time"
)

type Metric string

const (
	MetricCPU            Metric = "cpu"
	MetricCPUUser        Metric = "cpu_user"
	MetricCPUSystem      Metric = "cpu_system"
	MetricCPUIOWait      Metric = "cpu_iowait"
	MetricCPUSteal       Metric = "cpu_steal"
	MetricMemory         Metric = "memory"
	MetricSwap           Metric = "swap"
	MetricLoad1          Metric = "load_1"
	MetricLoad5          Metric = "load_5"
	MetricLoad15         Metric = "load_15"
	MetricNetworkRX      Metric = "network_rx"
	MetricNetworkTX      Metric = "network_tx"
	MetricNetworkPackets Metric = "network_packets"
	MetricDiskRead       Metric = "disk_read"
	MetricDiskWrite      Metric = "disk_write"
	MetricDiskIOPS       Metric = "disk_iops"
	MetricBlockRead      Metric = "block_read"
	MetricBlockWrite     Metric = "block_write"
)

type Resolution string

const (
	ResolutionRaw Resolution = "10s"
	Resolution1m  Resolution = "1m"
	Resolution15m Resolution = "15m"
	Resolution1h  Resolution = "1h"
)

type MetricQuery struct {
	Scope, ID string
	Metrics   []Metric
	From, To  time.Time
}
type Point struct {
	At    time.Time `json:"at"`
	Min   *float64  `json:"min"`
	Avg   *float64  `json:"avg"`
	Max   *float64  `json:"max"`
	Count int       `json:"count"`
}
type Series struct {
	Metric Metric  `json:"metric"`
	Unit   string  `json:"unit"`
	Points []Point `json:"points"`
}
type Gap struct {
	From   time.Time `json:"from"`
	To     time.Time `json:"to"`
	Reason string    `json:"reason"`
}
type MetricsResponse struct {
	Scope      string     `json:"scope"`
	ID         string     `json:"id,omitempty"`
	From       time.Time  `json:"from"`
	To         time.Time  `json:"to"`
	Resolution Resolution `json:"resolution"`
	Series     []Series   `json:"series"`
	Gaps       []Gap      `json:"gaps"`
}

func (m *Manager) HasMetricHistory(ctx context.Context) (bool, error) {
	var exists bool
	err := m.db.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM host_samples_10s WHERE host_id=?)", hostID).Scan(&exists)
	return exists, err
}

func (q MetricQuery) Validate() error {
	if q.Scope != "host" && q.Scope != "resource" {
		return fmt.Errorf("invalid scope")
	}
	if q.Scope == "host" && q.ID != "" {
		return fmt.Errorf("host metrics do not accept id")
	}
	if q.Scope == "resource" && q.ID == "" {
		return fmt.Errorf("resource id is required")
	}
	if q.From.IsZero() || q.To.IsZero() || !q.From.Before(q.To) || q.To.Sub(q.From) > 30*24*time.Hour {
		return fmt.Errorf("invalid time range")
	}
	if len(q.Metrics) == 0 || len(q.Metrics) > 6 {
		return fmt.Errorf("invalid metric count")
	}
	seen := map[Metric]bool{}
	for _, metric := range q.Metrics {
		if seen[metric] || !metricAllowed(q.Scope, metric) {
			return fmt.Errorf("invalid metric %q", metric)
		}
		seen[metric] = true
	}
	return nil
}

var hostMetrics = map[Metric]bool{
	MetricCPU: true, MetricCPUUser: true, MetricCPUSystem: true, MetricCPUIOWait: true, MetricCPUSteal: true,
	MetricMemory: true, MetricSwap: true,
	MetricLoad1: true, MetricLoad5: true, MetricLoad15: true,
	MetricNetworkRX: true, MetricNetworkTX: true,
	MetricDiskRead: true, MetricDiskWrite: true, MetricDiskIOPS: true,
}
var resourceMetrics = map[Metric]bool{
	MetricCPU: true, MetricMemory: true, MetricNetworkRX: true, MetricNetworkTX: true, MetricBlockRead: true, MetricBlockWrite: true,
}

func metricAllowed(scope string, metric Metric) bool {
	if scope == "host" {
		return hostMetrics[metric]
	}
	return resourceMetrics[metric]
}
func selectResolution(d time.Duration) Resolution {
	switch {
	case d <= 2*time.Hour:
		return ResolutionRaw
	case d <= 1000*time.Minute:
		return Resolution1m
	case d <= 1000*15*time.Minute:
		return Resolution15m
	default:
		return Resolution1h
	}
}
func metricUnit(metric Metric) string {
	switch metric {
	case MetricCPU, MetricCPUUser, MetricCPUSystem, MetricCPUIOWait, MetricCPUSteal:
		return "percent"
	case MetricMemory, MetricSwap:
		return "bytes"
	case MetricLoad1, MetricLoad5, MetricLoad15:
		return "load"
	case MetricDiskIOPS:
		return "ops_per_second"
	}
	return "bytes_per_second"
}

func (m *Manager) QueryMetrics(ctx context.Context, q MetricQuery) (MetricsResponse, error) {
	if err := q.Validate(); err != nil {
		return MetricsResponse{}, err
	}
	m.mu.Lock()
	retention, shortened := m.retention, append([]string(nil), m.budget.Shortened...)
	m.mu.Unlock()
	res := resolutionFor(q.From, q.To, time.Now(), effectiveRetention(retention, shortened))
	r := MetricsResponse{Scope: q.Scope, ID: q.ID, From: q.From.UTC(), To: q.To.UTC(), Resolution: res, Gaps: []Gap{}}
	for index, metric := range q.Metrics {
		points, err := m.metricPoints(ctx, q, metric, r.Resolution)
		if err != nil {
			return MetricsResponse{}, err
		}
		r.Series = append(r.Series, Series{Metric: metric, Unit: metricUnit(metric), Points: points})
		if index == 0 {
			gaps := findGaps(q.From, q.To, r.Resolution, points)
			if err = m.classifyGaps(ctx, q, gaps); err != nil {
				return MetricsResponse{}, err
			}
			r.Gaps = mergeGaps(gaps)
		}
	}
	return r, nil
}

// resolutionFor starts from the tier the range length selects and moves to
// coarser tiers until one whose retention still covers the range start, so a
// short range far in the past is read from a tier that has its data instead
// of being reported as one long gap (EVT-6). A retention of zero is
// unlimited; the 1h tier is the last resort.
func resolutionFor(from, to, now time.Time, r RetentionCutoffs) Resolution {
	tiers := []struct {
		res  Resolution
		keep time.Duration
	}{{ResolutionRaw, r.Raw}, {Resolution1m, r.OneMinute}, {Resolution15m, r.FifteenMinute}, {Resolution1h, r.OneHour}}
	wanted := selectResolution(to.Sub(from))
	for i, tier := range tiers {
		if tier.res != wanted {
			continue
		}
		for _, candidate := range tiers[i:] {
			if candidate.keep <= 0 || !from.Before(now.Add(-candidate.keep)) {
				return candidate.res
			}
		}
	}
	return Resolution1h
}

// effectiveRetention halves the tiers the latest emergency pass shortened
// (ADR 021 §7), matching what retention actually keeps.
func effectiveRetention(r RetentionCutoffs, shortened []string) RetentionCutoffs {
	for _, tier := range shortened {
		switch tier {
		case "raw":
			r.Raw /= 2
		case "1m":
			r.OneMinute /= 2
		}
	}
	return r
}

// The gap classifier reads one range per table for all gaps of a response
// (EVT-6). Each is a SEARCH: events_ts, collector_state_events_collector_ts
// (STO-7) and the resource_samples_10s primary key.
const (
	// gapEventsSQL lists the events that explain a gap in the whole range.
	gapEventsSQL = `SELECT ts,type FROM events WHERE ts>=? AND ts<=? AND type IN ('persistence_degraded','persistence_gap','host_reboot') ORDER BY ts`
	// gapCollectorStatesSQL lists the collector's state changes in the
	// range, starting with the last one at or before its start.
	gapCollectorStatesSQL = `SELECT ts,new_state FROM collector_state_events WHERE collector_name=? AND ts<=? AND ts>=COALESCE((SELECT MAX(ts) FROM collector_state_events WHERE collector_name=? AND ts<=?),0) ORDER BY ts`
	// gapResourceActivitySQL is prefixed with a gaps(idx,until) CTE and
	// returns the resource's last sample before each gap's end.
	gapResourceActivitySQL = ` SELECT gaps.idx,COALESCE(s.status,''),COALESCE(s.active_instance_count,0) FROM gaps LEFT JOIN resource_samples_10s s ON s.resource_id=? AND s.ts=(SELECT MAX(ts) FROM resource_samples_10s WHERE resource_id=? AND ts<gaps.until)`
)

// maxGapsPerActivityQuery bounds the bind variables of one activity query.
const maxGapsPerActivityQuery = 400

type gapEvent struct {
	at   int64
	kind string
}

// classifyGaps sets each gap's reason: host_reboot, persistence_failure,
// collector_unavailable, inactive (resources only) or missing, in that order.
func (m *Manager) classifyGaps(ctx context.Context, q MetricQuery, gaps []Gap) error {
	if len(gaps) == 0 {
		return nil
	}
	from, to := gaps[0].From.UnixMilli(), gaps[len(gaps)-1].To.UnixMilli()
	events, err := m.gapEvents(ctx, gapEventsSQL, from, to)
	if err != nil {
		return fmt.Errorf("gap events: %w", err)
	}
	collector := "host"
	if q.Scope == "resource" {
		collector = "docker"
	}
	states, err := m.gapEvents(ctx, gapCollectorStatesSQL, collector, to, collector, from)
	if err != nil {
		return fmt.Errorf("gap collector states: %w", err)
	}
	var inactive []bool
	if q.Scope == "resource" {
		if inactive, err = m.gapInactivity(ctx, q.ID, gaps); err != nil {
			return fmt.Errorf("gap resource activity: %w", err)
		}
	}
	for i := range gaps {
		gaps[i].Reason = classifyGap(gaps[i], events, states, inactive != nil && inactive[i])
	}
	return nil
}

func classifyGap(gap Gap, events, states []gapEvent, inactive bool) string {
	from, to := gap.From.UnixMilli(), gap.To.UnixMilli()
	within := func(e gapEvent) bool { return e.at >= from && e.at <= to }
	for _, kind := range []struct{ event, reason string }{{"host_reboot", "host_reboot"}, {"persistence_degraded", "persistence_failure"}, {"persistence_gap", "persistence_failure"}} {
		for _, e := range events {
			if e.kind == kind.event && within(e) {
				return kind.reason
			}
		}
	}
	down := func(state string) bool { return state == "degraded" || state == "down" }
	current := ""
	for _, s := range states {
		if s.at <= from {
			current = s.kind
		} else if s.at < to && down(s.kind) {
			return "collector_unavailable"
		}
	}
	if down(current) {
		return "collector_unavailable"
	}
	if inactive {
		return "inactive"
	}
	return "missing"
}

func (m *Manager) gapEvents(ctx context.Context, query string, args ...any) ([]gapEvent, error) {
	rows, err := m.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []gapEvent
	for rows.Next() {
		var e gapEvent
		if err = rows.Scan(&e.at, &e.kind); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// gapInactivity reports, per gap, whether the resource's last sample before
// the gap's end shows it paused, archived or without running instances.
func (m *Manager) gapInactivity(ctx context.Context, resourceID string, gaps []Gap) ([]bool, error) {
	out := make([]bool, len(gaps))
	for start := 0; start < len(gaps); start += maxGapsPerActivityQuery {
		chunk := gaps[start:min(start+maxGapsPerActivityQuery, len(gaps))]
		args := make([]any, 0, 2*len(chunk)+2)
		values := make([]string, len(chunk))
		for i, gap := range chunk {
			values[i] = "(?,?)"
			args = append(args, start+i, gap.To.UnixMilli())
		}
		args = append(args, resourceID, resourceID)
		rows, err := m.db.QueryContext(ctx, "WITH gaps(idx,until) AS (VALUES "+strings.Join(values, ",")+")"+gapResourceActivitySQL, args...)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var index, active int
			var status string
			if err = rows.Scan(&index, &status, &active); err != nil {
				rows.Close()
				return nil, err
			}
			out[index] = active == 0 || status == "paused" || status == "archived"
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}
func (m *Manager) metricPoints(ctx context.Context, q MetricQuery, metric Metric, res Resolution) ([]Point, error) {
	query, args, err := metricPointsSQL(q, metric, res)
	if err != nil {
		return nil, err
	}
	rows, err := m.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Point{}
	for rows.Next() {
		var ms int64
		var p Point
		if res == ResolutionRaw {
			if err = rows.Scan(&ms, &p.Min, &p.Avg, &p.Max); err != nil {
				return nil, err
			}
			if p.Avg != nil {
				p.Count = 1
			}
		} else {
			if err = rows.Scan(&ms, &p.Min, &p.Avg, &p.Max, &p.Count); err != nil {
				return nil, err
			}
		}
		p.At = time.UnixMilli(ms).UTC()
		out = append(out, p)
	}
	return out, rows.Err()
}

// metricPointsSQL is the history query of one metric at one resolution.
func metricPointsSQL(q MetricQuery, metric Metric, res Resolution) (string, []any, error) {
	minColumn, avgColumn, maxColumn, countColumn, table, err := metricSource(q.Scope, metric, res)
	if err != nil {
		return "", nil, err
	}
	columns := "ts," + minColumn + "," + avgColumn + "," + maxColumn
	if res != ResolutionRaw {
		columns += "," + countColumn
	}
	query := "SELECT " + columns + " FROM " + table + " WHERE ts>=? AND ts<=?"
	args := []any{q.From.UnixMilli(), q.To.UnixMilli()}
	switch {
	case q.Scope == "resource":
		query += " AND resource_id=?"
		args = append(args, q.ID)
	case res == ResolutionRaw:
		// The host rollup tables are keyed by ts alone; host_samples_10s is
		// keyed (host_id, ts), so the range needs host_id to search it (STO-7).
		query += " AND host_id=?"
		args = append(args, hostID)
	}
	query += " ORDER BY ts"
	return query, args, nil
}
func metricSource(scope string, metric Metric, res Resolution) (string, string, string, string, string, error) {
	raw := map[Metric]string{
		MetricCPU: "cpu_busy_pct", MetricCPUUser: "cpu_user_pct", MetricCPUSystem: "cpu_system_pct", MetricCPUIOWait: "cpu_iowait_pct", MetricCPUSteal: "cpu_steal_pct",
		MetricMemory: "memory_used_bytes", MetricSwap: "swap_used_bytes",
		MetricLoad1: "load_1", MetricLoad5: "load_5", MetricLoad15: "load_15",
		MetricNetworkRX: "network_rx_bps", MetricNetworkTX: "network_tx_bps",
		MetricDiskRead: "disk_read_bps", MetricDiskWrite: "disk_write_bps",
		MetricDiskIOPS:  "disk_read_iops + disk_write_iops",
		MetricBlockRead: "block_read_bps", MetricBlockWrite: "block_write_bps",
	}
	prefix := map[Metric]string{
		MetricCPU: "cpu", MetricCPUUser: "cpu_user", MetricCPUSystem: "cpu_system", MetricCPUIOWait: "cpu_iowait", MetricCPUSteal: "cpu_steal",
		MetricMemory: "memory", MetricSwap: "swap_used",
		MetricLoad1: "load_1", MetricLoad5: "load_5", MetricLoad15: "load_15",
		MetricNetworkRX: "network_rx", MetricNetworkTX: "network_tx",
		MetricDiskRead: "disk_read", MetricDiskWrite: "disk_write", MetricDiskIOPS: "disk_iops",
		MetricBlockRead: "block_read", MetricBlockWrite: "block_write",
	}[metric]
	if raw[metric] == "" {
		return "", "", "", "", "", fmt.Errorf("unsupported metric")
	}
	if res == ResolutionRaw {
		table := "resource_samples_10s"
		if scope == "host" {
			table = "host_samples_10s"
		}
		column := raw[metric]
		if scope == "resource" {
			switch metric {
			case MetricCPU:
				column = "cpu_host_pct"
			case MetricMemory:
				column = "memory_working_set_bytes"
			}
		}
		// A raw sample is its window's mean; the window range columns
		// (COL-9) hold its extremes, NULL on rows older than migration 024.
		rangeBase := column
		if metric == MetricDiskIOPS {
			rangeBase = "disk_iops"
		}
		return "COALESCE(" + rangeBase + "_min," + column + ")", column, "COALESCE(" + rangeBase + "_max," + column + ")", "", table, nil
	}
	if prefix == "" {
		return "", "", "", "", "", fmt.Errorf("unsupported metric for rollup resolution")
	}
	count := prefix + "_count"
	if metric == MetricCPU {
		count = "sample_count"
	}
	tablePrefix := "resource_rollups_"
	if scope == "host" {
		tablePrefix = "host_rollups_"
	}
	return prefix + "_min", prefix + "_avg", prefix + "_max", count, tablePrefix + string(res), nil
}
func findGaps(from, to time.Time, res Resolution, points []Point) []Gap {
	step := map[Resolution]time.Duration{ResolutionRaw: 10 * time.Second, Resolution1m: time.Minute, Resolution15m: 15 * time.Minute, Resolution1h: time.Hour}[res]
	if step == 0 {
		return nil
	}
	out := []Gap{}
	cursor := from.UTC()
	for _, p := range points {
		if p.At.Sub(cursor) > step*2 {
			out = append(out, Gap{From: cursor, To: p.At, Reason: "missing"})
		}
		if p.Avg == nil {
			out = append(out, Gap{From: p.At, To: p.At.Add(step), Reason: "missing"})
		}
		cursor = p.At.Add(step)
	}
	if to.UTC().Sub(cursor) > step*2 {
		out = append(out, Gap{From: cursor, To: to.UTC(), Reason: "missing"})
	}
	return out
}

func mergeGaps(gaps []Gap) []Gap {
	if len(gaps) < 2 {
		return gaps
	}
	out := []Gap{gaps[0]}
	for _, gap := range gaps[1:] {
		last := &out[len(out)-1]
		if gap.Reason == last.Reason && !gap.From.After(last.To) {
			if gap.To.After(last.To) {
				last.To = gap.To
			}
			continue
		}
		out = append(out, gap)
	}
	return out
}

func (m *Manager) HostCPU(ctx context.Context, from, to time.Time, limit int) ([]Point, error) {
	r, err := m.QueryMetrics(ctx, MetricQuery{Scope: "host", Metrics: []Metric{MetricCPU}, From: from, To: to})
	if err != nil {
		return nil, err
	}
	if len(r.Series) == 0 {
		return nil, nil
	}
	if limit > 0 && len(r.Series[0].Points) > limit {
		return r.Series[0].Points[:limit], nil
	}
	return r.Series[0].Points, nil
}
