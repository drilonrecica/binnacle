// SPDX-License-Identifier: AGPL-3.0-only
package host

import (
	"bufio"
	"sort"
	"strconv"
	"strings"

	"github.com/drilonrecica/binnacle/internal/metrics"
)

// MaxDetailDevices bounds the per-interface and per-device lists sent with
// every live frame, so a host with many NICs or disks cannot grow the frame
// without limit (ADR 014).
const MaxDetailDevices = 16

// StatExtras are the system-wide counters /proc/stat carries next to the CPU
// lines: cumulative context switches, interrupts and soft interrupts, and the
// current number of runnable and blocked tasks.
type StatExtras struct {
	ContextSwitches, Interrupts, SoftInterrupts uint64
	ProcsRunning, ProcsBlocked                  uint64
	// Have marks which counters were present, since a zero is a valid value.
	HaveContextSwitches, HaveInterrupts, HaveSoftInterrupts bool
	HaveProcsRunning, HaveProcsBlocked                      bool
}

// ParseProcStatExtras reads the non-CPU lines of /proc/stat. Malformed or
// missing lines leave the counter absent rather than failing the sample.
func ParseProcStatExtras(input string) StatExtras {
	var out StatExtras
	for _, line := range strings.Split(input, "\n") {
		f := strings.Fields(line)
		if len(f) < 2 {
			continue
		}
		n, err := strconv.ParseUint(f[1], 10, 64)
		if err != nil {
			continue
		}
		switch f[0] {
		case "ctxt":
			out.ContextSwitches, out.HaveContextSwitches = n, true
		case "intr":
			out.Interrupts, out.HaveInterrupts = n, true
		case "softirq":
			out.SoftInterrupts, out.HaveSoftInterrupts = n, true
		case "procs_running":
			out.ProcsRunning, out.HaveProcsRunning = n, true
		case "procs_blocked":
			out.ProcsBlocked, out.HaveProcsBlocked = n, true
		}
	}
	return out
}

// CPUBreakdown is the share of CPU time in the states CPUUsage folds away.
type CPUBreakdown struct{ Nice, IRQ, SoftIRQ, Idle *float64 }

// CPUDetail splits the delta between two samples into nice, irq, softirq and
// idle percentages of all CPU time.
func CPUDetail(previous, current CPUCounters) CPUBreakdown {
	total := func(c CPUCounters) uint64 {
		return c.User + c.Nice + c.System + c.Idle + c.IOWait + c.IRQ + c.SoftIRQ + c.Steal
	}
	p, c := total(previous), total(current)
	if c <= p {
		return CPUBreakdown{}
	}
	d := float64(c - p)
	pct := func(now, old uint64) *float64 {
		if now < old {
			return nil
		}
		v := float64(now-old) * 100 / d
		return &v
	}
	return CPUBreakdown{
		Nice:    pct(current.Nice, previous.Nice),
		IRQ:     pct(current.IRQ, previous.IRQ),
		SoftIRQ: pct(current.SoftIRQ, previous.SoftIRQ),
		Idle:    pct(current.Idle, previous.Idle),
	}
}

// CoreBusy returns the busy percentage of each core (cpu0, cpu1, ...) in
// core order. A core missing from either sample, or one whose counters went
// backwards, reads as 0 so the list keeps one entry per core.
func CoreBusy(previous, current map[string]CPUCounters) []float64 {
	cores := 0
	for name := range current {
		if index, ok := coreIndex(name); ok && index+1 > cores {
			cores = index + 1
		}
	}
	if cores == 0 {
		return nil
	}
	out := make([]float64, cores)
	for index := range out {
		name := "cpu" + strconv.Itoa(index)
		now, okNow := current[name]
		old, okOld := previous[name]
		if !okNow || !okOld {
			continue
		}
		if busy := CPUDelta(old, now).Busy; busy != nil {
			out[index] = *busy
		}
	}
	return out
}

func coreIndex(name string) (int, bool) {
	if !strings.HasPrefix(name, "cpu") || name == "cpu" {
		return 0, false
	}
	index, err := strconv.Atoi(strings.TrimPrefix(name, "cpu"))
	return index, err == nil && index >= 0
}

// detailInterface reports whether an interface belongs in the live list:
// per-container veth pairs are left out, they are one per container and
// their traffic already shows on each resource.
func detailInterface(name string) bool { return !strings.HasPrefix(name, "veth") }

// InterfaceRates computes per-interface throughput between two samples,
// sorted by name and bounded by MaxDetailDevices.
func InterfaceRates(previous, current map[string]NetworkCounters, elapsed float64) []metrics.InterfaceRate {
	names := make([]string, 0, len(current))
	for name := range current {
		if detailInterface(name) {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	if len(names) > MaxDetailDevices {
		names = names[:MaxDetailDevices]
	}
	out := make([]metrics.InterfaceRate, 0, len(names))
	for _, name := range names {
		row := metrics.InterfaceRate{Name: name}
		if old, ok := previous[name]; ok {
			now := current[name]
			row.RXBPS = Rate(now.RXBytes, old.RXBytes, elapsed)
			row.TXBPS = Rate(now.TXBytes, old.TXBytes, elapsed)
		}
		out = append(out, row)
	}
	return out
}

// DeviceRates computes per-device disk throughput between two samples for
// disks and their partitions, leaving out stacked and virtual devices
// (device-mapper, md, loop, ram, zram); sorted by name, bounded by
// MaxDetailDevices.
func DeviceRates(previous, current map[string]DiskCounters, elapsed float64) []metrics.DiskDeviceRate {
	names := make([]string, 0, len(current))
	for name := range current {
		if !stackedDevice(name) {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	if len(names) > MaxDetailDevices {
		names = names[:MaxDetailDevices]
	}
	out := make([]metrics.DiskDeviceRate, 0, len(names))
	for _, name := range names {
		row := metrics.DiskDeviceRate{Name: name}
		if old, ok := previous[name]; ok {
			now := current[name]
			row.ReadBPS = Rate(SectorToBytes(now.ReadSectors), SectorToBytes(old.ReadSectors), elapsed)
			row.WriteBPS = Rate(SectorToBytes(now.WriteSectors), SectorToBytes(old.WriteSectors), elapsed)
			row.ReadIOPS = Rate(now.Reads, old.Reads, elapsed)
			row.WriteIOPS = Rate(now.Writes, old.Writes, elapsed)
		}
		out = append(out, row)
	}
	return out
}

// ParseOSRelease returns PRETTY_NAME from an os-release file, falling back to
// NAME and VERSION_ID.
func ParseOSRelease(input string) string {
	values := map[string]string{}
	scanner := bufio.NewScanner(strings.NewReader(input))
	for scanner.Scan() {
		key, value, ok := strings.Cut(strings.TrimSpace(scanner.Text()), "=")
		if !ok || strings.HasPrefix(key, "#") {
			continue
		}
		if unquoted, err := strconv.Unquote(value); err == nil {
			value = unquoted
		} else {
			value = strings.Trim(value, `"'`)
		}
		values[key] = value
	}
	if pretty := values["PRETTY_NAME"]; pretty != "" {
		return pretty
	}
	return strings.TrimSpace(values["NAME"] + " " + values["VERSION_ID"])
}

// ParseCPUInfo returns the CPU model name and the number of logical CPUs
// listed in /proc/cpuinfo. Architectures without "model name" (some ARM
// kernels) fall back to "Hardware" or "Model".
func ParseCPUInfo(input string) (model string, cores int) {
	var hardware string
	scanner := bufio.NewScanner(strings.NewReader(input))
	for scanner.Scan() {
		key, value, ok := strings.Cut(scanner.Text(), ":")
		if !ok {
			continue
		}
		key, value = strings.TrimSpace(key), strings.TrimSpace(value)
		switch key {
		case "processor":
			cores++
		case "model name":
			if model == "" {
				model = value
			}
		case "Hardware", "Model":
			if hardware == "" {
				hardware = value
			}
		}
	}
	if model == "" {
		model = hardware
	}
	return model, cores
}
