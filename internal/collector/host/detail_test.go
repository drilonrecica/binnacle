// SPDX-License-Identifier: AGPL-3.0-only
package host

import (
	"fmt"
	"testing"
)

func TestParseProcStatExtras(t *testing.T) {
	got := ParseProcStatExtras("cpu 1 2 3 4\nintr 900 1 2 3\nctxt 1200\nbtime 1\nprocs_running 3\nprocs_blocked 0\nsoftirq 77 1 2\n")
	if !got.HaveContextSwitches || got.ContextSwitches != 1200 || got.Interrupts != 900 || got.SoftInterrupts != 77 {
		t.Fatalf("%+v", got)
	}
	if !got.HaveProcsBlocked || got.ProcsBlocked != 0 || got.ProcsRunning != 3 {
		t.Fatalf("%+v", got)
	}
	if missing := ParseProcStatExtras("cpu 1 2 3 4\nctxt bad\n"); missing.HaveContextSwitches || missing.HaveInterrupts {
		t.Fatalf("malformed or missing lines must stay absent: %+v", missing)
	}
}

func TestCPUDetailSplitsFoldedStates(t *testing.T) {
	previous := CPUCounters{User: 0, Nice: 0, System: 0, Idle: 0, IRQ: 0, SoftIRQ: 0}
	current := CPUCounters{User: 40, Nice: 10, System: 10, Idle: 30, IRQ: 5, SoftIRQ: 5}
	got := CPUDetail(previous, current)
	for name, want := range map[string]struct {
		v    *float64
		want float64
	}{"nice": {got.Nice, 10}, "irq": {got.IRQ, 5}, "softirq": {got.SoftIRQ, 5}, "idle": {got.Idle, 30}} {
		if want.v == nil || *want.v != want.want {
			t.Fatalf("%s = %v, want %v", name, want.v, want.want)
		}
	}
	if (CPUDetail(current, current) != CPUBreakdown{}) {
		t.Fatal("no elapsed time must give no breakdown")
	}
}

func TestCoreBusyKeepsCoreOrder(t *testing.T) {
	previous := map[string]CPUCounters{"cpu": {}, "cpu0": {}, "cpu1": {}}
	current := map[string]CPUCounters{
		"cpu":  {User: 100, Idle: 100},
		"cpu0": {User: 75, Idle: 25},
		"cpu1": {User: 25, Idle: 75},
		"cpu2": {User: 50, Idle: 50}, // new core: no previous sample yet
	}
	got := CoreBusy(previous, current)
	if len(got) != 3 || got[0] != 75 || got[1] != 25 || got[2] != 0 {
		t.Fatalf("%v", got)
	}
	if CoreBusy(nil, map[string]CPUCounters{"cpu": {}}) != nil {
		t.Fatal("no per-core lines must give no list")
	}
}

func TestInterfaceRatesSkipVethAndAreBounded(t *testing.T) {
	previous := map[string]NetworkCounters{"eth0": {RXBytes: 0, TXBytes: 0}}
	current := map[string]NetworkCounters{
		"eth0":     {RXBytes: 2000, TXBytes: 4000},
		"lo":       {RXBytes: 10},
		"vethabc1": {RXBytes: 99},
		"docker0":  {},
	}
	got := InterfaceRates(previous, current, 2)
	if len(got) != 3 || got[0].Name != "docker0" || got[1].Name != "eth0" || got[2].Name != "lo" {
		t.Fatalf("%+v", got)
	}
	if got[1].RXBPS == nil || *got[1].RXBPS != 1000 || *got[1].TXBPS != 2000 {
		t.Fatalf("eth0 rates: %+v", got[1])
	}
	if got[2].RXBPS != nil {
		t.Fatal("an interface without a previous sample has no rate yet")
	}
	many := map[string]NetworkCounters{}
	for index := 0; index < MaxDetailDevices+10; index++ {
		many[fmt.Sprintf("eth%02d", index)] = NetworkCounters{}
	}
	if n := len(InterfaceRates(nil, many, 2)); n != MaxDetailDevices {
		t.Fatalf("got %d interfaces, want the bound %d", n, MaxDetailDevices)
	}
}

func TestDeviceRatesSkipStackedDevices(t *testing.T) {
	previous := map[string]DiskCounters{"sda": {}, "sda1": {}}
	current := map[string]DiskCounters{
		"sda":   {Reads: 10, ReadSectors: 8, Writes: 20, WriteSectors: 16},
		"sda1":  {Reads: 4},
		"loop0": {Reads: 99},
		"dm-0":  {Reads: 99},
		"zram0": {Reads: 99},
	}
	got := DeviceRates(previous, current, 2)
	if len(got) != 2 || got[0].Name != "sda" || got[1].Name != "sda1" {
		t.Fatalf("%+v", got)
	}
	if *got[0].ReadBPS != 2048 || *got[0].WriteBPS != 4096 || *got[0].ReadIOPS != 5 || *got[0].WriteIOPS != 10 {
		t.Fatalf("sda rates: %+v", got[0])
	}
}

func TestParseOSRelease(t *testing.T) {
	if got := ParseOSRelease("NAME=\"Ubuntu\"\nVERSION_ID=\"24.04\"\nPRETTY_NAME=\"Ubuntu 24.04.1 LTS\"\n"); got != "Ubuntu 24.04.1 LTS" {
		t.Fatalf("%q", got)
	}
	if got := ParseOSRelease("# comment\nNAME=Alpine Linux\nVERSION_ID=3.22.2\n"); got != "Alpine Linux 3.22.2" {
		t.Fatalf("%q", got)
	}
}

func TestParseCPUInfo(t *testing.T) {
	x86 := "processor\t: 0\nmodel name\t: Intel(R) Xeon(R) CPU\n\nprocessor\t: 1\nmodel name\t: Intel(R) Xeon(R) CPU\n"
	if model, cores := ParseCPUInfo(x86); model != "Intel(R) Xeon(R) CPU" || cores != 2 {
		t.Fatalf("%q %d", model, cores)
	}
	arm := "processor\t: 0\nBogoMIPS\t: 108\n\nprocessor\t: 1\n\nModel\t: Raspberry Pi 5 Model B\n"
	if model, cores := ParseCPUInfo(arm); model != "Raspberry Pi 5 Model B" || cores != 2 {
		t.Fatalf("%q %d", model, cores)
	}
}

func TestMeminfoDetail(t *testing.T) {
	m, err := ParseMeminfo("MemTotal: 100 kB\nMemFree: 10 kB\nMemAvailable: 40 kB\nActive: 30 kB\nInactive: 20 kB\nActive(anon): 5 kB\n")
	if err != nil || m.Free != 10*1024 || m.Active != 30*1024 || m.Inactive != 20*1024 {
		t.Fatalf("%+v %v", m, err)
	}
}
