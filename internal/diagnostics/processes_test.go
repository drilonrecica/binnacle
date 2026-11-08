// SPDX-License-Identifier: AGPL-3.0-only
package diagnostics

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestReadProcessParsesStatStatusAndCgroup(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "42")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	stat := "42 (worker thread) S 1 1 1 0 0 0 0 0 0 0 20 10 0 0 0 0 1 0 500 0 25"
	files := map[string]string{
		"stat": stat, "cmdline": "worker\x00--serve\x00", "status": "Name:\tworker\nUid:\t1001\t1001\t1001\t1001\n",
		"cgroup": "0::/docker/" + strings.Repeat("a", 64) + "\n",
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	got, err := readProcess(root, 42)
	if err != nil {
		t.Fatal(err)
	}
	if got.Command != "worker --serve" || got.UID != 1001 || got.Ticks != 30 || got.StartTicks != 500 || got.ContainerID != strings.Repeat("a", 64) {
		t.Fatalf("process=%+v", got)
	}
}

func TestProcessHelpersHandleMalformedAndUsers(t *testing.T) {
	path := filepath.Join(t.TempDir(), "passwd")
	if err := os.WriteFile(path, []byte("root:x:0:0::/root:/bin/sh\ninvalid\nalice:x:1001:1001::/home/alice:/bin/sh\n"), 0600); err != nil {
		t.Fatal(err)
	}
	users := readPasswd(path)
	if users[0] != "root" || users[1001] != "alice" {
		t.Fatalf("users=%v", users)
	}
	if containerFromCgroup("0::/user.slice") != "" {
		t.Fatal("associated host process with a container")
	}
}

func TestProcessScanRedactsCommandLines(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{
		"stat":       "cpu 100 0 100 1000 0 0 0 0 0 0\n",
		"uptime":     "500.00 400.00\n",
		"42/stat":    "42 (mysql) S 1 1 1 0 0 0 0 0 0 0 20 10 0 0 0 0 1 0 500 0 25",
		"42/cmdline": "mysql\x00-phunter2\x00--host=db\x00",
		"42/status":  "Name:\tmysql\nUid:\t1001\t1001\t1001\t1001\n",
		"43/stat":    "43 (app) S 1 1 1 0 0 0 0 0 0 0 20 10 0 0 0 0 1 0 500 0 25",
		"43/cmdline": "app\x00--database-url=postgres://app:correct-horse@db/app\x00",
		"43/status":  "Name:\tapp\nUid:\t1001\t1001\t1001\t1001\n",
	}
	for name, body := range files {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	scanner := NewProcessScanner(root, filepath.Join(root, "passwd"))
	scanner.SampleInterval = time.Millisecond
	service, err := NewLogService(fakeLogs{}, 5000, 1<<20, nil)
	if err != nil {
		t.Fatal(err)
	}
	scanner.Redact = service.Redact
	values, err := scanner.Scan(context.Background(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(values) != 2 {
		t.Fatalf("processes=%+v", values)
	}
	commands := map[int]string{}
	for _, value := range values {
		commands[value.PID] = value.Command
	}
	if commands[42] != "mysql -p[REDACTED] --host=db" {
		t.Fatalf("mysql command=%q", commands[42])
	}
	if commands[43] != "app --database-url=postgres://app:[REDACTED]@db/app" {
		t.Fatalf("app command=%q", commands[43])
	}
}

func writeProcFixture(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for name, body := range files {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func TestProcessScanReportsDetailAndTaskCounts(t *testing.T) {
	// stat fields after the command: state ... nice(16) threads(17) ... start(19) vsize(20) rss(21)
	root := writeProcFixture(t, map[string]string{
		"stat":      "cpu 100 0 100 1000 0 0 0 0 0 0\n",
		"uptime":    "500.00 400.00\n",
		"meminfo":   "MemTotal: 1000 kB\n",
		"42/stat":   "42 (web) R 1 1 1 0 0 0 0 0 0 0 150 50 0 0 0 -5 7 0 500 4096000 25",
		"42/status": "Uid:\t0\t0\t0\t0\n",
		"43/stat":   "43 (idle) S 1 1 1 0 0 0 0 0 0 0 0 0 0 0 0 0 1 0 500 0 1",
		"44/stat":   "44 (kworker) I 2 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 1 0 500 0 0",
		"45/stat":   "45 (zombie) Z 1 1 1 0 0 0 0 0 0 0 0 0 0 0 0 0 1 0 500 0 0",
	})
	scanner := NewProcessScanner(root, filepath.Join(root, "passwd"))
	scanner.SampleInterval = time.Millisecond
	result, err := scanner.ScanHost(context.Background(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if result.Tasks != (TaskCounts{Total: 4, Threads: 10, Running: 1, Sleeping: 2, Other: 1}) {
		t.Fatalf("tasks %+v", result.Tasks)
	}
	var web Process
	for _, value := range result.Processes {
		if value.PID == 42 {
			web = value
		}
	}
	if web.Threads != 7 || web.Nice != -5 || web.VirtBytes != 4096000 || web.CPUTimeSeconds != 2 {
		t.Fatalf("web %+v", web)
	}
	wantMem := float64(25*os.Getpagesize()) * 100 / (1000 * 1024)
	if web.MemPercent == nil || *web.MemPercent != wantMem {
		t.Fatalf("mem%% %v, want %v", web.MemPercent, wantMem)
	}
}

func TestProcessScanReusesARecentResult(t *testing.T) {
	root := writeProcFixture(t, map[string]string{
		"stat":    "cpu 100 0 100 1000 0 0 0 0 0 0\n",
		"42/stat": "42 (web) R 1 1 1 0 0 0 0 0 0 0 150 50 0 0 0 0 1 0 500 0 25",
		"43/stat": "43 (db) S 1 1 1 0 0 0 0 0 0 0 10 5 0 0 0 0 1 0 500 0 25",
	})
	now := time.Unix(1_000, 0)
	scanner := NewProcessScanner(root, filepath.Join(root, "passwd"))
	scanner.SampleInterval = time.Millisecond
	scanner.now = func() time.Time { return now }
	first, err := scanner.ScanHost(context.Background(), 10)
	if err != nil || len(first.Processes) != 2 {
		t.Fatalf("%+v %v", first, err)
	}
	// A new process appears; within ReuseFor the previous scan still answers,
	// cut to the smaller limit asked for.
	if err := os.MkdirAll(filepath.Join(root, "44"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "44", "stat"), []byte("44 (new) R 1 1 1 0 0 0 0 0 0 0 0 0 0 0 0 0 1 0 500 0 1"), 0600); err != nil {
		t.Fatal(err)
	}
	now = now.Add(3 * time.Second)
	reused, err := scanner.ScanHost(context.Background(), 1)
	if err != nil || len(reused.Processes) != 1 || !reused.SampledAt.Equal(first.SampledAt) || reused.Tasks.Total != 2 {
		t.Fatalf("reused %+v %v", reused, err)
	}
	now = now.Add(2 * time.Second)
	fresh, err := scanner.ScanHost(context.Background(), 10)
	if err != nil || fresh.Tasks.Total != 3 || !fresh.SampledAt.After(first.SampledAt) {
		t.Fatalf("fresh %+v %v", fresh, err)
	}
}
