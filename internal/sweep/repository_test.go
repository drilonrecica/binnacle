// SPDX-License-Identifier: AGPL-3.0-only
package sweep

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/drilonrecica/binnacle/internal/storage"
)

func sweepRepository(t *testing.T, domains ...string) (*storage.Manager, *Repository) {
	t.Helper()
	ctx := context.Background()
	dir := t.TempDir()
	store := storage.New(filepath.Join(dir, "db.sqlite"), filepath.Join(dir, "runtime"))
	if err := store.Open(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	repo := NewRepository(store.DB())
	for _, name := range domains {
		if err := repo.Create(ctx, Domain{ID: "id-" + name, Domain: name, Interval: time.Hour, Enabled: true}); err != nil {
			t.Fatal(err)
		}
	}
	return store, repo
}

func healthyResult(id string, at time.Time) Result {
	notAfter := at.Add(60 * 24 * time.Hour)
	days := 60
	r := Result{DomainID: id, CheckedAt: at, HTTPStatus: 200, FinalURL: "https://example.com/", CertIssuer: "CN=R11", CertNotAfter: &notAfter, CertDaysLeft: &days, NSPresent: true, Resolves: true, NS: []string{"ns1.example.net", "ns2.example.net"}, Addresses: []string{"93.184.216.34"}, SPF: "v=spf1 -all", DMARCPolicy: "reject", HSTS: true, CSP: true}
	r.Fingerprint = r.fingerprint()
	return r
}

func TestRepositoryCreateListGetUpdateDelete(t *testing.T) {
	ctx := context.Background()
	_, repo := sweepRepository(t, "zeta.example", "alpha.example")
	entries, err := repo.List(ctx, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 || entries[0].Domain.Domain != "alpha.example" || entries[1].Domain.Domain != "zeta.example" {
		t.Fatalf("entries=%+v want alphabetical", entries)
	}
	if entries[0].State != nil {
		t.Fatalf("state before any run=%+v", entries[0].State)
	}
	if err = repo.Create(ctx, Domain{ID: "dup", Domain: "alpha.example", Interval: time.Hour, Enabled: true}); !errors.Is(err, ErrDuplicate) {
		t.Fatalf("duplicate create err=%v want ErrDuplicate", err)
	}
	if err = repo.Create(ctx, Domain{ID: "bad", Domain: "https://bad.example", Interval: time.Hour}); err == nil {
		t.Fatal("invalid domain was stored")
	}
	entry, err := repo.Get(ctx, "id-alpha.example")
	if err != nil || entry.Domain.Domain != "alpha.example" || !entry.Enabled || entry.Interval != time.Hour {
		t.Fatalf("get=%+v err=%v", entry, err)
	}
	updated := entry.Domain
	updated.Interval = 2 * time.Hour
	updated.Enabled = false
	if err = repo.Update(ctx, updated); err != nil {
		t.Fatal(err)
	}
	entry, err = repo.Get(ctx, "id-alpha.example")
	if err != nil || entry.Interval != 2*time.Hour || entry.Enabled {
		t.Fatalf("after update=%+v err=%v", entry, err)
	}
	if err = repo.Update(ctx, Domain{ID: "missing", Domain: "missing.example", Interval: time.Hour}); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("update missing err=%v", err)
	}
	if err = repo.Delete(ctx, "id-alpha.example"); err != nil {
		t.Fatal(err)
	}
	if _, err = repo.Get(ctx, "id-alpha.example"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("get after delete err=%v", err)
	}
	if err = repo.Delete(ctx, "id-alpha.example"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("second delete err=%v", err)
	}
}

func TestRepositoryDueAndSaveResultRoundTrip(t *testing.T) {
	ctx := context.Background()
	_, repo := sweepRepository(t, "a.example", "b.example")
	disabled, err := repo.Get(ctx, "id-b.example")
	if err != nil {
		t.Fatal(err)
	}
	disabled.Enabled = false
	if err = repo.Update(ctx, disabled.Domain); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 11, 25, 12, 0, 0, 0, time.UTC)
	due, err := repo.Due(ctx, now, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(due) != 1 || due[0].ID != "id-a.example" {
		t.Fatalf("due=%+v want only the enabled domain", due)
	}
	if due, err = repo.Due(ctx, now, 10, "id-a.example"); err != nil || len(due) != 0 {
		t.Fatalf("excluded domain still due: %+v err=%v", due, err)
	}

	result := healthyResult("id-a.example", now)
	changes, err := repo.SaveResult(ctx, due0(t, repo, now), result)
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 0 {
		t.Fatalf("first result reported changes %q", changes)
	}
	if due, err = repo.Due(ctx, now.Add(59*time.Minute), 10); err != nil || len(due) != 0 {
		t.Fatalf("domain due before its interval: %+v err=%v", due, err)
	}
	if due, err = repo.Due(ctx, now.Add(time.Hour), 10); err != nil || len(due) != 1 {
		t.Fatalf("domain not due after its interval: %+v err=%v", due, err)
	}
	entry, err := repo.Get(ctx, "id-a.example")
	if err != nil || entry.State == nil {
		t.Fatalf("entry=%+v err=%v", entry, err)
	}
	s := entry.State
	if s.HTTPStatus != 200 || s.FailureCode != "" || !s.CheckedAt.Equal(now) || !s.NextRunAt.Equal(now.Add(time.Hour)) || s.ConsecutiveFailures != 0 {
		t.Fatalf("state=%+v", s)
	}
	if s.CertNotAfter == nil || !s.CertNotAfter.Equal(*result.CertNotAfter) || s.CertDaysLeft == nil || *s.CertDaysLeft != 60 || s.CertIssuer != "CN=R11" {
		t.Fatalf("cert state=%+v", s)
	}
	if !s.NSPresent || !s.Resolves || strings.Join(s.NS, ",") != "ns1.example.net,ns2.example.net" || strings.Join(s.Addresses, ",") != "93.184.216.34" {
		t.Fatalf("dns state=%+v", s)
	}
	if s.SPF != "v=spf1 -all" || s.DMARCPolicy != "reject" || !s.HSTS || !s.CSP || s.Fingerprint != result.Fingerprint || s.FinalURL != "https://example.com/" {
		t.Fatalf("state=%+v", s)
	}

	failing := Result{DomainID: "id-a.example", CheckedAt: now.Add(time.Hour), FailureCode: FailureTimeout, NSPresent: true, Resolves: true, NS: result.NS, Addresses: result.Addresses, SPF: result.SPF, DMARCPolicy: result.DMARCPolicy}
	failing.Fingerprint = failing.fingerprint()
	for i := 1; i <= 2; i++ {
		if _, err = repo.SaveResult(ctx, entry.Domain, failing); err != nil {
			t.Fatal(err)
		}
		if entry, err = repo.Get(ctx, "id-a.example"); err != nil || entry.State.ConsecutiveFailures != i {
			t.Fatalf("failures after %d failing results=%+v err=%v", i, entry.State, err)
		}
	}
	if entry.State.CertNotAfter != nil || entry.State.CertDaysLeft != nil || entry.State.HTTPStatus != 0 {
		t.Fatalf("stale certificate kept after a failed run: %+v", entry.State)
	}
	if _, err = repo.SaveResult(ctx, entry.Domain, result); err != nil {
		t.Fatal(err)
	}
	if entry, err = repo.Get(ctx, "id-a.example"); err != nil || entry.State.ConsecutiveFailures != 0 {
		t.Fatalf("failures after a success=%+v err=%v", entry.State, err)
	}
}

func due0(t *testing.T, repo *Repository, now time.Time) Domain {
	t.Helper()
	due, err := repo.Due(context.Background(), now, 1)
	if err != nil || len(due) != 1 {
		t.Fatalf("due=%+v err=%v", due, err)
	}
	return due[0]
}

func TestRepositoryWritesChangeEventWhenFingerprintMoves(t *testing.T) {
	ctx := context.Background()
	store, repo := sweepRepository(t, "example.com")
	now := time.Date(2026, 11, 25, 12, 0, 0, 0, time.UTC)
	d := due0(t, repo, now)
	first := healthyResult(d.ID, now)
	countEvents := func() int {
		t.Helper()
		var n int
		if err := store.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM events WHERE type=?`, EventType).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	if _, err := repo.SaveResult(ctx, d, first); err != nil {
		t.Fatal(err)
	}
	same := first
	same.CheckedAt = now.Add(time.Hour)
	days := 59
	same.CertDaysLeft = &days
	if _, err := repo.SaveResult(ctx, d, same); err != nil {
		t.Fatal(err)
	}
	if n := countEvents(); n != 0 {
		t.Fatalf("%d events after an unchanged run", n)
	}
	moved := first
	moved.CheckedAt = now.Add(2 * time.Hour)
	moved.HTTPStatus = 525
	moved.DMARCPolicy = ""
	moved.Fingerprint = moved.fingerprint()
	changes, err := repo.SaveResult(ctx, d, moved)
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 2 {
		t.Fatalf("changes=%q", changes)
	}
	if n := countEvents(); n != 1 {
		t.Fatalf("%d events after a changed run, want 1", n)
	}
	var ts int64
	var severity, summary, correlation, source, details string
	if err = store.DB().QueryRowContext(ctx, `SELECT ts,severity,summary,correlation_key,source,details_json FROM events WHERE type=?`, EventType).Scan(&ts, &severity, &summary, &correlation, &source, &details); err != nil {
		t.Fatal(err)
	}
	if ts != moved.CheckedAt.UnixMilli() || severity != "warning" || source != "sweep" || correlation != "domain:example.com" {
		t.Fatalf("event ts=%d severity=%s source=%s correlation=%s", ts, severity, source, correlation)
	}
	if !strings.Contains(summary, "example.com") || !strings.Contains(summary, "HTTP 200 → 525") || !strings.Contains(summary, "DMARC policy reject → missing") {
		t.Fatalf("summary=%q", summary)
	}
	if !strings.Contains(details, `"changes"`) || !strings.Contains(details, `"domain":"example.com"`) {
		t.Fatalf("details=%s", details)
	}
}

func TestRepositoryWithoutDatabase(t *testing.T) {
	repo := NewRepository(nil)
	if err := repo.Create(context.Background(), Domain{ID: "x", Domain: "example.com", Interval: time.Hour}); err == nil {
		t.Fatal("create without a database succeeded")
	}
	if _, err := repo.List(context.Background(), 0, 0); err == nil {
		t.Fatal("list without a database succeeded")
	}
}
