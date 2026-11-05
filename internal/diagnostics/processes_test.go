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
