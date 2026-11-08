// SPDX-License-Identifier: AGPL-3.0-only
package production

import (
	"os"
	"path/filepath"
	"strings"

	hostcollector "github.com/drilonrecica/binnacle/internal/collector/host"
	"github.com/drilonrecica/binnacle/internal/metrics"
)

// hostOSReleaseMount is where the packaged Compose files mount the host's
// /etc/os-release; the container's own file describes the Binnacle image.
const hostOSReleaseMount = "/host/etc/os-release"

// readIdentity describes the host from read-only files: the hostname from the
// host's /etc/hostname (procfs would report the container's own UTS name),
// the OS from os-release, the kernel and CPU from the host's procfs. Fields
// that cannot be read stay empty; nothing here fails the collector.
func (s *Sampler) readIdentity() metrics.HostIdentity {
	hostRoot := s.HostRoot
	if hostRoot == "" {
		hostRoot = "/"
	}
	read := func(path string) string {
		raw, err := os.ReadFile(path)
		if err != nil {
			return ""
		}
		return string(raw)
	}
	var identity metrics.HostIdentity
	identity.Hostname = strings.TrimSpace(read(filepath.Join(hostRoot, "etc", "hostname")))
	for _, path := range []string{hostOSReleaseMount, filepath.Join(hostRoot, "etc", "os-release"), filepath.Join(hostRoot, "usr", "lib", "os-release")} {
		if raw := read(path); raw != "" {
			identity.OS = hostcollector.ParseOSRelease(raw)
			break
		}
	}
	identity.Kernel = strings.TrimSpace(read(filepath.Join(s.HostProc, "sys", "kernel", "osrelease")))
	identity.CPUModel, identity.CPUCores = hostcollector.ParseCPUInfo(read(filepath.Join(s.HostProc, "cpuinfo")))
	return identity
}
