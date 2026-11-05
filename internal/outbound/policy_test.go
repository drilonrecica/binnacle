// SPDX-License-Identifier: AGPL-3.0-only

package outbound

import (
	"context"
	"net"
	"net/netip"
	"testing"
)

type resolver map[string][]netip.Addr

func (r resolver) LookupNetIP(_ context.Context, _ string, host string) ([]netip.Addr, error) {
	return r[host], nil
}

func TestPolicyBlocksUnsafeTargets(t *testing.T) {
	cases := []struct {
		name, host, address string
		private             bool
	}{{"loopback", "loop.test", "127.0.0.1", true}, {"private", "private.test", "10.0.0.1", false}, {"link-local", "metadata.test", "169.254.169.254", true}, {"metadata alternate", "metadata-alt.test", "100.100.100.200", true}, {"multicast", "multicast.test", "224.0.0.1", true}, {"unspecified", "unspecified.test", "0.0.0.0", true}, {"broadcast", "broadcast.test", "255.255.255.255", true}}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			p := Policy{AllowPrivate: tt.private, Resolver: resolver{tt.host: {netip.MustParseAddr(tt.address)}}}
			if _, err := p.ValidateURL(context.Background(), "https://"+tt.host, "https"); err == nil {
				t.Fatal("unsafe target was accepted")
			}
		})
	}
	p := Policy{AllowPrivate: true, Resolver: resolver{"private.test": {netip.MustParseAddr("10.0.0.1")}}}
	if _, err := p.ValidateURL(context.Background(), "https://private.test", "https"); err != nil {
		t.Fatalf("private opt-in was rejected: %v", err)
	}
	if _, err := p.ValidateURL(context.Background(), "https://service.localhost", "https"); err == nil {
		t.Fatal(".localhost was accepted")
	}
}

func TestPolicyRevalidatesDNSAtDialTime(t *testing.T) {
	resolved := resolver{"service.test": {netip.MustParseAddr("203.0.113.10")}}
	p := Policy{Resolver: resolved, Dial: func(context.Context, string, string) (net.Conn, error) {
		t.Fatal("dial called after rebinding to blocked target")
		return nil, nil
	}}
	if _, err := p.ValidateURL(context.Background(), "https://service.test", "https"); err != nil {
		t.Fatal(err)
	}
	resolved["service.test"] = []netip.Addr{netip.MustParseAddr("127.0.0.1")}
	if _, err := p.DialContext(context.Background(), "tcp", "service.test:443"); err == nil {
		t.Fatal("DNS rebinding target was accepted")
	}
}

func TestPolicyBlocksReservedAndTranslatedRanges(t *testing.T) {
	cases := []struct {
		name, address  string
		private, allow bool
	}{
		{"cgnat without opt-in", "100.64.0.1", false, false},
		{"cgnat with opt-in", "100.64.0.1", true, true},
		{"this network", "0.1.2.3", true, false},
		{"ietf protocol assignments", "192.0.0.1", true, false},
		{"benchmarking low", "198.18.0.1", true, false},
		{"benchmarking high", "198.19.255.255", true, false},
		{"reserved class e", "240.0.0.1", true, false},
		{"nat64 loopback", "64:ff9b::7f00:1", true, false},
		{"nat64 metadata", "64:ff9b::a9fe:a9fe", true, false},
		{"nat64 private without opt-in", "64:ff9b::a00:1", false, false},
		{"nat64 public without opt-in", "64:ff9b::cb00:7101", false, false},
		{"nat64 public with opt-in", "64:ff9b::cb00:7101", true, true},
		{"6to4 loopback", "2002:7f00:1::", true, false},
		{"6to4 reserved", "2002:f000:1::", true, false},
		{"6to4 public without opt-in", "2002:cb00:7101::", false, false},
		{"6to4 public with opt-in", "2002:cb00:7101::", true, true},
		{"ipv4 compatible loopback", "::7f00:1", true, false},
		{"public", "203.0.113.1", false, true},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			p := Policy{AllowPrivate: tt.private}
			if got := p.Allowed(netip.MustParseAddr(tt.address)); got != tt.allow {
				t.Fatalf("Allowed(%s) private=%v = %v, want %v", tt.address, tt.private, got, tt.allow)
			}
		})
	}
}
