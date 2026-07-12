// SPDX-License-Identifier: AGPL-3.0-only

package checks

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

type Repository struct{ db *sql.DB }

func NewRepository(db *sql.DB) *Repository { return &Repository{db: db} }
func (r *Repository) SetDB(db *sql.DB)     { r.db = db }

func (r *Repository) Create(ctx context.Context, c Check) error {
	if err := c.Validate(); err != nil {
		return err
	}
	if r.db == nil {
		return errors.New("checks repository unavailable")
	}
	now := time.Now().UTC()
	if c.ID == "" {
		return errors.New("check id is required")
	}
	_, err := r.db.ExecContext(ctx, `INSERT INTO health_checks(id,resource_id,name,url,method,interval_seconds,timeout_seconds,expected_status_min,expected_status_max,body_substring,required,enabled,created_at,updated_at)
		VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, c.ID, c.ResourceID, c.Name, c.URL, c.Method, int(c.Interval/time.Second), int(c.Timeout/time.Second), c.ExpectedStatusMin, c.ExpectedStatusMax, nullString(c.BodySubstring), c.Required, c.Enabled, now.Unix(), now.Unix())
	return err
}

func (r *Repository) Update(ctx context.Context, c Check) error {
	if err := c.Validate(); err != nil {
		return err
	}
	res, err := r.db.ExecContext(ctx, `UPDATE health_checks SET resource_id=?,name=?,url=?,method=?,interval_seconds=?,timeout_seconds=?,expected_status_min=?,expected_status_max=?,body_substring=?,required=?,enabled=?,updated_at=? WHERE id=?`, c.ResourceID, c.Name, c.URL, c.Method, int(c.Interval/time.Second), int(c.Timeout/time.Second), c.ExpectedStatusMin, c.ExpectedStatusMax, nullString(c.BodySubstring), c.Required, c.Enabled, time.Now().UTC().Unix(), c.ID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}
func (r *Repository) Delete(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM health_checks WHERE id=?`, id)
	return err
}
func (r *Repository) Get(ctx context.Context, id string) (Check, error) {
	row := r.db.QueryRowContext(ctx, `SELECT id,resource_id,name,url,method,interval_seconds,timeout_seconds,expected_status_min,expected_status_max,COALESCE(body_substring,''),required,enabled,created_at,updated_at FROM health_checks WHERE id=?`, id)
	return scanCheck(row)
}
func (r *Repository) List(ctx context.Context, limit, offset int) ([]Check, error) {
	if limit < 1 || limit > 100 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	rows, err := r.db.QueryContext(ctx, `SELECT id,resource_id,name,url,method,interval_seconds,timeout_seconds,expected_status_min,expected_status_max,COALESCE(body_substring,''),required,enabled,created_at,updated_at FROM health_checks ORDER BY created_at DESC LIMIT ? OFFSET ?`, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Check{}
	for rows.Next() {
		c, err := scanCheck(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

type scanner interface{ Scan(...any) error }

func scanCheck(s scanner) (Check, error) {
	var c Check
	var interval, timeout, created, updated int64
	err := s.Scan(&c.ID, &c.ResourceID, &c.Name, &c.URL, &c.Method, &interval, &timeout, &c.ExpectedStatusMin, &c.ExpectedStatusMax, &c.BodySubstring, &c.Required, &c.Enabled, &created, &updated)
	c.Interval = time.Duration(interval) * time.Second
	c.Timeout = time.Duration(timeout) * time.Second
	c.CreatedAt = time.Unix(created, 0).UTC()
	c.UpdatedAt = time.Unix(updated, 0).UTC()
	return c, err
}

func (r *Repository) Due(ctx context.Context, now time.Time, limit int) ([]Check, error) {
	if limit < 1 || limit > 1000 {
		return nil, fmt.Errorf("invalid due-check limit")
	}
	rows, err := r.db.QueryContext(ctx, `SELECT c.id,c.resource_id,c.name,c.url,c.method,c.interval_seconds,c.timeout_seconds,c.expected_status_min,c.expected_status_max,COALESCE(c.body_substring,''),c.required,c.enabled,c.created_at,c.updated_at
	FROM health_checks c JOIN resources r ON r.id=c.resource_id LEFT JOIN health_check_state s ON s.check_id=c.id
	WHERE c.enabled=1 AND r.archived_at IS NULL AND r.status NOT IN ('paused','archived') AND (s.next_run_at IS NULL OR s.next_run_at<=?) ORDER BY COALESCE(s.next_run_at,0),c.id LIMIT ?`, now.Unix(), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Check{}
	for rows.Next() {
		c, err := scanCheck(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}
func (r *Repository) SaveResult(ctx context.Context, c Check, result Result) error {
	if result.CheckedAt.IsZero() {
		result.CheckedAt = time.Now().UTC()
	}
	_, err := r.db.ExecContext(ctx, `INSERT INTO health_check_state(check_id,status,failure_code,latency_ms,checked_at,consecutive_successes,consecutive_failures,next_run_at,updated_at)
	VALUES(?,?,?,?,?,CASE WHEN ?='success' THEN 1 ELSE 0 END,CASE WHEN ?='failure' THEN 1 ELSE 0 END,?,?)
	ON CONFLICT(check_id) DO UPDATE SET status=excluded.status,failure_code=excluded.failure_code,latency_ms=excluded.latency_ms,checked_at=excluded.checked_at,
	consecutive_successes=CASE WHEN excluded.status='success' THEN health_check_state.consecutive_successes+1 ELSE 0 END,
	consecutive_failures=CASE WHEN excluded.status='failure' THEN health_check_state.consecutive_failures+1 ELSE 0 END,next_run_at=excluded.next_run_at,updated_at=excluded.updated_at`, result.CheckID, result.Status, nullString(string(result.FailureCode)), result.LatencyMS, result.CheckedAt.Unix(), result.Status, result.Status, result.CheckedAt.Add(c.Interval).Unix(), result.CheckedAt.Unix())
	return err
}
func nullString(v string) any {
	if v == "" {
		return nil
	}
	return v
}
