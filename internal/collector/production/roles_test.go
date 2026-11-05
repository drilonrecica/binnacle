// SPDX-License-Identifier: AGPL-3.0-only

package production

import (
	"testing"

	hostcollector "github.com/drilonrecica/binnacle/internal/collector/host"
	"github.com/drilonrecica/binnacle/internal/dockerapi"
	"github.com/drilonrecica/binnacle/internal/metrics"
)

func TestDockerRootFromNamedVolumes(t *testing.T) {
	tests := []struct {
		name     string
		inspects map[string]dockerapi.Inspect
		want     string
	}{
		{"none", nil, ""},
		{"bind mounts only", map[string]dockerapi.Inspect{"a": {Mounts: []dockerapi.Mount{{Type: "bind", Source: "/srv/app/data/volumes/x/_data"}}}}, ""},
		{"default root", map[string]dockerapi.Inspect{"a": {Mounts: []dockerapi.Mount{{Type: "volume", Source: "/var/lib/docker/volumes/pg/_data", Destination: "/var/lib/postgresql"}}}}, "/var/lib/docker"},
		{"custom data-root", map[string]dockerapi.Inspect{
			"a": {Mounts: []dockerapi.Mount{{Type: "bind", Source: "/etc/app"}, {Type: "volume", Source: "/mnt/fast/docker/volumes/cache/_data"}}},
			"b": {Mounts: []dockerapi.Mount{{Type: "volume", Source: "/mnt/fast/docker/volumes/db/_data/"}}},
		}, "/mnt/fast/docker"},
		{"driver volume without a local path", map[string]dockerapi.Inspect{"a": {Mounts: []dockerapi.Mount{{Type: "volume", Source: ""}, {Type: "volume", Source: "/volumes/x/_data"}}}}, ""},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := dockerRootFromInspects(test.inspects); got != test.want {
				t.Fatalf("root=%q, want %q", got, test.want)
			}
		})
	}
}

func TestFilesystemRoles(t *testing.T) {
	mounts := []hostcollector.Mount{
		{Major: 8, Minor: 1, Target: "/"},
		{Major: 8, Minor: 17, Target: "/mnt/fast"},
		{Major: 8, Minor: 33, Target: "/srv"},
		// A bind of the fast disk at the default root does not move it.
		{Major: 8, Minor: 17, Root: "/docker", Target: "/var/lib/docker"},
	}
	if got := deviceHolding(mounts, "/mnt/fast/docker"); got != "8:17" {
		t.Fatalf("custom root device=%q", got)
	}
	if got := deviceHolding(mounts, "/var/lib/docker"); got != "8:17" {
		t.Fatalf("bind-mounted root device=%q", got)
	}
	if got := deviceHolding(mounts[:1], "/var/lib/docker"); got != "8:1" {
		t.Fatalf("root on / device=%q", got)
	}
	if got := deviceHolding(mounts, "/srvx/docker"); got != "8:1" {
		t.Fatalf("prefix that is not a directory matched: %q", got)
	}
	tests := []struct {
		target, device, docker, data, want string
	}{
		{"/", "8:1", "8:1", "8:1", metrics.FilesystemRoot},
		{"/mnt/fast", "8:17", "8:17", "8:17", metrics.FilesystemDocker},
		{"/srv", "8:33", "8:17", "8:33", metrics.FilesystemData},
		{"/srv", "8:33", "8:17", "", metrics.FilesystemOther},
		{"/boot", "8:2", "", "", metrics.FilesystemOther},
	}
	for _, test := range tests {
		if got := filesystemRole(test.target, test.device, test.docker, test.data); got != test.want {
			t.Errorf("role(%s)=%q, want %q", test.target, got, test.want)
		}
	}
}
