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
