// SPDX-License-Identifier: AGPL-3.0-only
package storage

import (
	"context"
	"time"
)

type HistoricalEvent struct {
	ID                              string    `json:"id"`
	At                              time.Time `json:"ts"`
	Type, Severity, Summary, Source string    `json:"type"`
}

func (m *Manager) Events(ctx context.Context, from, to time.Time, limit int) ([]HistoricalEvent, error) {
	if limit < 1 || limit > 200 {
		limit = 100
	}
	rows, e := m.db.QueryContext(ctx, "SELECT id,ts,type,severity,summary,source FROM events WHERE ts>=? AND ts<=? ORDER BY ts DESC,id DESC LIMIT ?", from.UnixMilli(), to.UnixMilli(), limit)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []HistoricalEvent{}
	for rows.Next() {
		var v HistoricalEvent
		var ms int64
		if e = rows.Scan(&v.ID, &ms, &v.Type, &v.Severity, &v.Summary, &v.Source); e != nil {
			return nil, e
		}
		v.At = time.UnixMilli(ms).UTC()
		out = append(out, v)
	}
	return out, rows.Err()
}
