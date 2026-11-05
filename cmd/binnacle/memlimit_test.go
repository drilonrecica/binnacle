// SPDX-License-Identifier: AGPL-3.0-only
package main

import (
	"os"
	"path/filepath"
	"testing"
)

func writeCgroupFile(t *testing.T, root, name, value string) {
	t.Helper()
	path := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(value), 0o644); err != nil {
		t.Fatal(err)
	}
}

// RUN-4: the GC is told about the container's memory limit.
func TestMemoryLimitFromCgroup(t *testing.T) {
	const mib = int64(1) << 20
	cases := []struct {
		name  string
		setup func(root string)
		env   string
		want  int64
		ok    bool
	}{
		{"cgroup v2 limit", func(root string) { writeCgroupFile(t, root, "memory.max", "134217728\n") }, "", 128 * mib * 9 / 10, true},
		{"cgroup v2 unlimited", func(root string) { writeCgroupFile(t, root, "memory.max", "max\n") }, "", 0, false},
		{"cgroup v1 limit", func(root string) { writeCgroupFile(t, root, "memory/memory.limit_in_bytes", "268435456\n") }, "", 256 * mib * 9 / 10, true},
		{"cgroup v1 unlimited", func(root string) {
			writeCgroupFile(t, root, "memory/memory.limit_in_bytes", "9223372036854771712\n")
		}, "", 0, false},
		{"no cgroup files", func(string) {}, "", 0, false},
		{"GOMEMLIMIT wins", func(root string) { writeCgroupFile(t, root, "memory.max", "134217728\n") }, "100MiB", 0, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			tc.setup(root)
			got, ok := memoryLimit(tc.env, root)
			if ok != tc.ok || got != tc.want {
				t.Fatalf("memoryLimit = %d, %v; want %d, %v", got, ok, tc.want, tc.ok)
			}
		})
	}
}
