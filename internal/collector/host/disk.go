// SPDX-License-Identifier: AGPL-3.0-only
package host

import (
	"fmt"
	"strconv"
	"strings"
)

const SectorBytes = 512

type DiskCounters struct{ Reads, ReadSectors, Writes, WriteSectors uint64 }

func ParseDiskstats(input string) (map[string]DiskCounters, error) {
	out := map[string]DiskCounters{}
	for _, line := range strings.Split(input, "\n") {
		f := strings.Fields(line)
		if len(f) == 0 {
			continue
		}
		if len(f) < 10 {
			return nil, fmt.Errorf("malformed diskstats")
		}
		if strings.HasPrefix(f[2], "loop") || strings.HasPrefix(f[2], "ram") {
			continue
		}
		v := func(i int) (uint64, error) { return strconv.ParseUint(f[i], 10, 64) }
		r, e := v(3)
		if e != nil {
			return nil, e
		}
		rs, e := v(5)
		if e != nil {
			return nil, e
		}
		w, e := v(7)
		if e != nil {
			return nil, e
		}
		ws, e := v(9)
		if e != nil {
			return nil, e
		}
		out[f[2]] = DiskCounters{r, rs, w, ws}
	}
	return out, nil
}
func SectorToBytes(sectors uint64) uint64 { return sectors * SectorBytes }
