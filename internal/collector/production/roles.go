// SPDX-License-Identifier: AGPL-3.0-only

package production

import (
	"path"
	"strings"

	hostcollector "github.com/drilonrecica/binnacle/internal/collector/host"
	"github.com/drilonrecica/binnacle/internal/dockerapi"
	"github.com/drilonrecica/binnacle/internal/metrics"
)

// defaultDockerRoot is the daemon's default data-root, assumed until a named
// volume shows where the data root is.
const defaultDockerRoot = "/var/lib/docker"

// dockerRootFromInspects infers the Docker data root from the containers'
// named volumes, whose host source is <data-root>/volumes/<name>/_data. The
// daemon reports the root itself only through GET /info, which the socket
// proxy denies (ADR 022). Several candidates (which a single daemon does
// not produce) resolve to the smallest, so the result is deterministic. It
// returns "" when no container has a named local volume.
func dockerRootFromInspects(inspects map[string]dockerapi.Inspect) string {
	root := ""
	for _, inspect := range inspects {
		for _, m := range inspect.Mounts {
			if m.Type != "volume" || !path.IsAbs(m.Source) {
				continue
			}
			source := path.Clean(m.Source)
			if path.Base(source) != "_data" {
				continue
			}
			volumes := path.Dir(path.Dir(source))
			candidate := path.Dir(volumes)
			if path.Base(volumes) != "volumes" || candidate == "/" {
				continue
			}
			if root == "" || candidate < root {
				root = candidate
			}
		}
	}
	return root
}

// deviceHolding returns the maj:min of the mount whose target is the longest
// directory prefix of p, "" when none is.
func deviceHolding(mounts []hostcollector.Mount, p string) string {
	best, device := -1, ""
	for _, m := range mounts {
		if contains(m.Target, p) && len(m.Target) > best {
			best, device = len(m.Target), m.Device()
		}
	}
	return device
}

// contains reports whether directory dir is p or one of its ancestors.
func contains(dir, p string) bool {
	dir, p = path.Clean(dir), path.Clean(p)
	return dir == "/" || p == dir || strings.HasPrefix(p, dir+"/")
}

// filesystemRole classifies one measured mount; root wins over docker, docker
// over data, as the Headroom page orders its instruments.
func filesystemRole(target, device, dockerDevice, dataDevice string) string {
	switch {
	case path.Clean(target) == "/":
		return metrics.FilesystemRoot
	case dockerDevice != "" && device == dockerDevice:
		return metrics.FilesystemDocker
	case dataDevice != "" && device == dataDevice:
		return metrics.FilesystemData
	}
	return metrics.FilesystemOther
}
