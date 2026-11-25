// SPDX-License-Identifier: AGPL-3.0-only

package sweep

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/netip"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/drilonrecica/binnacle/internal/metrics"
)

const (
	MinInterval     = 10 * time.Minute
	MaxInterval     = 24 * time.Hour
	DefaultInterval = time.Hour
	// Timeout bounds one whole probe: the DNS lookups and the HTTP request.
	Timeout     = 20 * time.Second
	MaxBodyRead = 64 << 10
	// MaxRedirects matches the HTTP check runner.
	MaxRedirects = 3
	// CertExpiringDays is the default threshold of the certificate alert.
	CertExpiringDays = 14
	// EventType is the events-table type written when a domain's
	// fingerprint changes between two runs.
	EventType = metrics.EventDomainChange
)

type FailureCode string

// The failure codes share their names with the HTTP check runner, so the UI
// labels them the same way.
const (
	FailureDNS           FailureCode = "dns"
	FailureTimeout       FailureCode = "timeout"
	FailureConnection    FailureCode = "connection"
	FailureTLS           FailureCode = "tls_handshake"
	FailureTargetBlocked FailureCode = "target_blocked"
)

// Domain is one swept domain: a bare, lowercase hostname.
type Domain struct {
	ID        string        `json:"id"`
	Domain    string        `json:"domain"`
	Interval  time.Duration `json:"interval"`
	Enabled   bool          `json:"enabled"`
	CreatedAt time.Time     `json:"createdAt"`
	UpdatedAt time.Time     `json:"updatedAt"`
}

// NormalizeDomain lowercases a user-entered domain and drops surrounding
// space and a trailing dot. It does not validate.
func NormalizeDomain(raw string) string {
	return strings.TrimSuffix(strings.ToLower(strings.TrimSpace(raw)), ".")
}

func (d Domain) Validate() error {
	if strings.TrimSpace(d.ID) == "" {
		return errors.New("domain id is required")
	}
	if err := validateHostname(d.Domain); err != nil {
		return err
	}
	if d.Interval < MinInterval || d.Interval > MaxInterval {
		return errors.New("interval must be between 10 minutes and 24 hours")
	}
	return nil
}

// validateHostname accepts a registrable, lowercase DNS name: at least two
// labels of letters, digits and inner hyphens, no scheme, port, path or
// credentials, not an IP literal and not under .localhost.
func validateHostname(name string) error {
	if name == "" {
		return errors.New("domain is required")
	}
	if len(name) > 253 {
		return errors.New("domain is too long")
	}
	if name != NormalizeDomain(name) {
		return errors.New("domain must be a lowercase hostname without a trailing dot")
	}
	if strings.ContainsAny(name, "/:@?#\\ \t") {
		return errors.New("domain must be a bare hostname without scheme, port or path")
	}
	if _, err := netip.ParseAddr(name); err == nil {
		return errors.New("domain must be a hostname, not an IP address")
	}
	labels := strings.Split(name, ".")
	if len(labels) < 2 {
		return errors.New("domain must have at least two labels")
	}
	if labels[len(labels)-1] == "localhost" {
		return errors.New("localhost domains are blocked")
	}
	for _, label := range labels {
		if label == "" || len(label) > 63 {
			return errors.New("domain labels must be 1 to 63 characters")
		}
		if label[0] == '-' || label[len(label)-1] == '-' {
			return errors.New("domain labels cannot start or end with a hyphen")
		}
		for _, r := range label {
			if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-') {
				return errors.New("domain contains characters outside a-z, 0-9 and hyphen")
			}
		}
	}
	return nil
}

// Result is one probe of a domain. Nil pointers mean the value was not
// observed (no TLS handshake, for example), not that it is zero.
type Result struct {
	DomainID     string      `json:"domainId"`
	CheckedAt    time.Time   `json:"checkedAt"`
	HTTPStatus   int         `json:"httpStatus,omitempty"`
	FailureCode  FailureCode `json:"failureCode,omitempty"`
	FinalURL     string      `json:"finalUrl,omitempty"`
	CertIssuer   string      `json:"certIssuer,omitempty"`
	CertNotAfter *time.Time  `json:"certNotAfter,omitempty"`
	CertDaysLeft *int        `json:"certDaysLeft,omitempty"`
	NSPresent    bool        `json:"nsPresent"`
	Resolves     bool        `json:"resolves"`
	NS           []string    `json:"ns,omitempty"`
	Addresses    []string    `json:"addresses,omitempty"`
	SPF          string      `json:"spf,omitempty"`
	DMARCPolicy  string      `json:"dmarcPolicy,omitempty"`
	HSTS         bool        `json:"hsts"`
	CSP          bool        `json:"csp"`
	Fingerprint  string      `json:"fingerprint"`
}

// Failed reports whether the domain counts as unreachable: no HTTP response
// or a 5xx status (a Cloudflare 525 included).
func (r Result) Failed() bool {
	return r.FailureCode != "" || r.HTTPStatus == 0 || r.HTTPStatus >= 500
}

// completed fills in what a prober may leave out: the time of the probe and
// the fingerprint of what it observed.
func (r Result) completed() Result {
	if r.CheckedAt.IsZero() {
		r.CheckedAt = time.Now().UTC()
	}
	if r.Fingerprint == "" {
		r.Fingerprint = r.fingerprint()
	}
	return r
}

// State is the stored latest result of a domain plus its schedule.
type State struct {
	Result
	NextRunAt           time.Time `json:"nextRunAt"`
	ConsecutiveFailures int       `json:"consecutiveFailures"`
}

// Entry is a domain with its latest state for list views.
type Entry struct {
	Domain
	State *State `json:"state,omitempty"`
}

func sortedCopy(values []string) []string {
	out := append([]string(nil), values...)
	sort.Strings(out)
	return out
}

// fingerprint hashes what the sweep watches for change: the status, the
// certificate identity, the NS and address sets, mail authentication and the
// two headers. Days left and timestamps change on their own and stay out.
func (r Result) fingerprint() string {
	h := sha256.New()
	write := func(parts ...string) {
		for _, part := range parts {
			h.Write([]byte(part))
			h.Write([]byte{0})
		}
	}
	notAfter := ""
	if r.CertNotAfter != nil {
		notAfter = strconv.FormatInt(r.CertNotAfter.Unix(), 10)
	}
	write("status", strconv.Itoa(r.HTTPStatus), "issuer", r.CertIssuer, "notAfter", notAfter)
	write(append([]string{"ns"}, sortedCopy(r.NS)...)...)
	write(append([]string{"a"}, sortedCopy(r.Addresses)...)...)
	write("spf", r.SPF, "dmarc", r.DMARCPolicy, "hsts", strconv.FormatBool(r.HSTS), "csp", strconv.FormatBool(r.CSP))
	return hex.EncodeToString(h.Sum(nil))
}

// Changes lists, in words, what differs between two results of the same
// domain, in the order of the fingerprint fields. Empty when nothing watched
// changed.
func Changes(before, after Result) []string {
	var out []string
	if before.HTTPStatus != after.HTTPStatus {
		out = append(out, fmt.Sprintf("HTTP %s → %s", statusWord(before.HTTPStatus, before.FailureCode), statusWord(after.HTTPStatus, after.FailureCode)))
	}
	switch {
	case before.CertNotAfter == nil && after.CertNotAfter != nil:
		out = append(out, "certificate now expires "+after.CertNotAfter.UTC().Format("2006-01-02"))
	case before.CertNotAfter != nil && after.CertNotAfter == nil:
		out = append(out, "certificate no longer observed")
	case before.CertNotAfter != nil && after.CertNotAfter != nil && (!before.CertNotAfter.Equal(*after.CertNotAfter) || before.CertIssuer != after.CertIssuer):
		out = append(out, "certificate now expires "+after.CertNotAfter.UTC().Format("2006-01-02"))
	}
	if !equalSets(before.NS, after.NS) {
		out = append(out, "NS "+setWord(before.NS)+" → "+setWord(after.NS))
	}
	if !equalSets(before.Addresses, after.Addresses) {
		out = append(out, "A/AAAA "+setWord(before.Addresses)+" → "+setWord(after.Addresses))
	}
	if before.SPF != after.SPF {
		out = append(out, "SPF "+orMissing(before.SPF)+" → "+orMissing(after.SPF))
	}
	if before.DMARCPolicy != after.DMARCPolicy {
		out = append(out, "DMARC policy "+orMissing(before.DMARCPolicy)+" → "+orMissing(after.DMARCPolicy))
	}
	if before.HSTS != after.HSTS {
		out = append(out, "Strict-Transport-Security "+presenceWord(after.HSTS))
	}
	if before.CSP != after.CSP {
		out = append(out, "Content-Security-Policy "+presenceWord(after.CSP))
	}
	return out
}

func statusWord(status int, code FailureCode) string {
	if status > 0 {
		return strconv.Itoa(status)
	}
	if code != "" {
		return string(code)
	}
	return "none"
}
func setWord(values []string) string {
	if len(values) == 0 {
		return "none"
	}
	return strings.Join(sortedCopy(values), ",")
}
func orMissing(v string) string {
	if v == "" {
		return "missing"
	}
	return v
}
func presenceWord(present bool) string {
	if present {
		return "added"
	}
	return "removed"
}
func equalSets(a, b []string) bool {
	a, b = sortedCopy(a), sortedCopy(b)
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
