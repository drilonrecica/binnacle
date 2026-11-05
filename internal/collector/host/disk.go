// SPDX-License-Identifier: AGPL-3.0-only
package host

import (
	"fmt"
	"os"
	"path/filepath"
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

// stackedDevicePrefixes name block devices whose I/O is also counted on the
// physical disks beneath them (device-mapper, md RAID) or that are not disks
// at all (zram, loop, ram).
var stackedDevicePrefixes = []string{"dm-", "md", "zram", "loop", "ram"}

// WholeDevices lists the physical whole devices under sysBlock (<HostSys>/block).
// /sys/block holds no partitions, but an entry carrying a "partition" file is
// skipped anyway, as are stacked and virtual devices, so each byte of disk I/O
// is counted once (COL-3).
func WholeDevices(sysBlock string) (map[string]bool, error) {
	entries, err := os.ReadDir(sysBlock)
	if err != nil {
		return nil, err
	}
	out := make(map[string]bool, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if stackedDevice(name) {
			continue
		}
		if _, err := os.Stat(filepath.Join(sysBlock, name, "partition")); err == nil {
			continue
		}
		out[name] = true
	}
	return out, nil
}

func stackedDevice(name string) bool {
	for _, prefix := range stackedDevicePrefixes {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}

// SumWholeDevices adds the counters of the diskstats devices present in whole.
func SumWholeDevices(disks map[string]DiskCounters, whole map[string]bool) DiskCounters {
	var total DiskCounters
	for name, d := range disks {
		if !whole[name] || stackedDevice(name) {
			continue
		}
		total.Reads += d.Reads
		total.ReadSectors += d.ReadSectors
		total.Writes += d.Writes
		total.WriteSectors += d.WriteSectors
	}
	return total
}
