// SPDX-License-Identifier: AGPL-3.0-only
package production

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/drilonrecica/binnacle/internal/metrics"
	"golang.org/x/sys/unix"
)

// hostProcFixture builds a host procfs whose PID 1 files are the kernel-format
// fixtures and whose self/ and top-level net/dev hold the container's own view,
// which the sampler must not read (COL-1, COL-2).
func hostProcFixture(t *testing.T) (string, func(name, value string)) {
	t.Helper()
	root := t.TempDir()
	write := func(name, value string) {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(value), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	fixture := func(name string) string {
		raw, err := os.ReadFile(filepath.Join("..", "host", "testdata", name))
		if err != nil {
			t.Fatal(err)
		}
		return string(raw)
	}
	write("stat", "cpu 10 0 10 80 0 0 0 0\n")
	write("meminfo", "MemTotal: 1000 kB\nMemAvailable: 400 kB\n")
	write("loadavg", "0.25 0.20 0.10 1/10 1\n")
	write("uptime", "100.0 50.0\n")
	write("diskstats", "")
	write("1/mountinfo", fixture("mountinfo"))
	write("1/net/dev", fixture("net_dev"))
	container := "1200 1100 0:90 / / rw,relatime - overlay overlay rw\n1201 1200 8:1 /var/lib/docker/volumes/data/_data /var/lib/binnacle rw - ext4 /dev/sda1 rw\n"
	write("self/mountinfo", container)
	write("mounts", "overlay / overlay rw 0 0\n")
	write("net/dev", "  eth0: 5 5 0 0 0 0 0 0 5 5 0 0 0 0 0 0\n    lo: 1 1 0 0 0 0 0 0 1 1 0 0 0 0 0 0\n")
	return root, write
}

func TestSamplerReadsHostMountNamespace(t *testing.T) {
	proc, _ := hostProcFixture(t)
	var called []string
	sampler := &Sampler{HostProc: proc, HostRoot: "/host/root"}
	sampler.mountPoint = func(path string) bool { return path == "/host/root" }
	sampler.statfs = func(path string, fs *unix.Statfs_t) error {
		called = append(called, path)
		fs.Blocks, fs.Bfree, fs.Bavail, fs.Bsize, fs.Files, fs.Ffree = 100, 40, 30, 4096, 10, 4
		return nil
	}
	_, filesystems, _, reason, err := sampler.collectHost(time.Now())
	if err != nil || reason != "" {
		t.Fatalf("err=%v reason=%q", err, reason)
	}
	sort.Strings(called)
	want := []string{"/host/root", "/host/root/boot/efi", "/host/root/mnt/backup disk", "/host/root/tmp"}
	if !reflect.DeepEqual(called, want) {
		t.Fatalf("statfs on %q want %q", called, want)
	}
	var points []string
	for _, fs := range filesystems {
		points = append(points, fs.MountPoint)
		if fs.UsedBytes == nil || *fs.UsedBytes != 60*4096 || fs.InodesUsed == nil || *fs.InodesUsed != 6 {
			t.Fatalf("filesystem %+v", fs)
		}
	}
	if !reflect.DeepEqual(points, []string{"/", "/boot/efi", "/mnt/backup disk", "/tmp"}) {
		t.Fatalf("mount points %q", points)
	}
	if filesystems[0].MountKey != "/dev/sda1|/" || filesystems[0].FSType != "ext4" {
		t.Fatalf("root %+v", filesystems[0])
	}
}

func TestSamplerReadsHostNetworkNamespace(t *testing.T) {
	proc, write := hostProcFixture(t)
	sampler := &Sampler{HostProc: proc, HostRoot: "/"}
	sampler.statfs = func(string, *unix.Statfs_t) error { return nil }
	start := time.Now()
	if _, _, _, _, err := sampler.collectHost(start); err != nil {
		t.Fatal(err)
	}
	write("1/net/dev", "  eth0: 8812345017 9123461 3 12 0 0 0 0 1290349812 4123344 1 2 0 0 0 0\n  ens4: 1000 10 0 0 0 0 0 0 2000 20 0 0 0 0 0 0\n    lo: 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0\n")
	host, _, _, _, err := sampler.collectHost(start.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if host.NetworkRXBPS == nil || *host.NetworkRXBPS != 1000 || host.NetworkTXBPS == nil || *host.NetworkTXBPS != 1000 {
		t.Fatalf("rx=%v tx=%v", host.NetworkRXBPS, host.NetworkTXBPS)
	}
}

func TestSamplerFilesystemDegradation(t *testing.T) {
	proc, _ := hostProcFixture(t)
	engine := metrics.NewEngine(16)
	sampler := &Sampler{Engine: engine, HostProc: proc, HostRoot: "/host/root"}
	sampler.mountPoint = func(string) bool { return false }
	sampler.statfs = func(string, *unix.Statfs_t) error { t.Fatal("statfs without a host root"); return nil }
	host, filesystems, _, reason, err := sampler.collectHost(time.Now())
	if err != nil || host.MemoryUsedBytes == nil || len(filesystems) != 0 || reason != "host root not mounted; filesystem metrics unavailable" {
		t.Fatalf("err=%v filesystems=%d reason=%q", err, len(filesystems), reason)
	}
	sampler.collect(context.Background(), nil)
	if h := engine.Snapshot().Collectors["host"]; h.State != metrics.CollectorDegraded || h.Reason != reason {
		t.Fatalf("host collector %+v", h)
	}

	sampler.mountPoint = func(string) bool { return true }
	sampler.statfs = func(path string, _ *unix.Statfs_t) error {
		if strings.HasSuffix(path, "/boot/efi") {
			return unix.EACCES
		}
		return nil
	}
	_, filesystems, _, reason, _ = sampler.collectHost(time.Now())
	if len(filesystems) != 3 || reason != "1 of 4 filesystems skipped: /boot/efi: permission denied" {
		t.Fatalf("filesystems=%d reason=%q", len(filesystems), reason)
	}

	if err := os.Remove(filepath.Join(proc, "1", "mountinfo")); err != nil {
		t.Fatal(err)
	}
	_, filesystems, _, reason, err = sampler.collectHost(time.Now())
	if err != nil || len(filesystems) != 0 || !strings.HasPrefix(reason, "host mounts unavailable: ") {
		t.Fatalf("err=%v filesystems=%d reason=%q", err, len(filesystems), reason)
	}
}

func TestSamplerKeepsDataDirFilesystem(t *testing.T) {
	proc, _ := hostProcFixture(t)
	dataDir := t.TempDir()
	sampler := &Sampler{HostProc: proc, HostRoot: "/", DataDir: dataDir}
	sampler.statfs = func(path string, fs *unix.Statfs_t) error {
		if path == dataDir {
			fs.Blocks, fs.Bsize = 7, 1
			return nil
		}
		return errors.New("only the data directory is reachable")
	}
	_, filesystems, _, _, _ := sampler.collectHost(time.Now())
	found := false
	for _, fs := range filesystems {
		if fs.MountPoint == dataDir && fs.TotalBytes != nil && *fs.TotalBytes == 7 {
			found = true
		}
	}
	if !found {
		t.Fatalf("data directory filesystem missing from %+v", filesystems)
	}
}
