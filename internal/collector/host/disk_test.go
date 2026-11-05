package host

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDisk(t *testing.T) {
	d, e := ParseDiskstats("8 0 sda 2 0 3 0 4 0 5 0 0 0 0\n7 0 loop0 1 0 1 0 0 0 0 0")
	if e != nil || d["sda"].ReadSectors != 3 || SectorToBytes(1) != 512 {
		t.Fatal(d, e)
	}
}

// sysBlockFixture builds a /sys/block with the given device directories; a
// device listed in partitions also carries a "partition" file.
func sysBlockFixture(t *testing.T, devices []string, partitions ...string) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "block")
	for _, name := range devices {
		if err := os.MkdirAll(filepath.Join(root, name), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range partitions {
		if err := os.WriteFile(filepath.Join(root, name, "partition"), []byte("1\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func TestWholeDeviceDiskIOCountsEachByteOnce(t *testing.T) {
	// One 100-sector write to an LVM volume on sda2 appears on dm-0, sda2 and
	// sda; md0 mirrors nvme0n1 and nvme1n1; zram0, loop0 and ram0 are virtual.
	stats := "" +
		"   8  0 sda     10 0 100 0 1 0 100 0 0 0 0 0 0 0 0\n" +
		"   8  1 sda1     0 0   0 0 0 0   0 0 0 0 0 0 0 0 0\n" +
		"   8  2 sda2    10 0 100 0 1 0 100 0 0 0 0 0 0 0 0\n" +
		" 253  0 dm-0    10 0 100 0 1 0 100 0 0 0 0 0 0 0 0\n" +
		" 259  0 nvme0n1  1 0   8 0 2 0  16 0 0 0 0 0 0 0 0\n" +
		" 259  1 nvme0n1p1 1 0  8 0 2 0  16 0 0 0 0 0 0 0 0\n" +
		" 259  2 nvme1n1  1 0   8 0 2 0  16 0 0 0 0 0 0 0 0\n" +
		"   9  0 md0      1 0   8 0 2 0  16 0 0 0 0 0 0 0 0\n" +
		" 252  0 zram0    5 0  50 0 5 0  50 0 0 0 0 0 0 0 0\n" +
		"   7  0 loop0    5 0  50 0 5 0  50 0 0 0 0 0 0 0 0\n" +
		"   1  0 ram0     5 0  50 0 5 0  50 0 0 0 0 0 0 0 0\n"
	disks, err := ParseDiskstats(stats)
	if err != nil {
		t.Fatal(err)
	}
	// sdb is a whole device that a stale sysfs could misreport as a partition.
	sys := sysBlockFixture(t, []string{"sda", "nvme0n1", "nvme1n1", "dm-0", "md0", "zram0", "loop0", "ram0", "sdb"}, "sdb")
	whole, err := WholeDevices(sys)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{"sda": true, "nvme0n1": true, "nvme1n1": true}
	if len(whole) != len(want) {
		t.Fatalf("whole devices %v, want %v", whole, want)
	}
	for name := range want {
		if !whole[name] {
			t.Fatalf("whole devices %v, want %v", whole, want)
		}
	}
	got := SumWholeDevices(disks, whole)
	if got != (DiskCounters{Reads: 12, ReadSectors: 116, Writes: 5, WriteSectors: 132}) {
		t.Fatalf("sum %+v", got)
	}
	if _, err := WholeDevices(filepath.Join(sys, "missing")); err == nil {
		t.Fatal("missing /sys/block must be an error, not an empty set")
	}
}
