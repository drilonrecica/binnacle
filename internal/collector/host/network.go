// SPDX-License-Identifier: AGPL-3.0-only
package host

import (
	"fmt"
	"strconv"
	"strings"
)

type NetworkCounters struct{ RXBytes, RXPackets, RXErrors, RXDrops, TXBytes, TXPackets, TXErrors, TXDrops uint64 }

func ParseNetDev(input string) (map[string]NetworkCounters, error) {
	out := map[string]NetworkCounters{}
	for _, line := range strings.Split(input, "\n") {
		p := strings.Split(line, ":")
		if len(p) != 2 {
			continue
		}
		f := strings.Fields(p[1])
		if len(f) < 16 {
			return nil, fmt.Errorf("malformed interface")
		}
		v := make([]uint64, 16)
		for i := range v {
			n, e := strconv.ParseUint(f[i], 10, 64)
			if e != nil {
				return nil, e
			}
			v[i] = n
		}
		out[strings.TrimSpace(p[0])] = NetworkCounters{v[0], v[1], v[2], v[3], v[8], v[9], v[10], v[11]}
	}
	return out, nil
}
func AggregateNetwork(in map[string]NetworkCounters) NetworkCounters {
	var out NetworkCounters
	for name, c := range in {
		if name == "lo" || strings.HasPrefix(name, "veth") || strings.HasPrefix(name, "docker") || strings.HasPrefix(name, "br-") {
			continue
		}
		out.RXBytes += c.RXBytes
		out.TXBytes += c.TXBytes
		out.RXPackets += c.RXPackets
		out.TXPackets += c.TXPackets
		out.RXErrors += c.RXErrors
		out.TXErrors += c.TXErrors
		out.RXDrops += c.RXDrops
		out.TXDrops += c.TXDrops
	}
	return out
}
func Rate(current, previous uint64, elapsed float64) *float64 {
	if elapsed <= 0 || current < previous {
		return nil
	}
	v := float64(current-previous) / elapsed
	return &v
}
