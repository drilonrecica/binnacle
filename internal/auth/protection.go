// SPDX-License-Identifier: AGPL-3.0-only
package auth

import (
	"container/list"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"sync"
	"time"
)

// TrustedProxies only affects forwarding-header provenance and the declared
// public origin, never authentication.
type TrustedProxies struct {
	prefixes []netip.Prefix
	public   *url.URL
}

// ParseTrustedProxies accepts host or network prefixes for peers allowed to
// supply X-Forwarded-* headers. Docker assigns a reverse proxy a new address on
// every recreate, so the network it shares with Binnacle is the stable identity.
// A zero-length prefix would trust every peer and is refused.
func ParseTrustedProxies(values []string) (TrustedProxies, error) {
	return parsePrefixes(values, false)
}

// ParseExactProxies accepts only single hosts (/32 or /128). Identity peers
// (ADR 018) stay exact: the header they send becomes the administrator.
func ParseExactProxies(values []string) (TrustedProxies, error) {
	return parsePrefixes(values, true)
}
func parsePrefixes(values []string, exact bool) (TrustedProxies, error) {
	p := TrustedProxies{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		prefix, err := netip.ParsePrefix(value)
		if err != nil {
			return TrustedProxies{}, err
		}
		if exact && prefix.Bits() != prefix.Addr().BitLen() {
			return TrustedProxies{}, errors.New("proxy CIDRs must identify one exact host (/32 for IPv4 or /128 for IPv6)")
		}
		if prefix.Bits() == 0 {
			return TrustedProxies{}, errors.New("trusted proxy CIDRs cannot cover every address (/0)")
		}
		p.prefixes = append(p.prefixes, prefix.Masked())
	}
	return p, nil
}

// WithPublicURL declares the origin browsers use to reach Binnacle. Only the
// scheme, host and port are kept; a path is permitted and ignored because the
// value is compared against Origin headers, which never carry one. An empty
// value leaves the request-derived behaviour in place.
func (p TrustedProxies) WithPublicURL(raw string) (TrustedProxies, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		p.public = nil
		return p, nil
	}
	u, err := url.Parse(raw)
	if err != nil {
		return TrustedProxies{}, fmt.Errorf("public URL: %w", err)
	}
	scheme := strings.ToLower(u.Scheme)
	if (scheme != "http" && scheme != "https") || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Opaque != "" {
		return TrustedProxies{}, errors.New("public URL must be an absolute http(s) origin such as https://binnacle.example.com")
	}
	p.public = &url.URL{Scheme: scheme, Host: normalizeHost(scheme, u.Host)}
	return p, nil
}

// PublicOrigin returns the declared origin or "" when none is configured.
func (p TrustedProxies) PublicOrigin() string {
	if p.public == nil {
		return ""
	}
	return p.public.String()
}

// normalizeHost lowercases a host and drops the scheme's default port so that
// https://example.test:443 and https://example.test compare equal.
func normalizeHost(scheme, host string) string {
	host = strings.ToLower(host)
	if h, port, err := net.SplitHostPort(host); err == nil {
		if (scheme == "https" && port == "443") || (scheme == "http" && port == "80") {
			host = h
			if strings.Contains(host, ":") {
				host = "[" + host + "]"
			}
		}
	}
	return host
}
func (p TrustedProxies) trusted(r *http.Request) bool {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return false
	}
	addr, err := netip.ParseAddr(host)
	if err != nil {
		return false
	}
	return p.trustedAddr(addr)
}

// TrustedPeer reports whether the immediate TCP peer is allowlisted. Forwarded
// headers are deliberately not considered.
func (p TrustedProxies) TrustedPeer(r *http.Request) bool { return p.trusted(r) }
func (p TrustedProxies) trustedAddr(addr netip.Addr) bool {
	for _, prefix := range p.prefixes {
		if prefix.Contains(addr) {
			return true
		}
	}
	return false
}

// forwardedValues joins every header line (a client may send its own line and
// a proxy may append rather than replace) into one left-to-right list.
func forwardedValues(r *http.Request, name string) []string {
	var out []string
	for _, line := range r.Header.Values(name) {
		for _, v := range strings.Split(line, ",") {
			if v = strings.TrimSpace(v); v != "" {
				out = append(out, v)
			}
		}
	}
	return out
}

// Secure reports whether the browser reached Binnacle over HTTPS. A declared
// public URL is authoritative; otherwise direct TLS or the rightmost
// X-Forwarded-Proto value set by a trusted peer decides.
func (p TrustedProxies) Secure(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	if p.public != nil {
		return p.public.Scheme == "https"
	}
	if !p.trusted(r) {
		return false
	}
	values := forwardedValues(r, "X-Forwarded-Proto")
	return len(values) > 0 && strings.EqualFold(values[len(values)-1], "https")
}
func (p TrustedProxies) ClientPrefix(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	if p.trusted(r) {
		values := forwardedValues(r, "X-Forwarded-For")
		if len(values) > 0 && len(values) <= 16 {
			for i := len(values) - 1; i >= 0; i-- {
				addr, parseErr := netip.ParseAddr(values[i])
				if parseErr != nil {
					continue
				}
				if p.trustedAddr(addr) {
					continue
				}
				host = addr.String()
				break
			}
		}
	}
	addr, err := netip.ParseAddr(host)
	if err != nil {
		return "unknown"
	}
	if addr.Is4() {
		return netip.PrefixFrom(addr, 24).Masked().String()
	}
	return netip.PrefixFrom(addr, 56).Masked().String()
}

type BucketPolicy struct {
	Capacity float64
	Refill   time.Duration
}
type bucket struct {
	tokens float64
	at     time.Time
	used   time.Time
}
type bucketEntry struct {
	key   string
	value bucket
}
type Limiter struct {
	mu     sync.Mutex
	values map[string]*list.Element
	order  *list.List
	max    int
	now    func() time.Time
}

// accountBuckets bounds the per-account login LRU. Only stored accounts are
// inserted, so the real population is the number of users (one), and an
// attacker presenting invented names can never evict a real bucket.
const accountBuckets = 64

type Protection struct {
	limiter  *Limiter
	accounts *Limiter
	proxies  TrustedProxies
}

func NewProtection(max int, proxies TrustedProxies) *Protection {
	return &Protection{limiter: NewLimiter(max), accounts: NewLimiter(accountBuckets), proxies: proxies}
}
func (p *Protection) Proxies() TrustedProxies { return p.proxies }
func (p *Protection) allow(scope, key string, policy BucketPolicy) (bool, time.Duration) {
	return p.limiter.Allow(scope+":"+key, policy)
}

// AllowLogin applies the per-network budget first and, only when it passes
// and exists reports a stored account, the per-account budget. exists runs
// lazily so a flood that is already network-limited costs no database read and
// never touches an account bucket (SEC-10).
func (p *Protection) AllowLogin(r *http.Request, account string, exists func(string) bool) (bool, time.Duration) {
	ok, retry := p.allow("login-ip", p.proxies.ClientPrefix(r), BucketPolicy{Capacity: 10, Refill: time.Minute})
	if !ok {
		return false, retry
	}
	account = strings.ToLower(strings.TrimSpace(account))
	if exists == nil || !exists(account) {
		return true, 0
	}
	return p.accounts.Allow(account, BucketPolicy{Capacity: 5, Refill: 5 * time.Minute})
}
func (p *Protection) AllowSetup(r *http.Request) (bool, time.Duration) {
	return p.allow("setup", p.proxies.ClientPrefix(r), BucketPolicy{Capacity: 5, Refill: 5 * time.Minute})
}
func (p *Protection) AllowDiagnostics(r *http.Request, actor string) (bool, time.Duration) {
	return p.allow("diagnostics", actor+":"+p.proxies.ClientPrefix(r), BucketPolicy{Capacity: 3, Refill: 15 * time.Minute})
}
func (p *Protection) AllowMetrics(r *http.Request) (bool, time.Duration) {
	return p.allow("metrics", p.proxies.ClientPrefix(r), BucketPolicy{Capacity: 60, Refill: time.Minute})
}
func (p *Protection) AllowEvents(r *http.Request) (bool, time.Duration) {
	return p.allow("events", p.proxies.ClientPrefix(r), BucketPolicy{Capacity: 60, Refill: time.Minute})
}
func (p *Protection) AllowResources(r *http.Request) (bool, time.Duration) {
	return p.allow("resources", p.proxies.ClientPrefix(r), BucketPolicy{Capacity: 120, Refill: time.Minute})
}
func (p *Protection) AllowLive(r *http.Request) (bool, time.Duration) {
	return p.allow("live", p.proxies.ClientPrefix(r), BucketPolicy{Capacity: 30, Refill: time.Minute})
}

func NewLimiter(max int) *Limiter {
	if max < 1 {
		max = 4096
	}
	return &Limiter{values: map[string]*list.Element{}, order: list.New(), max: max, now: time.Now}
}

// Allow is bounded even when an attacker presents unlimited distinct keys.
func (l *Limiter) Allow(key string, policy BucketPolicy) (bool, time.Duration) {
	if policy.Capacity <= 0 || policy.Refill <= 0 {
		return true, 0
	}
	now := l.now()
	l.mu.Lock()
	defer l.mu.Unlock()
	entry, ok := l.values[key]
	if !ok {
		for l.order.Len() >= l.max {
			oldest := l.order.Back()
			delete(l.values, oldest.Value.(bucketEntry).key)
			l.order.Remove(oldest)
		}
		entry = l.order.PushFront(bucketEntry{key: key, value: bucket{tokens: policy.Capacity, at: now, used: now}})
		l.values[key] = entry
	}
	v := entry.Value.(bucketEntry)
	elapsed := now.Sub(v.value.at)
	v.value.tokens += elapsed.Seconds() * policy.Capacity / policy.Refill.Seconds()
	if v.value.tokens > policy.Capacity {
		v.value.tokens = policy.Capacity
	}
	v.value.at, v.value.used = now, now
	if v.value.tokens < 1 {
		entry.Value = v
		l.order.MoveToFront(entry)
		return false, time.Duration((1-v.value.tokens)*policy.Refill.Seconds()/policy.Capacity) * time.Second
	}
	v.value.tokens--
	entry.Value = v
	l.order.MoveToFront(entry)
	return true, 0
}

const CSRFCookieName = "binnacle_csrf"

func NewCSRFToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
func CSRFHash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return base64.RawStdEncoding.EncodeToString(sum[:])
}
func SetCSRFCookie(w http.ResponseWriter, token string, secure bool, expires time.Time) {
	http.SetCookie(w, &http.Cookie{Name: CSRFCookieName, Value: token, Path: "/", HttpOnly: false, Secure: secure, SameSite: http.SameSiteLaxMode, Expires: expires.UTC()})
}
func ClearCSRFCookie(w http.ResponseWriter, secure bool) {
	http.SetCookie(w, &http.Cookie{Name: CSRFCookieName, Value: "", Path: "/", Secure: secure, SameSite: http.SameSiteLaxMode, MaxAge: -1, Expires: time.Unix(1, 0)})
}
func ValidCSRF(r *http.Request, expectedHash string) bool {
	if r.Method == http.MethodGet || r.Method == http.MethodHead || r.Method == http.MethodOptions {
		return true
	}
	cookie, err := r.Cookie(CSRFCookieName)
	if err != nil {
		return false
	}
	header := r.Header.Get("X-CSRF-Token")
	if header == "" || subtle.ConstantTimeCompare([]byte(cookie.Value), []byte(header)) != 1 {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(CSRFHash(header)), []byte(expectedHash)) == 1
}

// SameOrigin compares the request's Origin (or Referer) with the origin the
// browser must be using. With a declared public URL that origin is the
// configuration itself; without one it is reconstructed from the Host header and
// the proxy-aware Secure check.
func SameOrigin(r *http.Request, proxies TrustedProxies) bool {
	raw := r.Header.Get("Origin")
	if raw == "" {
		raw = r.Referer()
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || u.Scheme == "" {
		return false
	}
	scheme := strings.ToLower(u.Scheme)
	got := scheme + "://" + normalizeHost(scheme, u.Host)
	if proxies.public != nil {
		return got == proxies.public.String()
	}
	expected := "http"
	if proxies.Secure(r) {
		expected = "https"
	}
	return got == expected+"://"+normalizeHost(expected, r.Host)
}
