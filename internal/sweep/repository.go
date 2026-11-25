// SPDX-License-Identifier: AGPL-3.0-only

package sweep

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/drilonrecica/binnacle/internal/storage"
)

// ErrDuplicate is returned when a domain is already in the sweep.
var ErrDuplicate = errors.New("domain already exists")

var errUnavailable = errors.New("sweep repository unavailable")

type Repository struct{ db *storage.DB }

func NewRepository(db *storage.DB) *Repository { return &Repository{db: db} }
func (r *Repository) SetDB(db *storage.DB)     { r.db = db }

func (r *Repository) Create(ctx context.Context, d Domain) error {
	if err := d.Validate(); err != nil {
		return err
	}
	if r.db == nil {
		return errUnavailable
	}
	now := time.Now().UTC().Unix()
	_, err := r.db.ExecContext(ctx, `INSERT INTO sweep_domains(id,domain,interval_seconds,enabled,created_at,updated_at) VALUES(?,?,?,?,?,?)`, d.ID, d.Domain, int(d.Interval/time.Second), d.Enabled, now, now)
	if err != nil && strings.Contains(err.Error(), "UNIQUE") {
		return ErrDuplicate
	}
	return err
}

func (r *Repository) Update(ctx context.Context, d Domain) error {
	if err := d.Validate(); err != nil {
		return err
	}
	if r.db == nil {
		return errUnavailable
	}
	res, err := r.db.ExecContext(ctx, `UPDATE sweep_domains SET domain=?,interval_seconds=?,enabled=?,updated_at=? WHERE id=?`, d.Domain, int(d.Interval/time.Second), d.Enabled, time.Now().UTC().Unix(), d.ID)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return ErrDuplicate
		}
		return err
	}
	return affected(res)
}

func (r *Repository) Delete(ctx context.Context, id string) error {
	if r.db == nil {
		return errUnavailable
	}
	res, err := r.db.ExecContext(ctx, `DELETE FROM sweep_domains WHERE id=?`, id)
	if err != nil {
		return err
	}
	return affected(res)
}

func affected(res sql.Result) error {
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

const entryColumns = `d.id,d.domain,d.interval_seconds,d.enabled,d.created_at,d.updated_at,
	s.checked_at,s.next_run_at,s.http_status,s.failure_code,s.cert_not_after,s.cert_days_left,s.ns_present,s.resolves,s.spf,s.dmarc_policy,s.hsts,s.csp,s.fingerprint,s.details_json,s.consecutive_failures`

func (r *Repository) Get(ctx context.Context, id string) (Entry, error) {
	if r.db == nil {
		return Entry{}, errUnavailable
	}
	row := r.db.QueryRowContext(ctx, `SELECT `+entryColumns+` FROM sweep_domains d LEFT JOIN sweep_state s ON s.domain_id=d.id WHERE d.id=?`, id)
	return scanEntry(row)
}

// List returns the domains in alphabetical order with their latest state.
func (r *Repository) List(ctx context.Context, limit, offset int) ([]Entry, error) {
	if r.db == nil {
		return nil, errUnavailable
	}
	if limit < 1 || limit > 500 {
		limit = 200
	}
	if offset < 0 {
		offset = 0
	}
	rows, err := r.db.QueryContext(ctx, `SELECT `+entryColumns+` FROM sweep_domains d LEFT JOIN sweep_state s ON s.domain_id=d.id ORDER BY d.domain LIMIT ? OFFSET ?`, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Entry{}
	for rows.Next() {
		entry, err := scanEntry(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, entry)
	}
	return out, rows.Err()
}

type scanner interface{ Scan(...any) error }

// details carries the parts of a result that have no column of their own.
type details struct {
	NS        []string `json:"ns,omitempty"`
	Addresses []string `json:"addresses,omitempty"`
	Issuer    string   `json:"issuer,omitempty"`
	FinalURL  string   `json:"finalUrl,omitempty"`
}

func scanEntry(s scanner) (Entry, error) {
	var e Entry
	var interval, created, updated int64
	var checked, next, status, notAfter, daysLeft, failures sql.NullInt64
	var failure, spf, dmarc, fingerprint, detailsJSON sql.NullString
	var nsPresent, resolves, hsts, csp sql.NullBool
	err := s.Scan(&e.ID, &e.Domain.Domain, &interval, &e.Enabled, &created, &updated, &checked, &next, &status, &failure, &notAfter, &daysLeft, &nsPresent, &resolves, &spf, &dmarc, &hsts, &csp, &fingerprint, &detailsJSON, &failures)
	if err != nil {
		return Entry{}, err
	}
	e.Interval = time.Duration(interval) * time.Second
	e.CreatedAt = time.Unix(created, 0).UTC()
	e.UpdatedAt = time.Unix(updated, 0).UTC()
	if !checked.Valid {
		return e, nil
	}
	state := &State{Result: Result{DomainID: e.ID, CheckedAt: time.Unix(checked.Int64, 0).UTC(), HTTPStatus: int(status.Int64), FailureCode: FailureCode(failure.String), NSPresent: nsPresent.Bool, Resolves: resolves.Bool, SPF: spf.String, DMARCPolicy: dmarc.String, HSTS: hsts.Bool, CSP: csp.Bool, Fingerprint: fingerprint.String}, NextRunAt: time.Unix(next.Int64, 0).UTC(), ConsecutiveFailures: int(failures.Int64)}
	if notAfter.Valid {
		v := time.Unix(notAfter.Int64, 0).UTC()
		state.CertNotAfter = &v
	}
	if daysLeft.Valid {
		v := int(daysLeft.Int64)
		state.CertDaysLeft = &v
	}
	if detailsJSON.Valid && detailsJSON.String != "" {
		var d details
		if err = json.Unmarshal([]byte(detailsJSON.String), &d); err != nil {
			return Entry{}, fmt.Errorf("decode sweep details for %s: %w", e.ID, err)
		}
		state.NS, state.Addresses, state.CertIssuer, state.FinalURL = d.NS, d.Addresses, d.Issuer, d.FinalURL
	}
	e.State = state
	return e, nil
}

// Due returns up to limit enabled domains whose next run is due, skipping
// the ids in exclude: domains already queued or running keep a past
// next_run_at until their result is saved (EVT-4).
func (r *Repository) Due(ctx context.Context, now time.Time, limit int, exclude ...string) ([]Domain, error) {
	if r.db == nil {
		return nil, errUnavailable
	}
	if limit < 1 || limit > 1000 {
		return nil, fmt.Errorf("invalid due-domain limit")
	}
	if len(exclude) > 1000 {
		return nil, fmt.Errorf("too many excluded domains")
	}
	excluded := ""
	args := []any{now.Unix()}
	if len(exclude) > 0 {
		excluded = " AND d.id NOT IN (?" + strings.Repeat(",?", len(exclude)-1) + ")"
		for _, id := range exclude {
			args = append(args, id)
		}
	}
	args = append(args, limit)
	rows, err := r.db.QueryContext(ctx, `SELECT d.id,d.domain,d.interval_seconds,d.enabled,d.created_at,d.updated_at FROM sweep_domains d LEFT JOIN sweep_state s ON s.domain_id=d.id
	WHERE d.enabled=1 AND (s.next_run_at IS NULL OR s.next_run_at<=?)`+excluded+` ORDER BY COALESCE(s.next_run_at,0),d.domain LIMIT ?`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Domain{}
	for rows.Next() {
		var d Domain
		var interval, created, updated int64
		if err = rows.Scan(&d.ID, &d.Domain, &interval, &d.Enabled, &created, &updated); err != nil {
			return nil, err
		}
		d.Interval = time.Duration(interval) * time.Second
		d.CreatedAt = time.Unix(created, 0).UTC()
		d.UpdatedAt = time.Unix(updated, 0).UTC()
		out = append(out, d)
	}
	return out, rows.Err()
}

// SaveResult stores the latest result of a domain and schedules the next
// run. When the fingerprint differs from the stored one, a domain_change
// event describing the difference is written in the same transaction; the
// returned list is that description, empty when nothing watched changed.
func (r *Repository) SaveResult(ctx context.Context, d Domain, result Result) ([]string, error) {
	if r.db == nil {
		return nil, errUnavailable
	}
	if result.CheckedAt.IsZero() {
		result.CheckedAt = time.Now().UTC()
	}
	if result.Fingerprint == "" {
		result.Fingerprint = result.fingerprint()
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	previous, stored, err := previousResult(ctx, tx, d.ID)
	if err != nil {
		return nil, err
	}
	detailsJSON, err := json.Marshal(details{NS: result.NS, Addresses: result.Addresses, Issuer: result.CertIssuer, FinalURL: result.FinalURL})
	if err != nil {
		return nil, fmt.Errorf("encode sweep details: %w", err)
	}
	failed := 0
	if result.Failed() {
		failed = 1
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO sweep_state(domain_id,checked_at,next_run_at,http_status,failure_code,cert_not_after,cert_days_left,ns_present,resolves,spf,dmarc_policy,hsts,csp,fingerprint,details_json,consecutive_failures,updated_at)
	VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
	ON CONFLICT(domain_id) DO UPDATE SET checked_at=excluded.checked_at,next_run_at=excluded.next_run_at,http_status=excluded.http_status,failure_code=excluded.failure_code,cert_not_after=excluded.cert_not_after,cert_days_left=excluded.cert_days_left,
	ns_present=excluded.ns_present,resolves=excluded.resolves,spf=excluded.spf,dmarc_policy=excluded.dmarc_policy,hsts=excluded.hsts,csp=excluded.csp,fingerprint=excluded.fingerprint,details_json=excluded.details_json,
	consecutive_failures=CASE WHEN excluded.consecutive_failures=0 THEN 0 ELSE sweep_state.consecutive_failures+1 END,updated_at=excluded.updated_at`,
		d.ID, result.CheckedAt.Unix(), result.CheckedAt.Add(d.Interval).Unix(), nullInt(result.HTTPStatus), nullString(string(result.FailureCode)), nullTime(result.CertNotAfter), nullIntPtr(result.CertDaysLeft), result.NSPresent, result.Resolves, nullString(result.SPF), nullString(result.DMARCPolicy), result.HSTS, result.CSP, result.Fingerprint, string(detailsJSON), failed, result.CheckedAt.Unix())
	if err != nil {
		return nil, err
	}
	var changes []string
	if stored && previous.Fingerprint != "" && previous.Fingerprint != result.Fingerprint {
		changes = Changes(previous, result)
		if len(changes) > 0 {
			if err = insertChangeEvent(ctx, tx, d, result, changes); err != nil {
				return nil, err
			}
		}
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return changes, nil
}

// previousResult reads the stored result of a domain inside a transaction.
func previousResult(ctx context.Context, tx *sql.Tx, id string) (Result, bool, error) {
	var status sql.NullInt64
	var notAfter sql.NullInt64
	var failure, spf, dmarc, fingerprint, detailsJSON sql.NullString
	var previous Result
	err := tx.QueryRowContext(ctx, `SELECT http_status,failure_code,cert_not_after,ns_present,resolves,spf,dmarc_policy,hsts,csp,fingerprint,details_json FROM sweep_state WHERE domain_id=?`, id).Scan(&status, &failure, &notAfter, &previous.NSPresent, &previous.Resolves, &spf, &dmarc, &previous.HSTS, &previous.CSP, &fingerprint, &detailsJSON)
	if errors.Is(err, sql.ErrNoRows) {
		return Result{}, false, nil
	}
	if err != nil {
		return Result{}, false, err
	}
	previous.HTTPStatus = int(status.Int64)
	previous.FailureCode = FailureCode(failure.String)
	previous.SPF, previous.DMARCPolicy, previous.Fingerprint = spf.String, dmarc.String, fingerprint.String
	if notAfter.Valid {
		v := time.Unix(notAfter.Int64, 0).UTC()
		previous.CertNotAfter = &v
	}
	if detailsJSON.Valid && detailsJSON.String != "" {
		var d details
		if err = json.Unmarshal([]byte(detailsJSON.String), &d); err != nil {
			return Result{}, false, fmt.Errorf("decode sweep details for %s: %w", id, err)
		}
		previous.NS, previous.Addresses, previous.CertIssuer, previous.FinalURL = d.NS, d.Addresses, d.Issuer, d.FinalURL
	}
	return previous, true, nil
}

func insertChangeEvent(ctx context.Context, tx *sql.Tx, d Domain, result Result, changes []string) error {
	severity := "info"
	if result.Failed() {
		severity = "warning"
	}
	payload, err := json.Marshal(map[string]any{"domain": d.Domain, "domainId": d.ID, "changes": changes, "fingerprint": result.Fingerprint})
	if err != nil {
		return fmt.Errorf("encode sweep change: %w", err)
	}
	summary := d.Domain + " changed: " + strings.Join(changes, "; ")
	_, err = tx.ExecContext(ctx, `INSERT INTO events(id,ts,type,severity,summary,details_json,correlation_key,source,created_at)VALUES(?,?,?,?,?,?,?,?,?)`, "sweep-"+randomID(), result.CheckedAt.UnixMilli(), EventType, severity, summary, string(payload), "domain:"+d.Domain, "sweep", result.CheckedAt.UnixMilli())
	return err
}

func randomID() string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func nullString(v string) any {
	if v == "" {
		return nil
	}
	return v
}
func nullInt(v int) any {
	if v == 0 {
		return nil
	}
	return v
}
func nullIntPtr(v *int) any {
	if v == nil {
		return nil
	}
	return *v
}
func nullTime(v *time.Time) any {
	if v == nil {
		return nil
	}
	return v.Unix()
}
