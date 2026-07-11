// SPDX-License-Identifier: AGPL-3.0-only
package storage

import (
	"context"
	"fmt"
	"time"
)

type Point struct {
	At  time.Time
	Avg *float64
}

func (m *Manager) HostCPU(ctx context.Context, from, to time.Time, limit int) ([]Point, error) {
	if limit < 1 || limit > 1000 {
		limit = 1000
	}
	rows, e := m.db.QueryContext(ctx, "SELECT ts,cpu_busy_pct FROM host_samples_10s WHERE ts>=? AND ts<=? ORDER BY ts LIMIT ?", from.UnixMilli(), to.UnixMilli(), limit)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []Point{}
	for rows.Next() {
		var ms int64
		var v *float64
		if e = rows.Scan(&ms, &v); e != nil {
			return nil, e
		}
		out = append(out, Point{time.UnixMilli(ms).UTC(), v})
	}
	if e = rows.Err(); e != nil {
		return nil, e
	}
	if len(out) > limit {
		return nil, fmt.Errorf("point cap exceeded")
	}
	return out, nil
}
