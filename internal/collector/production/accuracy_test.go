// SPDX-License-Identifier: AGPL-3.0-only
package production

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/drilonrecica/binnacle/internal/dockerapi"
	"golang.org/x/sys/unix"
)

// sysFixture builds a host sysfs whose block/ lists the given whole devices.
func sysFixture(t *testing.T, devices ...string) string {
	t.Helper()
	root := t.TempDir()
	for _, name := range devices {
		if err := os.MkdirAll(filepath.Join(root, "block", name), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func TestSamplerDiskIOCountsWholeDevicesOnce(t *testing.T) {
	proc, write := hostProcFixture(t)
	sampler := &Sampler{HostProc: proc, HostSys: sysFixture(t, "sda", "dm-0"), HostRoot: "/"}
	sampler.statfs = func(string, *unix.Statfs_t) error { return nil }
	diskstats := func(sectors int) string {
		line := func(name string) string {
			return "   8 0 " + name + " 0 0 " + itoa(sectors) + " 0 0 0 0 0 0 0 0 0 0 0 0\n"
		}
		return line("sda") + line("sda1") + line("dm-0")
	}
	write("diskstats", diskstats(0))
	start := time.Now()
	if _, _, _, reason, err := sampler.collectHost(start); err != nil || reason != "" {
		t.Fatalf("err=%v reason=%q", err, reason)
	}
	write("diskstats", diskstats(4))
	host, _, _, _, err := sampler.collectHost(start.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if host.DiskReadBPS == nil || *host.DiskReadBPS != 4*512 {
		t.Fatalf("read B/s %v, want %d", host.DiskReadBPS, 4*512)
	}
}

func TestSamplerDiskIOUnknownWithoutSysBlock(t *testing.T) {
	proc, write := hostProcFixture(t)
	write("diskstats", "   8 0 sda 0 0 8 0 0 0 0 0 0 0 0 0 0 0 0\n")
	sampler := &Sampler{HostProc: proc, HostSys: t.TempDir(), HostRoot: "/"}
	sampler.statfs = func(string, *unix.Statfs_t) error { return nil }
	start := time.Now()
	sampler.collectHost(start)
	host, _, _, reason, err := sampler.collectHost(start.Add(time.Second))
	if err != nil || host.DiskReadBPS != nil || host.DiskWriteIOPS != nil {
		t.Fatalf("err=%v read=%v, want unknown disk I/O", err, host.DiskReadBPS)
	}
	if !strings.HasPrefix(reason, "disk I/O unavailable: ") {
		t.Fatalf("reason %q", reason)
	}
}

// hasMonotonic reports whether t carries a monotonic clock reading, which
// time.Time prints as an "m=" suffix.
func hasMonotonic(t time.Time) bool { return strings.Contains(t.String(), " m=") }

func TestSamplerKeepsMonotonicTimeForRates(t *testing.T) {
	proc, _ := hostProcFixture(t)
	docker := &fakeDocker{stats: dockerapi.Stats{Memory: dockerapi.MemoryStats{Usage: 1}}}
	sampler := &Sampler{HostProc: proc, HostSys: sysFixture(t, "sda"), HostRoot: "/", Docker: docker}
	sampler.statfs = func(string, *unix.Statfs_t) error { return nil }
	tick := time.Now()
	host, _, _, _, err := sampler.collectHost(tick)
	if err != nil {
		t.Fatal(err)
	}
	resources, _, err := sampler.collectDocker(context.Background(), tick, host.MemoryTotalBytes)
	if err != nil || len(resources) != 1 {
		t.Fatalf("resources=%+v err=%v", resources, err)
	}
	for name, at := range map[string]time.Time{"network": sampler.previousNetworkAt, "disk": sampler.previousDiskAt, "docker": sampler.previousStats["123456789012"].at} {
		if !hasMonotonic(at) {
			t.Errorf("%s baseline %v lost its monotonic reading; a wall-clock step would skew rates", name, at)
		}
	}
	for name, at := range map[string]time.Time{"host": host.At, "resource": resources[0].LastSeenAt} {
		if at.Location() != time.UTC || hasMonotonic(at) {
			t.Errorf("%s stamp %v, want the stored UTC copy", name, at)
		}
	}
}

func TestSamplerHostNetworkContainerHasUnknownNetworkIO(t *testing.T) {
	docker := &fakeDocker{stats: dockerapi.Stats{Memory: dockerapi.MemoryStats{Usage: 1}, IO: dockerapi.IOCounters{Read: 100}}}
	sampler := &Sampler{Docker: docker}
	tick := time.Now()
	if _, _, err := sampler.collectDocker(context.Background(), tick, nil); err != nil {
		t.Fatal(err)
	}
	docker.stats.IO.Read = 300
	resources, _, err := sampler.collectDocker(context.Background(), tick.Add(2*time.Second), nil)
	if err != nil || len(resources) != 1 {
		t.Fatalf("resources=%+v err=%v", resources, err)
	}
	r := resources[0]
	if r.RXBPS != nil || r.TXBPS != nil || r.Components[0].RXBPS != nil {
		t.Fatalf("rx=%v tx=%v, want nil for a container without networks", r.RXBPS, r.TXBPS)
	}
	if r.BlockReadBPS == nil || *r.BlockReadBPS != 100 {
		t.Fatalf("block read %v, want 100", r.BlockReadBPS)
	}
}

func itoa(v int) string { return strconv.Itoa(v) }
