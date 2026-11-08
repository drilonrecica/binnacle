// SPDX-License-Identifier: AGPL-3.0-only
package production

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func TestSamplerReportsLiveDetail(t *testing.T) {
	proc, write := hostProcFixture(t)
	sampler := &Sampler{HostProc: proc, HostSys: sysFixture(t, "sda"), HostRoot: "/"}
	sampler.statfs = func(string, *unix.Statfs_t) error { return nil }
	stat := func(scale uint64) string {
		n := func(v uint64) string { return itoa(int(v * scale)) }
		return "cpu " + n(40) + " " + n(10) + " " + n(10) + " " + n(30) + " 0 " + n(5) + " " + n(5) + " 0\n" +
			"cpu0 " + n(30) + " 0 0 " + n(10) + " 0 0 0 0\n" +
			"cpu1 " + n(10) + " 0 0 " + n(30) + " 0 0 0 0\n" +
			"intr " + n(500) + " 1 2\nctxt " + n(1000) + "\nprocs_running 2\nprocs_blocked 1\nsoftirq " + n(200) + " 1\n"
	}
	netDev := func(bytes int) string {
		row := func(name string) string {
			return name + ": " + itoa(bytes) + " 1 0 0 0 0 0 0 " + itoa(2*bytes) + " 1 0 0 0 0 0 0\n"
		}
		return "Inter-|\n face |\n" + row("  eth0") + row("    lo") + row("vethab12")
	}
	diskstats := func(sectors int) string {
		line := func(name string) string {
			return "   8 0 " + name + " 1 0 " + itoa(sectors) + " 0 1 0 " + itoa(sectors) + " 0 0 0 0 0 0 0 0\n"
		}
		return line("sda") + line("sda1") + line("loop0")
	}
	write("meminfo", "MemTotal: 1000 kB\nMemFree: 100 kB\nMemAvailable: 400 kB\nActive: 300 kB\nInactive: 200 kB\n")
	write("stat", stat(1))
	write("1/net/dev", netDev(0))
	write("diskstats", diskstats(0))
	start := time.Now()
	first, _, _, _, err := sampler.collectHost(start)
	if err != nil {
		t.Fatal(err)
	}
	if first.ProcsRunning == nil || *first.ProcsRunning != 2 || first.ContextSwitchesPS != nil || first.CPUCores != nil {
		t.Fatalf("first tick: gauges only, no rates: %+v", first)
	}

	write("stat", stat(2))
	write("1/net/dev", netDev(1000))
	write("diskstats", diskstats(4))
	host, _, _, _, err := sampler.collectHost(start.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if host.ContextSwitchesPS == nil || *host.ContextSwitchesPS != 1000 || *host.InterruptsPS != 500 || *host.SoftInterruptsPS != 200 {
		t.Fatalf("stat rates: ctx=%v intr=%v softirq=%v", host.ContextSwitchesPS, host.InterruptsPS, host.SoftInterruptsPS)
	}
	if host.CPUIdlePercent == nil || *host.CPUIdlePercent != 30 || *host.CPUNicePercent != 10 || *host.CPUIRQPercent != 5 {
		t.Fatalf("cpu breakdown: idle=%v nice=%v irq=%v", host.CPUIdlePercent, host.CPUNicePercent, host.CPUIRQPercent)
	}
	if len(host.CPUCores) != 2 || host.CPUCores[0] != 75 || host.CPUCores[1] != 25 {
		t.Fatalf("cores %v", host.CPUCores)
	}
	if host.MemoryFreeBytes == nil || *host.MemoryFreeBytes != 100*1024 || *host.MemoryActiveBytes != 300*1024 || *host.MemoryInactiveBytes != 200*1024 {
		t.Fatalf("memory detail %+v", host)
	}
	if len(host.Interfaces) != 2 || host.Interfaces[0].Name != "eth0" || host.Interfaces[1].Name != "lo" {
		t.Fatalf("interfaces %+v (veth must be left out)", host.Interfaces)
	}
	if rx := host.Interfaces[0].RXBPS; rx == nil || *rx != 1000 || *host.Interfaces[0].TXBPS != 2000 {
		t.Fatalf("eth0 %+v", host.Interfaces[0])
	}
	if len(host.Disks) != 2 || host.Disks[0].Name != "sda" || host.Disks[1].Name != "sda1" {
		t.Fatalf("disks %+v (loop must be left out)", host.Disks)
	}
	if r := host.Disks[0].ReadBPS; r == nil || *r != 4*512 {
		t.Fatalf("sda read %v", r)
	}
	// The aggregate disk rate still counts whole devices only (COL-3).
	if host.DiskReadBPS == nil || *host.DiskReadBPS != 4*512 {
		t.Fatalf("aggregate read %v", host.DiskReadBPS)
	}
}

func TestSamplerReadsHostIdentity(t *testing.T) {
	proc, write := hostProcFixture(t)
	write("sys/kernel/osrelease", "6.8.0-137-generic\n")
	write("cpuinfo", "processor\t: 0\nmodel name\t: Intel(R) Xeon(R) CPU\n\nprocessor\t: 1\nmodel name\t: Intel(R) Xeon(R) CPU\n")
	root := t.TempDir()
	for name, value := range map[string]string{"etc/hostname": "vps-01\n", "etc/os-release": "PRETTY_NAME=\"Ubuntu 24.04.1 LTS\"\n"} {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(value), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	got := (&Sampler{HostProc: proc, HostRoot: root}).readIdentity()
	if got.Hostname != "vps-01" || got.Kernel != "6.8.0-137-generic" || got.CPUModel != "Intel(R) Xeon(R) CPU" || got.CPUCores != 2 {
		t.Fatalf("%+v", got)
	}
	// /host/etc/os-release is absent in tests, so the host root's copy is used.
	if got.OS != "Ubuntu 24.04.1 LTS" {
		t.Fatalf("os %q", got.OS)
	}
	if empty := (&Sampler{HostProc: t.TempDir(), HostRoot: t.TempDir()}).readIdentity(); empty.Hostname != "" || empty.CPUCores != 0 {
		t.Fatalf("unreadable files must leave fields empty: %+v", empty)
	}
}
