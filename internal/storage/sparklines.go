// SPDX-License-Identifier: AGPL-3.0-only
package storage

import (
	"context"
	"fmt"
	"math"
	"time"
)

// Sparkline bounds keep the query cheap: one bucketed scan over raw samples,
// at most two metrics, at most a few hours, and a fixed number of slots.
const (
	sparklineMaxRange   = 6 * time.Hour
	sparklineMaxMetrics = 2
	sparklineSlots      = 60
	sparklineMaxSeries  = 500
)

type SparklineQuery struct {
	Metrics  []Metric
	From, To time.Time
}

type SparklineResponse struct {
	From        time.Time                        `json:"from"`
	To          time.Time                        `json:"to"`
	StepSeconds int                              `json:"stepSeconds"`
	Resources   map[string]map[Metric][]*float64 `json:"resources"`
}

func (q SparklineQuery) Validate() error {
	if q.From.IsZero() || q.To.IsZero() || !q.From.Before(q.To) || q.To.Sub(q.From) > sparklineMaxRange {
		return fmt.Errorf("invalid time range")
	}
	if len(q.Metrics) == 0 || len(q.Metrics) > sparklineMaxMetrics {
		return fmt.Errorf("invalid metric count")
	}
	seen := map[Metric]bool{}
	for _, metric := range q.Metrics {
		if seen[metric] || sparklineColumn(metric) == "" {
			return fmt.Errorf("invalid metric %q", metric)
		}
		seen[metric] = true
	}
	return nil
}

func sparklineColumn(metric Metric) string {
	switch metric {
	case MetricCPU:
		return "cpu_host_pct"
	case MetricMemory:
		return "memory_working_set_bytes"
	case MetricNetworkRX:
		return "network_rx_bps"
	case MetricNetworkTX:
		return "network_tx_bps"
	}
	return ""
}

// ResourceSparklines returns fixed-slot averages for every active resource in
// the range so a list view can draw one sparkline per row from one request.
func (m *Manager) ResourceSparklines(ctx context.Context, q SparklineQuery) (SparklineResponse, error) {
	if err := q.Validate(); err != nil {
		return SparklineResponse{}, err
	}
	from, to := q.From.UTC(), q.To.UTC()
	stepMs := max(int64(math.Ceil(float64(to.Sub(from).Milliseconds())/float64(sparklineSlots)/10_000))*10_000, 10_000)
	origin := from.UnixMilli()
	slots := min(int((to.UnixMilli()-origin)/stepMs)+1, sparklineSlots+1)

	columns := ""
	for _, metric := range q.Metrics {
		columns += ", AVG(" + sparklineColumn(metric) + ")"
	}
	query := "SELECT resource_id, ((ts - ?) / ?) AS slot" + columns +
		" FROM resource_samples_10s WHERE ts>=? AND ts<=? AND active_instance_count>0 GROUP BY resource_id, slot ORDER BY resource_id, slot"
	rows, err := m.db.QueryContext(ctx, query, origin, stepMs, origin, to.UnixMilli())
	if err != nil {
		return SparklineResponse{}, err
	}
	defer rows.Close()

	out := SparklineResponse{From: from, To: to, StepSeconds: int(stepMs / 1000), Resources: map[string]map[Metric][]*float64{}}
	values := make([]*float64, len(q.Metrics))
	scan := make([]any, 0, 2+len(q.Metrics))
	for rows.Next() {
		var id string
		var slot int64
		scan = scan[:0]
		scan = append(scan, &id, &slot)
		for index := range values {
			values[index] = nil
			scan = append(scan, &values[index])
		}
		if err := rows.Scan(scan...); err != nil {
			return SparklineResponse{}, err
		}
		if slot < 0 || int(slot) >= slots {
			continue
		}
		series, ok := out.Resources[id]
		if !ok {
			if len(out.Resources) >= sparklineMaxSeries {
				continue
			}
			series = map[Metric][]*float64{}
			for _, metric := range q.Metrics {
				series[metric] = make([]*float64, slots)
			}
			out.Resources[id] = series
		}
		for index, metric := range q.Metrics {
			if values[index] != nil {
				value := *values[index]
				series[metric][slot] = &value
			}
		}
	}
	return out, rows.Err()
}
