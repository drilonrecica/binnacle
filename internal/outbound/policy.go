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

func (p Policy) Allowed(addr netip.Addr) bool {
	addr = addr.Unmap()
	if !addr.IsValid() || !addr.IsGlobalUnicast() || addr.IsUnspecified() || addr.IsLoopback() || addr.IsLinkLocalUnicast() || addr.IsLinkLocalMulticast() || addr.IsMulticast() {
		return false
	}
	if _, blocked := metadataAddresses[addr]; blocked {
		return false
	}
	return p.AllowPrivate || !addr.IsPrivate()
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
