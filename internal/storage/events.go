// SPDX-License-Identifier: AGPL-3.0-only
package storage

import (
	"context"
	"database/sql"
	"strings"
	"time"
)

type HistoricalEvent struct {
	ID                string    `json:"id"`
	At                time.Time `json:"ts"`
	Type              string    `json:"type"`
	Severity          string    `json:"severity"`
	Summary           string    `json:"summary"`
	Details           *string   `json:"details,omitempty"`
	CorrelationKey    *string   `json:"correlationKey,omitempty"`
	ContainerInstance *string   `json:"containerInstanceId,omitempty"`
	ResourceID        *string   `json:"resourceId,omitempty"`
	Source            string    `json:"source"`
}

func (m *Manager) Events(ctx context.Context, from, to time.Time, limit int) ([]HistoricalEvent, error) {
	return m.EventsFor(ctx, from, to, limit, "")
}
func (m *Manager) EventsFor(ctx context.Context, from, to time.Time, limit int, resourceID string) ([]HistoricalEvent, error) {
	if limit < 1 || limit > 200 {
		limit = 100
	}
	query := "SELECT id,ts,type,severity,summary,details_json,correlation_key,container_instance_id,resource_id,source FROM events WHERE ts>=? AND ts<=?"
	args := []any{from.UnixMilli(), to.UnixMilli()}
	if resourceID != "" {
		query += " AND resource_id=?"
		args = append(args, resourceID)
	}
	query += " ORDER BY ts DESC,id DESC LIMIT ?"
	args = append(args, limit)
	rows, e := m.db.QueryContext(ctx, query, args...)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []HistoricalEvent{}
	for rows.Next() {
		var v HistoricalEvent
		var ms int64
		var details, correlation, containerInstance, resourceIDVal sql.NullString
		if e = rows.Scan(&v.ID, &ms, &v.Type, &v.Severity, &v.Summary, &details, &correlation, &containerInstance, &resourceIDVal, &v.Source); e != nil {
			return nil, e
		}
		v.At = time.UnixMilli(ms).UTC()
		if details.Valid {
			v.Details = &details.String
		}
		if correlation.Valid {
			v.CorrelationKey = &correlation.String
		}
		if containerInstance.Valid {
			v.ContainerInstance = &containerInstance.String
		}
		if resourceIDVal.Valid {
			v.ResourceID = &resourceIDVal.String
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (m *Manager) ExportEvents(ctx context.Context, from, to time.Time, limit int) ([]HistoricalEvent, error) {
	if limit < 1 || limit > 10001 {
		limit = 10001
	}
	rows, e := m.db.QueryContext(ctx, "SELECT id,ts,type,severity,summary,details_json,correlation_key,container_instance_id,resource_id,source FROM events WHERE ts>=? AND ts<=? ORDER BY ts,id LIMIT ?", from.UnixMilli(), to.UnixMilli(), limit)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []HistoricalEvent{}
	for rows.Next() {
		var v HistoricalEvent
		var ms int64
		var details, correlation, container, resource sql.NullString
		if e = rows.Scan(&v.ID, &ms, &v.Type, &v.Severity, &v.Summary, &details, &correlation, &container, &resource, &v.Source); e != nil {
			return nil, e
		}
		v.At = time.UnixMilli(ms).UTC()
		if details.Valid {
			v.Details = &details.String
		}
		if correlation.Valid {
			v.CorrelationKey = &correlation.String
		}
		if container.Valid {
			v.ContainerInstance = &container.String
		}
		if resource.Valid {
			v.ResourceID = &resource.String
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// EventQuery filters the historical event feed. Zero values mean "any".
type EventQuery struct {
	From, To   time.Time
	ResourceID string
	Severity   string
	Types      []string
	Limit      int
	// BeforeID paginates: only events strictly older than this event id.
	BeforeID string
}

const maxEventLimit = 500

// QueryEvents returns events newest first, honouring filters and the cursor.
func (m *Manager) QueryEvents(ctx context.Context, q EventQuery) ([]HistoricalEvent, error) {
	limit := q.Limit
	if limit < 1 || limit > maxEventLimit {
		limit = 100
	}
	query := "SELECT id,ts,type,severity,summary,details_json,correlation_key,container_instance_id,resource_id,source FROM events WHERE ts>=? AND ts<=?"
	args := []any{q.From.UnixMilli(), q.To.UnixMilli()}
	if q.ResourceID != "" {
		query += " AND resource_id=?"
		args = append(args, q.ResourceID)
	}
	if q.Severity != "" {
		query += " AND severity=?"
		args = append(args, q.Severity)
	}
	if len(q.Types) > 0 {
		query += " AND type IN (?" + strings.Repeat(",?", len(q.Types)-1) + ")"
		for _, value := range q.Types {
			args = append(args, value)
		}
	}
	if q.BeforeID != "" {
		var cursorTS int64
		if err := m.db.QueryRowContext(ctx, "SELECT ts FROM events WHERE id=?", q.BeforeID).Scan(&cursorTS); err == nil {
			query += " AND (ts<? OR (ts=? AND id<?))"
			args = append(args, cursorTS, cursorTS, q.BeforeID)
		}
	}
	query += " ORDER BY ts DESC,id DESC LIMIT ?"
	args = append(args, limit)
	rows, err := m.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []HistoricalEvent{}
	for rows.Next() {
		var v HistoricalEvent
		var ms int64
		var details, correlation, containerInstance, resourceIDVal sql.NullString
		if err = rows.Scan(&v.ID, &ms, &v.Type, &v.Severity, &v.Summary, &details, &correlation, &containerInstance, &resourceIDVal, &v.Source); err != nil {
			return nil, err
		}
		v.At = time.UnixMilli(ms).UTC()
		if details.Valid {
			v.Details = &details.String
		}
		if correlation.Valid {
			v.CorrelationKey = &correlation.String
		}
		if containerInstance.Valid {
			v.ContainerInstance = &containerInstance.String
		}
		if resourceIDVal.Valid {
			v.ResourceID = &resourceIDVal.String
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
