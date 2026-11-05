package host

import (
	"os"
	"reflect"
	"testing"
)

func readFixture(t *testing.T, name string) string {
	t.Helper()
	raw, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func TestParseMountinfoFixture(t *testing.T) {
	mounts, err := ParseMountinfo(readFixture(t, "mountinfo"))
	if err != nil {
		t.Fatal(err)
	}
	if len(mounts) != 16 {
		t.Fatalf("parsed %d mounts", len(mounts))
	}
	root := mounts[5]
	if root != (Mount{Major: 8, Minor: 1, Root: "/", Target: "/", FSType: "ext4", Source: "/dev/sda1"}) {
		t.Fatalf("root mount %+v", root)
	}
	// Optional fields of variable count must not shift the fields after "-".
	if overlay := mounts[14]; overlay.FSType != "overlay" || overlay.Source != "overlay" || overlay.Device() != "0:52" {
		t.Fatalf("overlay mount %+v", overlay)
	}
	if backup := mounts[11]; backup.Target != "/mnt/backup disk" || backup.FSType != "xfs" {
		t.Fatalf("escaped target %+v", backup)
	}
}

func TestSelectFilesystemsDedupesAndDropsPseudo(t *testing.T) {
	mounts, err := ParseMountinfo(readFixture(t, "mountinfo"))
	if err != nil {
		t.Fatal(err)
	}
	var targets []string
	for _, m := range SelectFilesystems(mounts, "") {
		targets = append(targets, m.Target)
	}
	want := []string{"/", "/boot/efi", "/mnt/backup disk", "/tmp"}
	if !reflect.DeepEqual(targets, want) {
		t.Fatalf("targets %q want %q", targets, want)
	}
}

func TestSelectFilesystemsKeepsDataDevice(t *testing.T) {
	mounts := []Mount{
		{Major: 8, Minor: 1, Root: "/", Target: "/", FSType: "ext4", Source: "/dev/sda1"},
		{Major: 0, Minor: 40, Root: "/", Target: "/run/binnacle", FSType: "tmpfs", Source: "tmpfs"},
	}
	got := SelectFilesystems(mounts, "0:40")
	if len(got) != 2 || got[1].Target != "/run/binnacle" {
		t.Fatalf("%+v", got)
	}
}

// The old parser read mountinfo as /proc/mounts, taking the mount id as the
// source and maj:min as the target. These lines must parse by the separator.
func TestParseMountinfoRejectsMountsFormat(t *testing.T) {
	if _, err := ParseMountinfo("/dev/sda1 / ext4 rw,relatime 0 0\n"); err == nil {
		t.Fatal("a /proc/mounts line must not parse as mountinfo")
	}
	m, err := ParseMountinfo("36 35 98:0 /mnt1 /mnt2 rw,noatime master:1 - ext3 /dev/root rw,errors=continue\n")
	if err != nil || len(m) != 1 || m[0].Target != "/mnt2" || m[0].Root != "/mnt1" || m[0].Source != "/dev/root" || m[0].Device() != "98:0" {
		t.Fatalf("%+v %v", m, err)
	}
}
