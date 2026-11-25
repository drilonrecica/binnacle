// SPDX-License-Identifier: AGPL-3.0-only

package sweep

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/drilonrecica/binnacle/internal/outbound"
)

// Resolver is the DNS surface the probe needs. net.DefaultResolver
// implements it; tests inject a map.
type Resolver interface {
	LookupNS(context.Context, string) ([]*net.NS, error)
	LookupTXT(context.Context, string) ([]string, error)
	LookupNetIP(context.Context, string, string) ([]netip.Addr, error)
}

// Prober runs one sweep of a domain: DNS, mail authentication records and
// one bounded HTTPS request through the shared outbound policy.
type Prober struct {
	AllowPrivate bool
	Resolver     Resolver
	Dialer       *net.Dialer
	DialContext  func(context.Context, string, string) (net.Conn, error)
	// TLSConfig is the client configuration of the HTTPS request; nil is the
	// system default. Tests use it to trust a local certificate.
	TLSConfig *tls.Config
	// Timeout bounds the whole probe; zero means Timeout.
	Timeout time.Duration
	Now     func() time.Time
}

const userAgent = "Binnacle domain sweep (+https://github.com/drilonrecica/binnacle)"

func (p *Prober) resolver() Resolver {
	if p.Resolver != nil {
		return p.Resolver
	}
	return net.DefaultResolver
}

func (p *Prober) policy() outbound.Policy {
	return outbound.Policy{AllowPrivate: p.AllowPrivate, Resolver: p.resolver(), Dialer: p.Dialer, Dial: p.DialContext}
}

func (p *Prober) timeout() time.Duration {
	if p.Timeout > 0 {
		return p.Timeout
	}
	return Timeout
}

func (p *Prober) validateURL(ctx context.Context, raw string) (*url.URL, error) {
	u, err := p.policy().ValidateURL(ctx, raw, "http", "https")
	if errors.Is(err, outbound.ErrDNS) {
		return nil, fmt.Errorf("%s: %w", FailureDNS, err)
	}
	if err != nil {
		return nil, fmt.Errorf("%s", FailureTargetBlocked)
	}
	return u, nil
}

// Probe sweeps one domain. It never returns an error: everything it could
// not observe is a failure code or an absent value in the result.
func (p *Prober) Probe(ctx context.Context, d Domain) Result {
	now := time.Now
	if p.Now != nil {
		now = p.Now
	}
	result := Result{DomainID: d.ID, CheckedAt: now().UTC()}
	ctx, cancel := context.WithTimeout(ctx, p.timeout())
	defer cancel()
	p.lookupDNS(ctx, d.Domain, &result)
	p.lookupMail(ctx, d.Domain, &result)
	p.fetch(ctx, d.Domain, &result, now)
	result.Fingerprint = result.fingerprint()
	return result
}

// lookupDNS records the NS set (registered) and the A/AAAA set (resolves).
// A lookup error reads as no records: the HTTP step then reports why.
func (p *Prober) lookupDNS(ctx context.Context, domain string, result *Result) {
	if records, err := p.resolver().LookupNS(ctx, domain); err == nil {
		for _, record := range records {
			if host := NormalizeDomain(record.Host); host != "" {
				result.NS = append(result.NS, host)
			}
		}
		sort.Strings(result.NS)
	}
	result.NSPresent = len(result.NS) > 0
	if addrs, err := p.resolver().LookupNetIP(ctx, "ip", domain); err == nil {
		for _, addr := range addrs {
			result.Addresses = append(result.Addresses, addr.Unmap().String())
		}
		sort.Strings(result.Addresses)
	}
	result.Resolves = len(result.Addresses) > 0
}

// lookupMail stores the apex SPF record and the DMARC policy (the p= tag of
// the _dmarc TXT record), empty when absent.
func (p *Prober) lookupMail(ctx context.Context, domain string, result *Result) {
	if records, err := p.resolver().LookupTXT(ctx, domain); err == nil {
		for _, record := range records {
			if strings.HasPrefix(strings.ToLower(strings.TrimSpace(record)), "v=spf1") {
				result.SPF = strings.TrimSpace(record)
				break
			}
		}
	}
	if records, err := p.resolver().LookupTXT(ctx, "_dmarc."+domain); err == nil {
		for _, record := range records {
			if strings.HasPrefix(strings.ToLower(strings.TrimSpace(record)), "v=dmarc1") {
				result.DMARCPolicy = dmarcPolicy(record)
				break
			}
		}
	}
}

// dmarcPolicy returns the lowercase p= tag of a DMARC record, or "".
func dmarcPolicy(record string) string {
	for _, tag := range strings.Split(record, ";") {
		key, value, ok := strings.Cut(strings.TrimSpace(tag), "=")
		if ok && strings.EqualFold(strings.TrimSpace(key), "p") {
			return strings.ToLower(strings.TrimSpace(value))
		}
	}
	return ""
}

// fetch makes the bounded GET https://<domain>/ and records the final
// status, the leaf certificate and the two security headers.
func (p *Prober) fetch(ctx context.Context, domain string, result *Result, now func() time.Time) {
	target := "https://" + domain + "/"
	if _, err := p.validateURL(ctx, target); err != nil {
		result.FailureCode = classify(err)
		return
	}
	transport := &http.Transport{
		Proxy: nil, DisableKeepAlives: true, TLSHandshakeTimeout: p.timeout(), TLSClientConfig: p.TLSConfig,
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			return p.policy().DialContext(ctx, network, address)
		},
	}
	client := &http.Client{Transport: transport, Timeout: p.timeout()}
	redirects := 0
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		redirects++
		if redirects > MaxRedirects {
			return errors.New("redirect limit exceeded")
		}
		_, err := p.validateURL(req.Context(), req.URL.String())
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		result.FailureCode = FailureTargetBlocked
		return
	}
	req.Header.Set("User-Agent", userAgent)
	resp, err := client.Do(req)
	if err != nil {
		result.FailureCode = classify(err)
		return
	}
	defer resp.Body.Close()
	result.HTTPStatus = resp.StatusCode
	if resp.Request != nil && resp.Request.URL != nil {
		result.FinalURL = resp.Request.URL.String()
	}
	result.HSTS = resp.Header.Get("Strict-Transport-Security") != ""
	result.CSP = resp.Header.Get("Content-Security-Policy") != ""
	if resp.TLS != nil && len(resp.TLS.PeerCertificates) > 0 {
		leaf := resp.TLS.PeerCertificates[0]
		notAfter := leaf.NotAfter.UTC()
		days := int(math.Floor(notAfter.Sub(now()).Hours() / 24))
		result.CertNotAfter = &notAfter
		result.CertDaysLeft = &days
		result.CertIssuer = leaf.Issuer.String()
	}
	// The body is drained up to the bound and never kept; a response that
	// breaks off after its headers is a connection failure, as for checks.
	if _, err = io.Copy(io.Discard, io.LimitReader(resp.Body, MaxBodyRead)); err != nil {
		result.FailureCode = FailureConnection
	}
}

// classify mirrors the HTTP check runner's failure classification.
func classify(err error) FailureCode {
	s := err.Error()
	for _, code := range []FailureCode{FailureTargetBlocked, FailureDNS} {
		if strings.Contains(s, string(code)) {
			return code
		}
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, syscall.ETIMEDOUT) {
		return FailureTimeout
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return FailureTimeout
	}
	var tlsErr tls.RecordHeaderError
	if errors.As(err, &tlsErr) || strings.Contains(strings.ToLower(s), "tls") || strings.Contains(strings.ToLower(s), "certificate") {
		return FailureTLS
	}
	return FailureConnection
}
