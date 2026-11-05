// SPDX-License-Identifier: AGPL-3.0-only
package production

import (
	"context"
	"encoding/json"
	"math"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/drilonrecica/binnacle/internal/dockerapi"
	"github.com/drilonrecica/binnacle/internal/metrics"
	"golang.org/x/sys/unix"
)

// validationDir is a fixture recorded with scripts/validate-host.sh --record:
// two /proc snapshots of a real host, its /sys/block list, statfs of every
// mount and df, plus Docker Engine API responses. reference.json and
// docker/reference.json hold the values the reference tools report for the
// same inputs, computed by the script independently of the collectors.
// docs/operations/metric-validation.md describes how to record a new one.
const validationDir = "testdata/validation"

type hostReference struct {
	Elapsed       float64 `json:"elapsed"`
	CPUBusyPct    float64 `json:"cpu_busy_pct"`
	MemTotalBytes int64   `json:"mem_total_bytes"`
	MemUsedBytes  int64   `json:"mem_used_bytes"`
	Load1         float64 `json:"load1"`
	NetRXBPS      float64 `json:"net_rx_bps"`
	NetTXBPS      float64 `json:"net_tx_bps"`
	DiskReadBPS   float64 `json:"disk_read_bps"`
	DiskWriteBPS  float64 `json:"disk_write_bps"`
	DiskReadIOPS  float64 `json:"disk_read_iops"`
	DiskWriteIOPS float64 `json:"disk_write_iops"`
}

type dockerReference struct {
	CPUPercent    *float64 `json:"cpu_percent"`
	OnlineCPUs    int      `json:"online_cpus"`
	MemUsageBytes int64    `json:"mem_usage_bytes"`
	RXBPS         *float64 `json:"rx_bps"`
	TXBPS         *float64 `json:"tx_bps"`
	BlockReadBPS  float64  `json:"block_read_bps"`
	BlockWriteBPS float64  `json:"block_write_bps"`
}

func readJSON(t *testing.T, path string, value any) {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, value); err != nil {
		t.Fatalf("%s: %v", path, err)
	}
}

// copyTree copies the regular files under from into to.
func copyTree(t *testing.T, from, to string) {
	t.Helper()
	err := filepath.WalkDir(from, func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		rel, err := filepath.Rel(from, path)
		if err != nil {
			return err
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		target := filepath.Join(to, rel)
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		return os.WriteFile(target, raw, 0o600)
	})
	if err != nil {
		t.Fatal(err)
	}
}

// fixtureSys rebuilds <sys>/block from sys-block.txt ("name" or
// "name partition" per line).
func fixtureSys(t *testing.T, list string) string {
	t.Helper()
	raw, err := os.ReadFile(list)
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	for _, line := range strings.Split(string(raw), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		dir := filepath.Join(root, "block", fields[0])
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if len(fields) > 1 && fields[1] == "partition" {
			if err := os.WriteFile(filepath.Join(dir, "partition"), []byte("1\n"), 0o600); err != nil {
				t.Fatal(err)
			}
		}
	}
	return root
}

// fixtureStatfs reads statfs.tsv into Statfs_t values by mount target.
func fixtureStatfs(t *testing.T, path string) map[string]unix.Statfs_t {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]unix.Statfs_t{}
	for _, line := range strings.Split(string(raw), "\n") {
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		f := strings.Split(line, "\t")
		if len(f) != 8 {
			t.Fatalf("statfs.tsv: malformed line %q", line)
		}
		n := make([]uint64, 7)
		for i := range n {
			v, err := strconv.ParseUint(f[i+1], 10, 64)
			if err != nil {
				t.Fatalf("statfs.tsv %q: %v", line, err)
			}
			n[i] = v
		}
		var st unix.Statfs_t
		st.Bsize, st.Frsize = int64(n[0]), int64(n[1])
		st.Blocks, st.Bfree, st.Bavail, st.Files, st.Ffree = n[2], n[3], n[4], n[5], n[6]
		out[f[0]] = st
	}
	return out
}

type dfRow struct{ size, used, avail int64 }

// fixtureDF reads `df -B1 --output=size,used,avail,itotal,iused,fstype,target`.
func fixtureDF(t *testing.T, path string) map[string]dfRow {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]dfRow{}
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	for _, line := range lines[1:] {
		f := strings.Fields(line)
		if len(f) < 7 {
			t.Fatalf("df.txt: malformed line %q", line)
		}
		var n [3]int64
		for i := range n {
			v, err := strconv.ParseInt(f[i], 10, 64)
			if err != nil {
				t.Fatalf("df.txt %q: %v", line, err)
			}
			n[i] = v
		}
		out[strings.Join(f[6:], " ")] = dfRow{n[0], n[1], n[2]}
	}
	return out
}

// show prints an optional value for a failure message.
func show[T any](v *T) any {
	if v == nil {
		return "unknown"
	}
	return *v
}

func readElapsed(t *testing.T, path string) time.Duration {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	seconds, err := strconv.ParseFloat(strings.TrimSpace(string(raw)), 64)
	if err != nil {
		t.Fatal(err)
	}
	return time.Duration(seconds * float64(time.Second))
}

// near compares a collector value with a reference value that was computed
// from the same counters, so only float rounding may separate them.
func near(t *testing.T, name string, got *float64, want float64) {
	t.Helper()
	if got == nil {
		t.Errorf("%s = unknown, reference %v", name, want)
		return
	}
	if math.Abs(*got-want) > 1e-6*math.Max(1, math.Abs(want)) {
		t.Errorf("%s = %v, reference %v", name, *got, want)
	}
}

func TestValidationHostFixtureMatchesReferenceTools(t *testing.T) {
	var ref hostReference
	readJSON(t, filepath.Join(validationDir, "reference.json"), &ref)
	elapsed := readElapsed(t, filepath.Join(validationDir, "elapsed"))
	statfs := fixtureStatfs(t, filepath.Join(validationDir, "statfs.tsv"))
	df := fixtureDF(t, filepath.Join(validationDir, "df.txt"))

	proc := t.TempDir()
	copyTree(t, filepath.Join(validationDir, "proc", "t0"), proc)
	sampler := &Sampler{HostProc: proc, HostSys: fixtureSys(t, filepath.Join(validationDir, "sys-block.txt")), HostRoot: "/"}
	sampler.mountPoint = func(string) bool { return true }
	sampler.statfs = func(path string, st *unix.Statfs_t) error {
		v, ok := statfs[path]
		if !ok {
			return unix.EACCES
		}
		*st = v
		return nil
	}

	start := time.Now()
	if _, _, _, _, err := sampler.collectHost(start); err != nil {
		t.Fatal(err)
	}
	copyTree(t, filepath.Join(validationDir, "proc", "t1"), proc)
	host, filesystems, _, reason, err := sampler.collectHost(start.Add(elapsed))
	if err != nil {
		t.Fatal(err)
	}
	if reason != "" {
		t.Fatalf("host collector reason %q, want a complete observation", reason)
	}

	// top: 100 - id - wa; free: total - available; /proc/loadavg.
	near(t, "cpu busy %", host.CPUPercent, ref.CPUBusyPct)
	if host.MemoryTotalBytes == nil || *host.MemoryTotalBytes != ref.MemTotalBytes {
		t.Errorf("memory total %v, reference %d", show(host.MemoryTotalBytes), ref.MemTotalBytes)
	}
	if host.MemoryUsedBytes == nil || *host.MemoryUsedBytes != ref.MemUsedBytes {
		t.Errorf("memory used %v, reference %d", show(host.MemoryUsedBytes), ref.MemUsedBytes)
	}
	near(t, "load 1m", host.Load1, ref.Load1)
	// /proc/1/net/dev without lo and the Docker bridge and veth interfaces.
	near(t, "network rx B/s", host.NetworkRXBPS, ref.NetRXBPS)
	near(t, "network tx B/s", host.NetworkTXBPS, ref.NetTXBPS)
	// /proc/diskstats over the whole devices of /sys/block: the fixture has
	// partitions and zram0, which must not be added again (COL-3).
	near(t, "disk read B/s", host.DiskReadBPS, ref.DiskReadBPS)
	near(t, "disk write B/s", host.DiskWriteBPS, ref.DiskWriteBPS)
	near(t, "disk read IOPS", host.DiskReadIOPS, ref.DiskReadIOPS)
	near(t, "disk write IOPS", host.DiskWriteIOPS, ref.DiskWriteIOPS)

	// The recorded host has btrfs / and /home on one device, CIFS shares
	// behind autofs mounts of the same targets, tmpfs under /dev and /run,
	// and a container overlay: one filesystem per device, no pseudo ones.
	var targets []string
	for _, fs := range filesystems {
		targets = append(targets, fs.MountPoint)
		row, ok := df[fs.MountPoint]
		if !ok {
			t.Errorf("filesystem %s is not in df", fs.MountPoint)
			continue
		}
		// df and statfs were read moments apart; sizes are exact, usage may
		// have moved by a few blocks on a busy filesystem.
		if fs.TotalBytes == nil || *fs.TotalBytes != row.size {
			t.Errorf("%s size %v, df %d", fs.MountPoint, show(fs.TotalBytes), row.size)
		}
		for name, pair := range map[string][2]any{"used": {fs.UsedBytes, row.used}, "avail": {fs.AvailableBytes, row.avail}} {
			got, want := pair[0].(*int64), pair[1].(int64)
			if got == nil || math.Abs(float64(*got-want)) > 1<<20 {
				t.Errorf("%s %s %v, df %d", fs.MountPoint, name, show(got), want)
			}
		}
	}
	want := []string{"/", "/boot", "/mnt/anime", "/mnt/media4tb", "/mnt/movies", "/mnt/sidedisk", "/mnt/tvshows", "/tmp"}
	if !slices.Equal(targets, want) {
		t.Errorf("filesystems %v, want %v", targets, want)
	}
}

// serveDockerFixture answers the Engine API read calls from the fixture on a
// Unix socket: the container list, inspect, and the stats read of the phase
// set with the returned function. It records every stats request.
func serveDockerFixture(t *testing.T, dir string) (socket string, setPhase func(string), statsCalls func() []string) {
	t.Helper()
	// Unix socket paths are limited to about 108 bytes; t.TempDir is too long.
	base, err := os.MkdirTemp("", "bnv")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(base) })
	socket = filepath.Join(base, "docker.sock")
	listener, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	phase := "t0"
	var calls []string
	version := regexp.MustCompile(`^/v[0-9.]+`)
	route := regexp.MustCompile(`^/containers/([0-9a-f]{64})/(json|stats)$`)
	serveFile := func(w http.ResponseWriter, path string) {
		raw, err := os.ReadFile(path)
		if err != nil {
			http.Error(w, `{"message":"no such container"}`, http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(raw)
	}
	server := &http.Server{ReadHeaderTimeout: 5 * time.Second, Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := version.ReplaceAllString(r.URL.Path, "")
		if path == "/containers/json" {
			serveFile(w, filepath.Join(dir, "containers.json"))
			return
		}
		m := route.FindStringSubmatch(path)
		if m == nil {
			http.Error(w, `{"message":"not in fixture"}`, http.StatusNotFound)
			return
		}
		if m[2] == "json" {
			serveFile(w, filepath.Join(dir, "inspect", m[1]+".json"))
			return
		}
		mu.Lock()
		current := phase
		calls = append(calls, m[1])
		mu.Unlock()
		serveFile(w, filepath.Join(dir, "stats-"+current, m[1]+".json"))
	})}
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(func() { server.Close() })
	return socket,
		func(p string) { mu.Lock(); phase = p; mu.Unlock() },
		func() []string { mu.Lock(); defer mu.Unlock(); return slices.Clone(calls) }
}

func TestValidationDockerFixtureMatchesDockerStats(t *testing.T) {
	dir := filepath.Join(validationDir, "docker")
	var ref map[string]dockerReference
	readJSON(t, filepath.Join(dir, "reference.json"), &ref)
	elapsed := readElapsed(t, filepath.Join(dir, "elapsed"))
	var listing []struct{ Id, State string }
	readJSON(t, filepath.Join(dir, "containers.json"), &listing)

	socket, setPhase, statsCalls := serveDockerFixture(t, dir)
	engine, err := dockerapi.NewEngine(socket)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { engine.Close() })
	sampler := &Sampler{Docker: engine, MaxDockerConcurrency: 2}
	hostTotal := int64(33604382720)
	ctx := context.Background()

	tick := time.Now()
	if _, reason, err := sampler.collectDocker(ctx, tick, &hostTotal); err != nil || reason != "" {
		t.Fatalf("first tick: reason=%q err=%v", reason, err)
	}
	setPhase("t1")
	resources, reason, err := sampler.collectDocker(ctx, tick.Add(elapsed), &hostTotal)
	if err != nil || reason != "" {
		t.Fatalf("second tick: reason=%q err=%v", reason, err)
	}
	components := map[string]metrics.ResourceComponent{}
	for _, r := range resources {
		for _, c := range r.Components {
			components[string(c.ID)] = c
		}
	}

	for id, want := range ref {
		c, ok := components[id]
		if !ok {
			t.Errorf("container %s missing from the resources", id[:12])
			continue
		}
		name := c.Name
		// docker stats CPU % is the share of one CPU; Binnacle's host percent
		// is the share of the whole host, so it is CPU % / online CPUs.
		if want.CPUPercent != nil {
			near(t, name+" cpu host %", c.CPUHostPercent, *want.CPUPercent/float64(want.OnlineCPUs))
		}
		if c.MemoryBytes == nil || *c.MemoryBytes != want.MemUsageBytes {
			t.Errorf("%s memory %v, docker stats MEM USAGE %d", name, show(c.MemoryBytes), want.MemUsageBytes)
		}
		for label, pair := range map[string][2]*float64{"rx": {c.RXBPS, want.RXBPS}, "tx": {c.TXBPS, want.TXBPS}} {
			got, reference := pair[0], pair[1]
			if reference == nil {
				if got != nil {
					t.Errorf("%s net %s = %v, want unknown for a container without networks", name, label, *got)
				}
				continue
			}
			near(t, name+" net "+label, got, *reference)
		}
		near(t, name+" block read", c.BlockReadBPS, want.BlockReadBPS)
		near(t, name+" block write", c.BlockWriteBPS, want.BlockWriteBPS)
	}

	// Exited containers are listed with their state and never asked for stats.
	for _, entry := range listing {
		if entry.State == "running" {
			continue
		}
		c, ok := components[entry.Id]
		if !ok || c.Status != metrics.StatusDown || c.CPUHostPercent != nil || c.MemoryBytes != nil {
			t.Errorf("exited container %s: %+v, want down without metrics", entry.Id[:12], c)
		}
		if slices.Contains(statsCalls(), entry.Id) {
			t.Errorf("exited container %s was asked for stats", entry.Id[:12])
		}
	}
}
