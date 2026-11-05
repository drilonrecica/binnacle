// SPDX-License-Identifier: AGPL-3.0-only

// Package outbound enforces Binnacle's shared SSRF and DNS-rebinding policy.
package outbound

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"strings"
)

var (
	ErrBlocked = errors.New("outbound target is blocked")
	ErrDNS     = errors.New("outbound target DNS lookup failed")
)

type Resolver interface {
	LookupNetIP(context.Context, string, string) ([]netip.Addr, error)
}

type Policy struct {
	AllowPrivate bool
	Resolver     Resolver
	Dialer       *net.Dialer
	Dial         func(context.Context, string, string) (net.Conn, error)
}

var metadataAddresses = map[netip.Addr]struct{}{
	netip.MustParseAddr("169.254.169.254"): {},
	netip.MustParseAddr("100.100.100.200"): {},
	netip.MustParseAddr("fd00:ec2::254"):   {},
}

// reservedPrefixes are never valid outbound targets, even with the private
// opt-in: they are unroutable, reserved for protocol use, or for benchmarking.
var reservedPrefixes = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),     // "this" network
	netip.MustParsePrefix("192.0.0.0/24"),  // IETF protocol assignments
	netip.MustParsePrefix("198.18.0.0/15"), // benchmarking
	netip.MustParsePrefix("240.0.0.0/4"),   // reserved, class E
}

// privatePrefixes are blocked unless the deployment opts in to private
// targets, in addition to the RFC 1918 and ULA ranges netip already knows.
var privatePrefixes = []netip.Prefix{
	netip.MustParsePrefix("100.64.0.0/10"), // carrier-grade NAT, used by Tailscale
}

var (
	nat64Prefix          = netip.MustParsePrefix("64:ff9b::/96") // RFC 6052 well-known prefix
	sixToFourPrefix      = netip.MustParsePrefix("2002::/16")    // RFC 3056
	ipv4CompatiblePrefix = netip.MustParsePrefix("::/96")        // deprecated IPv4-compatible form
)

// embeddedIPv4 returns the IPv4 address carried inside a NAT64, 6to4 or
// IPv4-compatible IPv6 address, so that the policy can judge the host the
// packet would actually reach after translation.
func embeddedIPv4(addr netip.Addr) (netip.Addr, bool) {
	if !addr.Is6() || addr.Is4In6() {
		return netip.Addr{}, false
	}
	raw := addr.As16()
	switch {
	case nat64Prefix.Contains(addr), ipv4CompatiblePrefix.Contains(addr):
		return netip.AddrFrom4([4]byte{raw[12], raw[13], raw[14], raw[15]}), true
	case sixToFourPrefix.Contains(addr):
		return netip.AddrFrom4([4]byte{raw[2], raw[3], raw[4], raw[5]}), true
	}
	return netip.Addr{}, false
}

func containsAny(prefixes []netip.Prefix, addr netip.Addr) bool {
	for _, prefix := range prefixes {
		if prefix.Contains(addr) {
			return true
		}
	}
	return false
}

func (p Policy) Allowed(addr netip.Addr) bool {
	addr = addr.Unmap()
	if !addr.IsValid() || !addr.IsGlobalUnicast() || addr.IsUnspecified() || addr.IsLoopback() || addr.IsLinkLocalUnicast() || addr.IsLinkLocalMulticast() || addr.IsMulticast() {
		return false
	}
	if _, blocked := metadataAddresses[addr]; blocked {
		return false
	}
	if containsAny(reservedPrefixes, addr) {
		return false
	}
	// A translated address reaches the embedded IPv4 host: that host must pass
	// on its own, and the translation itself counts as a private target.
	if inner, ok := embeddedIPv4(addr); ok {
		return p.AllowPrivate && p.Allowed(inner)
	}
	return p.AllowPrivate || !(addr.IsPrivate() || containsAny(privatePrefixes, addr))
}

func (p Policy) Resolve(ctx context.Context, host string) ([]netip.Addr, error) {
	host = strings.ToLower(strings.TrimSuffix(host, "."))
	if host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return nil, ErrBlocked
	}
	resolver := p.Resolver
	if resolver == nil {
		resolver = net.DefaultResolver
	}
	addrs, err := resolver.LookupNetIP(ctx, "ip", host)
	if err != nil || len(addrs) == 0 {
		return nil, fmt.Errorf("%w", ErrDNS)
	}
	for _, addr := range addrs {
		if !p.Allowed(addr) {
			return nil, ErrBlocked
		}
	}
	return addrs, nil
}

func (p Policy) ValidateURL(ctx context.Context, raw string, schemes ...string) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" || u.User != nil {
		return nil, ErrBlocked
	}
	allowedScheme := false
	for _, scheme := range schemes {
		allowedScheme = allowedScheme || u.Scheme == scheme
	}
	if !allowedScheme {
		return nil, ErrBlocked
	}
	if _, err = p.Resolve(ctx, u.Hostname()); err != nil {
		return nil, err
	}
	return u, nil
}

// DialContext resolves again immediately before dialing and connects only to a
// validated address, preventing DNS rebinding between configuration and use.
func (p Policy) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}
	addrs, err := p.Resolve(ctx, host)
	if err != nil {
		return nil, err
	}
	dial := p.Dial
	if dial == nil {
		d := p.Dialer
		if d == nil {
			d = &net.Dialer{}
		}
		dial = d.DialContext
	}
	var last error
	for _, addr := range addrs {
		if conn, dialErr := dial(ctx, network, net.JoinHostPort(addr.String(), port)); dialErr == nil {
			return conn, nil
		} else {
			last = dialErr
		}
	}
	if last == nil {
		last = ErrDNS
	}
	return nil, last
}
