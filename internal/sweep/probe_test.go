// SPDX-License-Identifier: AGPL-3.0-only
package sweep

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
	"time"
)

// fakeResolver answers from maps; a name that is absent has no records.
type fakeResolver struct {
	ns  map[string][]string
	txt map[string][]string
	ip  map[string][]netip.Addr
}

func (r fakeResolver) LookupNS(_ context.Context, name string) ([]*net.NS, error) {
	out := []*net.NS{}
	for _, host := range r.ns[name] {
		out = append(out, &net.NS{Host: host})
	}
	return out, nil
}
func (r fakeResolver) LookupTXT(_ context.Context, name string) ([]string, error) {
	return r.txt[name], nil
}
func (r fakeResolver) LookupNetIP(_ context.Context, _ string, host string) ([]netip.Addr, error) {
	return r.ip[host], nil
}

const testDomain = "example.com" // the SAN of httptest's certificate

func healthyResolver() fakeResolver {
	return fakeResolver{
		ns:  map[string][]string{testDomain: {"NS2.example.net.", "ns1.example.net."}},
		txt: map[string][]string{testDomain: {"google-site-verification=abc", "v=spf1 include:_spf.example.net -all"}, "_dmarc." + testDomain: {"v=DMARC1; p=reject; rua=mailto:dmarc@example.com"}},
		ip:  map[string][]netip.Addr{testDomain: {netip.MustParseAddr("93.184.216.34")}},
	}
}

// tlsServer starts an HTTPS server and returns a prober that dials it for
// every address the policy resolved, trusting its certificate.
func tlsServer(t *testing.T, handler http.HandlerFunc, resolver Resolver, now time.Time) (*httptest.Server, *Prober) {
	t.Helper()
	server := httptest.NewTLSServer(handler)
	t.Cleanup(server.Close)
	dialer := &net.Dialer{}
	prober := &Prober{
		Resolver: resolver,
		DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
			return dialer.DialContext(ctx, network, server.Listener.Addr().String())
		},
		TLSConfig: server.Client().Transport.(*http.Transport).TLSClientConfig,
		Now:       func() time.Time { return now },
	}
	return server, prober
}

func secure(status int) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Strict-Transport-Security", "max-age=63072000")
		w.Header().Set("Content-Security-Policy", "default-src 'self'")
		w.WriteHeader(status)
		if _, err := w.Write([]byte("ok")); err != nil {
			panic(err)
		}
	}
}

func TestProbeHealthyDomain(t *testing.T) {
	now := time.Date(2026, 11, 25, 12, 0, 0, 0, time.UTC)
	server, prober := tlsServer(t, secure(200), healthyResolver(), now)
	result := prober.Probe(context.Background(), Domain{ID: "d1", Domain: testDomain, Interval: time.Hour})
	if result.FailureCode != "" || result.HTTPStatus != 200 || result.Failed() {
		t.Fatalf("result=%+v", result)
	}
	if !result.NSPresent || !result.Resolves {
		t.Fatalf("dns: %+v", result)
	}
	if strings.Join(result.NS, ",") != "ns1.example.net,ns2.example.net" {
		t.Fatalf("ns=%v (want lowercased, sorted, without trailing dots)", result.NS)
	}
	if len(result.Addresses) != 1 || result.Addresses[0] != "93.184.216.34" {
		t.Fatalf("addresses=%v", result.Addresses)
	}
	if result.SPF != "v=spf1 include:_spf.example.net -all" || result.DMARCPolicy != "reject" {
		t.Fatalf("mail auth: spf=%q dmarc=%q", result.SPF, result.DMARCPolicy)
	}
	if !result.HSTS || !result.CSP {
		t.Fatalf("headers: hsts=%v csp=%v", result.HSTS, result.CSP)
	}
	cert := server.Certificate()
	if result.CertNotAfter == nil || !result.CertNotAfter.Equal(cert.NotAfter) || result.CertIssuer == "" {
		t.Fatalf("cert: %+v", result)
	}
	wantDays := int(cert.NotAfter.Sub(now).Hours() / 24)
	if result.CertDaysLeft == nil || *result.CertDaysLeft != wantDays {
		t.Fatalf("days left=%v want %d", result.CertDaysLeft, wantDays)
	}
	if result.Fingerprint == "" || result.CheckedAt != now {
		t.Fatalf("fingerprint=%q checkedAt=%s", result.Fingerprint, result.CheckedAt)
	}
	if result.FinalURL != "https://example.com/" {
		t.Fatalf("final url=%q", result.FinalURL)
	}
}

func TestProbeRecordsFiveTwentyFiveAsStatus(t *testing.T) {
	_, prober := tlsServer(t, secure(525), healthyResolver(), time.Now())
	result := prober.Probe(context.Background(), Domain{ID: "d1", Domain: testDomain, Interval: time.Hour})
	if result.HTTPStatus != 525 || result.FailureCode != "" {
		t.Fatalf("result=%+v", result)
	}
	if !result.Failed() {
		t.Fatal("a 525 must count as unreachable")
	}
}

func TestProbeCertificateDaysLeft(t *testing.T) {
	server := httptest.NewTLSServer(secure(200))
	defer server.Close()
	notAfter := server.Certificate().NotAfter
	tests := []struct {
		name string
		now  time.Time
		want int
	}{
		{"ten days", notAfter.Add(-10*24*time.Hour - time.Hour), 10},
		{"under a day", notAfter.Add(-time.Hour), 0},
		{"expired", notAfter.Add(time.Hour), -1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, prober := tlsServer(t, secure(200), healthyResolver(), tt.now)
			result := prober.Probe(context.Background(), Domain{ID: "d1", Domain: testDomain, Interval: time.Hour})
			if result.CertDaysLeft == nil || *result.CertDaysLeft != tt.want {
				t.Fatalf("days left=%v want %d (failure=%q)", result.CertDaysLeft, tt.want, result.FailureCode)
			}
		})
	}
}

func TestProbeMissingRecords(t *testing.T) {
	resolver := healthyResolver()
	delete(resolver.ns, testDomain)
	resolver.txt = map[string][]string{testDomain: {"v=spf2.0/pra ~all"}, "_dmarc." + testDomain: {"v=DMARC1; rua=mailto:x@example.com"}}
	_, prober := tlsServer(t, func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) }, resolver, time.Now())
	result := prober.Probe(context.Background(), Domain{ID: "d1", Domain: testDomain, Interval: time.Hour})
	if result.NSPresent || len(result.NS) != 0 {
		t.Fatalf("ns present without records: %+v", result)
	}
	if !result.Resolves || result.HTTPStatus != 200 {
		t.Fatalf("http still works without NS: %+v", result)
	}
	if result.SPF != "" || result.DMARCPolicy != "" {
		t.Fatalf("spf=%q dmarc=%q want both empty", result.SPF, result.DMARCPolicy)
	}
	if result.HSTS || result.CSP {
		t.Fatalf("headers reported present: %+v", result)
	}
}

func TestProbeFollowsBoundedRedirects(t *testing.T) {
	hops := 0
	_, prober := tlsServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/final" {
			secure(200)(w, r)
			return
		}
		hops++
		http.Redirect(w, r, "https://example.com/final", http.StatusFound)
	}, healthyResolver(), time.Now())
	result := prober.Probe(context.Background(), Domain{ID: "d1", Domain: testDomain, Interval: time.Hour})
	if result.HTTPStatus != 200 || !result.HSTS || !strings.HasSuffix(result.FinalURL, "/final") {
		t.Fatalf("result=%+v", result)
	}

	_, looping := tlsServer(t, func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "https://example.com/again", http.StatusFound)
	}, healthyResolver(), time.Now())
	result = looping.Probe(context.Background(), Domain{ID: "d1", Domain: testDomain, Interval: time.Hour})
	if result.HTTPStatus != 0 || result.FailureCode != FailureConnection {
		t.Fatalf("redirect loop result=%+v", result)
	}
}

func TestProbeClassifiesFailures(t *testing.T) {
	tests := []struct {
		name    string
		prober  *Prober
		want    FailureCode
		resolve bool
	}{
		{"unresolvable", &Prober{Resolver: fakeResolver{ns: map[string][]string{testDomain: {"ns1.example.net"}}}}, FailureDNS, false},
		{"private without opt-in", &Prober{Resolver: fakeResolver{ip: map[string][]netip.Addr{testDomain: {netip.MustParseAddr("10.0.0.1")}}}}, FailureTargetBlocked, true},
		{"metadata even with opt-in", &Prober{AllowPrivate: true, Resolver: fakeResolver{ip: map[string][]netip.Addr{testDomain: {netip.MustParseAddr("169.254.169.254")}}}}, FailureTargetBlocked, true},
		{"connection refused", &Prober{Resolver: healthyResolver(), DialContext: func(context.Context, string, string) (net.Conn, error) {
			return nil, &net.OpError{Op: "dial", Err: errRefused{}}
		}}, FailureConnection, true},
		{"untrusted certificate", untrusted(t), FailureTLS, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.prober.Probe(context.Background(), Domain{ID: "d1", Domain: testDomain, Interval: time.Hour})
			if result.FailureCode != tt.want || result.HTTPStatus != 0 {
				t.Fatalf("result=%+v want failure %q", result, tt.want)
			}
			if result.Resolves != tt.resolve {
				t.Fatalf("resolves=%v want %v", result.Resolves, tt.resolve)
			}
			if !result.Failed() {
				t.Fatal("failure must count as unreachable")
			}
		})
	}
}

// untrusted dials a local TLS server whose certificate the system roots do
// not know.
func untrusted(t *testing.T) *Prober {
	t.Helper()
	_, prober := tlsServer(t, secure(200), healthyResolver(), time.Now())
	prober.TLSConfig = nil
	return prober
}

type errRefused struct{}

func (errRefused) Error() string { return "connection refused" }

func TestProbeTimesOut(t *testing.T) {
	release := make(chan struct{})
	defer close(release)
	_, prober := tlsServer(t, func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-release:
		case <-r.Context().Done():
		}
	}, healthyResolver(), time.Now())
	prober.Timeout = 200 * time.Millisecond
	result := prober.Probe(context.Background(), Domain{ID: "d1", Domain: testDomain, Interval: time.Hour})
	if result.FailureCode != FailureTimeout {
		t.Fatalf("result=%+v", result)
	}
}

func TestProbeFingerprintChangesWithObservations(t *testing.T) {
	_, healthy := tlsServer(t, secure(200), healthyResolver(), time.Now())
	first := healthy.Probe(context.Background(), Domain{ID: "d1", Domain: testDomain, Interval: time.Hour})
	again := healthy.Probe(context.Background(), Domain{ID: "d1", Domain: testDomain, Interval: time.Hour})
	if first.Fingerprint != again.Fingerprint {
		t.Fatal("two identical probes produced different fingerprints")
	}
	_, degraded := tlsServer(t, func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) }, healthyResolver(), time.Now())
	later := degraded.Probe(context.Background(), Domain{ID: "d1", Domain: testDomain, Interval: time.Hour})
	if later.Fingerprint == first.Fingerprint {
		t.Fatal("dropped headers did not change the fingerprint")
	}
	if changes := Changes(first, later); len(changes) != 2 {
		t.Fatalf("changes=%q (want HSTS and CSP removed)", changes)
	}
}
