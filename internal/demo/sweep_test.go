// SPDX-License-Identifier: AGPL-3.0-only
package demo

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/drilonrecica/binnacle/internal/storage"
	"github.com/drilonrecica/binnacle/internal/sweep"
)

func TestDomainProberCoversEveryState(t *testing.T) {
	prober := DomainProber{}
	probe := func(name string) sweep.Result {
		return prober.Probe(context.Background(), sweep.Domain{ID: name, Domain: name, Interval: time.Hour})
	}
	healthy := probe("shop.demo.invalid")
	if healthy.Failed() || !healthy.HSTS || !healthy.CSP || healthy.SPF == "" || healthy.DMARCPolicy != "reject" || healthy.CertDaysLeft == nil || *healthy.CertDaysLeft <= sweep.CertExpiringDays {
		t.Fatalf("healthy=%+v", healthy)
	}
	if r := probe("expiring.demo.invalid"); r.CertDaysLeft == nil || *r.CertDaysLeft > sweep.CertExpiringDays || r.Failed() {
		t.Fatalf("expiring=%+v", r)
	}
	if r := probe("origin-down.demo.invalid"); r.HTTPStatus != 525 || !r.Failed() || r.FailureCode != "" {
		t.Fatalf("origin down=%+v", r)
	}
	if r := probe("nomail.demo.invalid"); r.SPF != "" || r.DMARCPolicy != "" || r.Failed() {
		t.Fatalf("no mail=%+v", r)
	}
	if r := probe("legacy.demo.invalid"); r.HSTS || r.CSP || r.Failed() {
		t.Fatalf("legacy=%+v", r)
	}
	if r := probe("parked.demo.invalid"); r.Resolves || r.FailureCode != sweep.FailureDNS || !r.NSPresent || r.CertNotAfter != nil {
		t.Fatalf("parked=%+v", r)
	}
	if healthy.Fingerprint != "" {
		t.Fatal("the repository computes the fingerprint; the prober leaves it empty")
	}
}

func TestSeedDomainsIsIdempotent(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	store := storage.New(filepath.Join(dir, "db.sqlite"), filepath.Join(dir, "runtime"))
	if err := store.Open(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	now := time.Date(2026, 11, 25, 12, 0, 0, 0, time.UTC)
	for range 2 {
		if err := SeedDomains(ctx, store.DB(), now); err != nil {
			t.Fatal(err)
		}
	}
	entries, err := sweep.NewRepository(store.DB()).List(ctx, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != len(DemoDomains) {
		t.Fatalf("%d domains after seeding twice, want %d", len(entries), len(DemoDomains))
	}
	for _, entry := range entries {
		if entry.State == nil || entry.State.CheckedAt.After(now) || !entry.State.NextRunAt.After(now.Add(-time.Hour)) {
			t.Fatalf("%s seeded without a recent state: %+v", entry.Domain.Domain, entry.State)
		}
	}
	var events int
	if err = store.DB().QueryRow(`SELECT COUNT(*) FROM events WHERE type='domain_change'`).Scan(&events); err != nil || events != 0 {
		t.Fatalf("seeding wrote %d change events err=%v", events, err)
	}
}
