package host

import "testing"

func TestDisk(t *testing.T) {
	d, e := ParseDiskstats("8 0 sda 2 0 3 0 4 0 5 0 0 0 0\n7 0 loop0 1 0 1 0 0 0 0 0")
	if e != nil || d["sda"].ReadSectors != 3 || SectorToBytes(1) != 512 {
		t.Fatal(d, e)
	}
}
