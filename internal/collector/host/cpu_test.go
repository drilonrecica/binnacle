package host

import (
	"strings"
	"testing"
)

func TestCPU(t *testing.T) {
	v, e := ParseProcStat("cpu 10 0 10 80 0 0 0 0\ncpu0 10 0 10 80\n")
	if e != nil || len(v) != 2 {
		t.Fatal(e)
	}
	d := CPUDelta(v["cpu"], CPUCounters{User: 20, System: 20, Idle: 160})
	if d.Busy == nil || *d.Busy != 20 {
		t.Fatalf("%+v", d)
	}
	if CPUDelta(CPUCounters{Idle: 2}, CPUCounters{Idle: 1}).Busy != nil {
		t.Fatal("reset")
	}
	_, e = ParseProcStat(strings.TrimSpace("cpu x"))
	if e == nil {
		t.Fatal("malformed")
	}
}

func TestCPUCountsIRQAndSoftIRQAsBusy(t *testing.T) {
	before, e := ParseProcStat("cpu 10 0 10 80 0 0 0 0\n")
	if e != nil {
		t.Fatal(e)
	}
	// 100 jiffies elapse: 10 user, 10 system, 15 irq, 15 softirq, 50 idle.
	after, e := ParseProcStat("cpu 20 0 20 130 0 15 15 0\n")
	if e != nil {
		t.Fatal(e)
	}
	if after["cpu"].IRQ != 15 || after["cpu"].SoftIRQ != 15 {
		t.Fatalf("counters %+v", after["cpu"])
	}
	d := CPUDelta(before["cpu"], after["cpu"])
	if d.Busy == nil || *d.Busy != 50 {
		t.Fatalf("busy %v, want 50", d.Busy)
	}
}

func TestCPUShortLineLeavesIRQZero(t *testing.T) {
	v, e := ParseProcStat("cpu 10 0 10 80 5\n")
	if e != nil || v["cpu"].IOWait != 5 || v["cpu"].IRQ != 0 || v["cpu"].SoftIRQ != 0 {
		t.Fatalf("%+v %v", v, e)
	}
}
