package host

import "testing"

func TestMounts(t *testing.T) {
	m := ParseMounts("/dev/sda / ext4 rw\nproc /proc proc rw\ntmpfs /data tmpfs rw", "/data")
	if len(m) != 2 {
		t.Fatal(m)
	}
}
