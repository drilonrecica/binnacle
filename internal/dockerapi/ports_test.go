// SPDX-License-Identifier: AGPL-3.0-only
package dockerapi

import (
	"net/netip"
	"reflect"
	"testing"

	containertypes "github.com/moby/moby/api/types/container"
)

func TestFormatPorts(t *testing.T) {
	ports := []containertypes.PortSummary{
		{IP: netip.MustParseAddr("0.0.0.0"), PrivatePort: 80, PublicPort: 8080, Type: "tcp"},
		{IP: netip.MustParseAddr("::"), PrivatePort: 80, PublicPort: 8080, Type: "tcp"}, // same binding over IPv6
		{PrivatePort: 5432, Type: "tcp"}, // exposed, not published
		{PrivatePort: 443, PublicPort: 443, Type: "udp"},
	}
	want := []string{"443->443/udp", "5432/tcp", "8080->80/tcp"}
	if got := formatPorts(ports); !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	var many []containertypes.PortSummary
	for port := uint16(1); port <= maxPorts+5; port++ {
		many = append(many, containertypes.PortSummary{PrivatePort: port, Type: "tcp"})
	}
	if n := len(formatPorts(many)); n != maxPorts {
		t.Fatalf("got %d ports, want the bound %d", n, maxPorts)
	}
}
