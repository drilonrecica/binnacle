// SPDX-License-Identifier: AGPL-3.0-only
package sweep

import (
	"testing"
	"time"
)

func TestNormalizeDomain(t *testing.T) {
	tests := []struct{ in, want string }{
		{"Example.COM", "example.com"},
		{"  example.com.  ", "example.com"},
		{"shop.example.co.uk", "shop.example.co.uk"},
	}
	for _, tt := range tests {
		if got := NormalizeDomain(tt.in); got != tt.want {
			t.Fatalf("NormalizeDomain(%q)=%q want %q", tt.in, got, tt.want)
		}
	}
}

func TestDomainValidate(t *testing.T) {
	valid := Domain{ID: "d", Domain: "example.com", Interval: time.Hour}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid domain rejected: %v", err)
	}
	tests := []struct {
		name   string
		domain string
		every  time.Duration
	}{
		{"empty", "", time.Hour},
		{"scheme", "https://example.com", time.Hour},
		{"path", "example.com/health", time.Hour},
		{"port", "example.com:443", time.Hour},
		{"uppercase", "Example.com", time.Hour},
		{"space", "exam ple.com", time.Hour},
		{"single label", "localhost", time.Hour},
		{"localhost suffix", "app.localhost", time.Hour},
		{"ip literal", "93.184.216.34", time.Hour},
		{"leading hyphen", "-bad.example.com", time.Hour},
		{"empty label", "bad..example.com", time.Hour},
		{"too long label", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa.example.com", time.Hour},
		{"interval too short", "example.com", 9 * time.Minute},
		{"interval too long", "example.com", 25 * time.Hour},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := Domain{ID: "d", Domain: tt.domain, Interval: tt.every}
			if err := d.Validate(); err == nil {
				t.Fatalf("%q every %s was accepted", tt.domain, tt.every)
			}
		})
	}
}

func TestFingerprintCoversObservedFields(t *testing.T) {
	base := func() Result {
		return Result{HTTPStatus: 200, CertIssuer: "CN=R11", CertNotAfter: ptrTime(time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)), NS: []string{"ns2.example.net", "ns1.example.net"}, Addresses: []string{"93.184.216.34"}, SPF: "v=spf1 -all", DMARCPolicy: "reject", HSTS: true, CSP: true}
	}
	first := base().fingerprint()
	if first == "" {
		t.Fatal("empty fingerprint")
	}
	reordered := base()
	reordered.NS = []string{"ns1.example.net", "ns2.example.net"}
	if reordered.fingerprint() != first {
		t.Fatal("NS order changed the fingerprint")
	}
	mutations := map[string]func(*Result){
		"status":   func(r *Result) { r.HTTPStatus = 525 },
		"issuer":   func(r *Result) { r.CertIssuer = "CN=E5" },
		"notAfter": func(r *Result) { r.CertNotAfter = ptrTime(time.Date(2027, 4, 1, 0, 0, 0, 0, time.UTC)) },
		"ns":       func(r *Result) { r.NS = []string{"ns1.example.net"} },
		"a":        func(r *Result) { r.Addresses = []string{"93.184.216.35"} },
		"spf":      func(r *Result) { r.SPF = "v=spf1 ~all" },
		"dmarc":    func(r *Result) { r.DMARCPolicy = "none" },
		"hsts":     func(r *Result) { r.HSTS = false },
		"csp":      func(r *Result) { r.CSP = false },
	}
	for name, mutate := range mutations {
		r := base()
		mutate(&r)
		if r.fingerprint() == first {
			t.Fatalf("%s change left the fingerprint unchanged", name)
		}
	}
	days := base()
	days.CertDaysLeft = ptrInt(3)
	if days.fingerprint() != first {
		t.Fatal("days left (which changes daily) must not be part of the fingerprint")
	}
}

func TestChangesDescribeWhatMoved(t *testing.T) {
	before := Result{HTTPStatus: 200, CertIssuer: "CN=R11", CertNotAfter: ptrTime(time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)), NS: []string{"ns1.example.net"}, Addresses: []string{"93.184.216.34"}, SPF: "v=spf1 -all", DMARCPolicy: "reject", HSTS: true, CSP: true, NSPresent: true, Resolves: true}
	after := before
	after.HTTPStatus = 525
	after.CertNotAfter = ptrTime(time.Date(2027, 4, 1, 0, 0, 0, 0, time.UTC))
	after.DMARCPolicy = ""
	after.CSP = false
	changes := Changes(before, after)
	want := []string{"HTTP 200 → 525", "certificate now expires 2027-04-01", "DMARC policy reject → missing", "Content-Security-Policy removed"}
	if len(changes) != len(want) {
		t.Fatalf("changes=%q want %q", changes, want)
	}
	for i := range want {
		if changes[i] != want[i] {
			t.Fatalf("changes[%d]=%q want %q", i, changes[i], want[i])
		}
	}
	if got := Changes(before, before); len(got) != 0 {
		t.Fatalf("identical results reported %q", got)
	}
}

func TestResultFailed(t *testing.T) {
	if (Result{HTTPStatus: 200}).Failed() || (Result{HTTPStatus: 404}).Failed() {
		t.Fatal("2xx/4xx counted as failure")
	}
	if !(Result{HTTPStatus: 525}).Failed() || !(Result{FailureCode: FailureTimeout}).Failed() || !(Result{}).Failed() {
		t.Fatal("5xx, failure code or no response must count as failure")
	}
}

func ptrTime(t time.Time) *time.Time { return &t }
func ptrInt(v int) *int              { return &v }
