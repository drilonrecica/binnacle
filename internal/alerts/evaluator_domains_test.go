// SPDX-License-Identifier: AGPL-3.0-only
package alerts_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/drilonrecica/binnacle/internal/alerts"
	"github.com/drilonrecica/binnacle/internal/metrics"
	"github.com/drilonrecica/binnacle/internal/storage"
)

type domainState struct {
	status                 int
	failure                string
	certDays               *int
	nsPresent, resolves    bool
	spf, dmarc             string
	hsts, csp              bool
	consecutiveFailureRuns int
}

func seedDomain(t *testing.T, store *storage.Manager, name string, s domainState, at time.Time) {
	t.Helper()
	if _, err := store.DB().Exec(`INSERT OR IGNORE INTO sweep_domains(id,domain,interval_seconds,enabled,created_at,updated_at)VALUES(?,?,3600,1,?,?)`, "id-"+name, name, at.Unix(), at.Unix()); err != nil {
		t.Fatal(err)
	}
	var status, failure, days any
	if s.status != 0 {
		status = s.status
	}
	if s.failure != "" {
		failure = s.failure
	}
	if s.certDays != nil {
		days = *s.certDays
	}
	if _, err := store.DB().Exec(`INSERT OR REPLACE INTO sweep_state(domain_id,checked_at,next_run_at,http_status,failure_code,cert_days_left,ns_present,resolves,spf,dmarc_policy,hsts,csp,fingerprint,details_json,consecutive_failures,updated_at)VALUES(?,?,?,?,?,?,?,?,?,?,?,?,'fp','{}',?,?)`,
		"id-"+name, at.Unix(), at.Add(time.Hour).Unix(), status, failure, days, s.nsPresent, s.resolves, nullable(s.spf), nullable(s.dmarc), s.hsts, s.csp, s.consecutiveFailureRuns, at.Unix()); err != nil {
		t.Fatal(err)
	}
}

func nullable(v string) any {
	if v == "" {
		return nil
	}
	return v
}

func phaseOf(t *testing.T, store *storage.Manager, family, target string) string {
	t.Helper()
	var phase string
	err := store.DB().QueryRow(`SELECT phase FROM alert_evaluation_state WHERE dedup_key=?`, family+":"+target).Scan(&phase)
	if errors.Is(err, sql.ErrNoRows) {
		return "none"
	}
	if err != nil {
		t.Fatal(err)
	}
	return phase
}

func TestEvaluatorDomainFamilies(t *testing.T) {
	ctx := context.Background()
	store, repo := healthStore(t)
	engine := metrics.NewEngine(4)
	now := time.Date(2026, 11, 25, 12, 0, 0, 0, time.UTC)
	evaluator := alerts.NewEvaluator(repo, engine)
	evaluator.Now = func() time.Time { return now }
	tick := func() {
		t.Helper()
		if err := evaluator.Evaluate(ctx); err != nil {
			t.Fatal(err)
		}
		now = now.Add(10 * time.Second)
	}

	// Unreachable, mail authentication missing; the headers and the
	// certificate were not observed, so those families hold.
	seedDomain(t, store, "example.com", domainState{failure: "timeout", nsPresent: true, resolves: true}, now)
	tick()
	tick()
	for family, want := range map[string]string{
		alerts.FamilyDomainUnreachable:     "firing",
		alerts.FamilyDomainMailAuthMissing: "firing",
		alerts.FamilyDomainDNSMissing:      "none",
		alerts.FamilyDomainHeadersMissing:  "none",
		alerts.FamilyDomainCertExpiring:    "none",
	} {
		if got := phaseOf(t, store, family, "example.com"); got != want {
			t.Fatalf("%s phase=%s want %s", family, got, want)
		}
	}
	var targetType, severity string
	if err := store.DB().QueryRow(`SELECT target_type,severity FROM alerts WHERE family=? AND status='firing'`, alerts.FamilyDomainUnreachable).Scan(&targetType, &severity); err != nil || targetType != "domain" || severity != "critical" {
		t.Fatalf("unreachable alert target_type=%s severity=%s err=%v", targetType, severity, err)
	}

	// A 525 is a status, and still unreachable.
	seedDomain(t, store, "example.com", domainState{status: 525, nsPresent: true, resolves: true, hsts: true, csp: true}, now)
	tick()
	if got := phaseOf(t, store, alerts.FamilyDomainUnreachable, "example.com"); got != "firing" {
		t.Fatalf("525 left unreachable in phase %s", got)
	}

	// Recovered, with a certificate about to expire and both headers.
	five := 5
	seedDomain(t, store, "example.com", domainState{status: 200, certDays: &five, nsPresent: true, resolves: true, spf: "v=spf1 -all", dmarc: "reject", hsts: true, csp: true}, now)
	tick()
	tick()
	for family, want := range map[string]string{
		alerts.FamilyDomainUnreachable:     "healthy",
		alerts.FamilyDomainMailAuthMissing: "healthy",
		alerts.FamilyDomainCertExpiring:    "firing",
		alerts.FamilyDomainHeadersMissing:  "none",
	} {
		if got := phaseOf(t, store, family, "example.com"); got != want {
			t.Fatalf("%s phase=%s want %s", family, got, want)
		}
	}
	var resolved int
	if err := store.DB().QueryRow(`SELECT COUNT(*) FROM alerts WHERE family=? AND status='resolved'`, alerts.FamilyDomainUnreachable).Scan(&resolved); err != nil || resolved != 1 {
		t.Fatalf("resolved unreachable alerts=%d err=%v", resolved, err)
	}
	var observed float64
	if err := store.DB().QueryRow(`SELECT observed_value FROM alerts WHERE family=? AND status='firing'`, alerts.FamilyDomainCertExpiring).Scan(&observed); err != nil || observed != 5 {
		t.Fatalf("certificate alert observed=%v err=%v", observed, err)
	}

	// Headers dropped, DNS gone: the DNS family fires; mail auth holds
	// without NS records instead of piling on.
	seedDomain(t, store, "example.com", domainState{status: 200, certDays: &five, nsPresent: false, resolves: false}, now)
	tick()
	tick()
	if got := phaseOf(t, store, alerts.FamilyDomainDNSMissing, "example.com"); got != "firing" {
		t.Fatalf("dns missing phase=%s", got)
	}
	if got := phaseOf(t, store, alerts.FamilyDomainHeadersMissing, "example.com"); got != "firing" {
		t.Fatalf("headers missing phase=%s", got)
	}
	if got := phaseOf(t, store, alerts.FamilyDomainMailAuthMissing, "example.com"); got != "healthy" {
		t.Fatalf("mail auth without NS records moved to %s", got)
	}

	// A removed domain resolves whatever was firing for it.
	if _, err := store.DB().Exec(`DELETE FROM sweep_domains WHERE domain='example.com'`); err != nil {
		t.Fatal(err)
	}
	tick()
	tick()
	for _, family := range []string{alerts.FamilyDomainCertExpiring, alerts.FamilyDomainDNSMissing, alerts.FamilyDomainHeadersMissing} {
		if got := phaseOf(t, store, family, "example.com"); got != "healthy" {
			t.Fatalf("%s phase after removal=%s", family, got)
		}
	}
	var firing int
	if err := store.DB().QueryRow(`SELECT COUNT(*) FROM alerts WHERE target_type='domain' AND status='firing'`).Scan(&firing); err != nil || firing != 0 {
		t.Fatalf("firing domain alerts after removal=%d err=%v", firing, err)
	}
}

func TestDomainDefaultRules(t *testing.T) {
	byFamily := map[string]alerts.Rule{}
	for _, r := range alerts.DefaultRules() {
		byFamily[r.Family] = r
	}
	tests := []struct {
		family   string
		severity alerts.Severity
	}{{alerts.FamilyDomainUnreachable, alerts.Critical}, {alerts.FamilyDomainCertExpiring, alerts.Warning}, {alerts.FamilyDomainDNSMissing, alerts.Critical}, {alerts.FamilyDomainMailAuthMissing, alerts.Warning}, {alerts.FamilyDomainHeadersMissing, alerts.Warning}}
	for _, tt := range tests {
		r, ok := byFamily[tt.family]
		if !ok {
			t.Fatalf("missing %s", tt.family)
		}
		if !r.BuiltIn || !r.Enabled || r.Severity != tt.severity || r.ScopeType != "global" {
			t.Fatalf("rule %s=%+v", tt.family, r)
		}
	}
	cert := byFamily[alerts.FamilyDomainCertExpiring]
	if cert.Threshold == nil || *cert.Threshold != 14 || cert.RecoveryThreshold == nil || *cert.RecoveryThreshold != 14 {
		t.Fatalf("certificate rule thresholds=%+v", cert)
	}
}
