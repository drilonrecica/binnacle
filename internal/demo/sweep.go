// SPDX-License-Identifier: AGPL-3.0-only
package demo

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/drilonrecica/binnacle/internal/storage"
	"github.com/drilonrecica/binnacle/internal/sweep"
)

// DomainProber answers the sweep without network access. The answer follows
// the domain name, so the demo shows every state the Domains tab can render:
// a healthy domain, a certificate about to expire, a Cloudflare 525, a domain
// without mail authentication, one without security headers and one that
// does not resolve.
type DomainProber struct{}

func (DomainProber) Probe(_ context.Context, d sweep.Domain) sweep.Result {
	now := time.Now().UTC()
	notAfter := now.Add(62 * 24 * time.Hour).Truncate(time.Hour)
	days := 62
	r := sweep.Result{DomainID: d.ID, CheckedAt: now, HTTPStatus: 200, FinalURL: "https://" + d.Domain + "/", CertIssuer: "CN=R11,O=Let's Encrypt,C=US", CertNotAfter: &notAfter, CertDaysLeft: &days, NSPresent: true, Resolves: true, NS: []string{"ns1.demo.invalid", "ns2.demo.invalid"}, Addresses: []string{"203.0.113.10", "2001:db8::10"}, SPF: "v=spf1 include:_spf.demo.invalid -all", DMARCPolicy: "reject", HSTS: true, CSP: true}
	switch {
	case strings.HasPrefix(d.Domain, "expiring."):
		soon := now.Add(5*24*time.Hour + time.Hour).Truncate(time.Hour)
		five := 5
		r.CertNotAfter, r.CertDaysLeft = &soon, &five
	case strings.HasPrefix(d.Domain, "origin-down."):
		r.HTTPStatus = 525
	case strings.HasPrefix(d.Domain, "nomail."):
		r.SPF, r.DMARCPolicy = "", ""
	case strings.HasPrefix(d.Domain, "legacy."):
		r.HSTS, r.CSP = false, false
		r.DMARCPolicy = "none"
	case strings.HasPrefix(d.Domain, "parked."):
		r.HTTPStatus, r.FinalURL, r.CertIssuer, r.CertNotAfter, r.CertDaysLeft = 0, "", "", nil, nil
		r.Resolves, r.Addresses = false, nil
		r.FailureCode = sweep.FailureDNS
		r.HSTS, r.CSP = false, false
	}
	return r
}

// DemoDomains are the domains demo mode seeds.
var DemoDomains = []string{"shop.demo.invalid", "blog.demo.invalid", "expiring.demo.invalid", "origin-down.demo.invalid", "nomail.demo.invalid", "legacy.demo.invalid", "parked.demo.invalid"}

// SeedDomains adds the demo domains with a first result each, so the tab is
// populated before the scheduler's first sweep. Existing rows are kept.
func SeedDomains(ctx context.Context, db *storage.DB, now time.Time) error {
	repo := sweep.NewRepository(db)
	prober := DomainProber{}
	for i, name := range DemoDomains {
		d := sweep.Domain{ID: "demo-domain-" + name, Domain: name, Interval: time.Hour, Enabled: true}
		switch err := repo.Create(ctx, d); {
		case err == nil:
		case errors.Is(err, sweep.ErrDuplicate):
			continue
		default:
			return err
		}
		result := prober.Probe(ctx, d)
		// Spread the first results over the past hour so "last checked" and
		// the next runs differ per domain.
		result.CheckedAt = now.Add(-time.Duration(i*7) * time.Minute)
		if _, err := repo.SaveResult(ctx, d, result); err != nil {
			return err
		}
	}
	return nil
}
