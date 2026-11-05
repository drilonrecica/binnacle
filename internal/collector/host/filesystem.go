// SPDX-License-Identifier: AGPL-3.0-only
package host

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// Mount is one line of /proc/<pid>/mountinfo (proc(5)).
type Mount struct {
	Major, Minor uint32
	// Root is the path inside the filesystem that is mounted at Target; a bind
	// mount of a subdirectory has a Root other than "/".
	Root, Target, FSType, Source string
}

// Device is the maj:min key used to deduplicate mounts of one filesystem.
func (m Mount) Device() string { return fmt.Sprintf("%d:%d", m.Major, m.Minor) }

// ParseMountinfo parses the mountinfo format: six or more fields, optional
// fields ended by a lone "-", then filesystem type, source and super options.
// Paths are unescaped from the kernel's octal form (\040 for a space).
func ParseMountinfo(input string) ([]Mount, error) {
	out := []Mount{}
	for n, line := range strings.Split(input, "\n") {
		f := strings.Fields(line)
		if len(f) == 0 {
			continue
		}
		sep := -1
		for i := 6; i < len(f); i++ {
			if f[i] == "-" {
				sep = i
				break
			}
		}
		if len(f) < 6 || sep < 0 || len(f) < sep+3 {
			return nil, fmt.Errorf("mountinfo line %d: malformed", n+1)
		}
		majStr, minStr, ok := strings.Cut(f[2], ":")
		if !ok {
			return nil, fmt.Errorf("mountinfo line %d: malformed device %q", n+1, f[2])
		}
		major, err := strconv.ParseUint(majStr, 10, 32)
		if err != nil {
			return nil, fmt.Errorf("mountinfo line %d: %w", n+1, err)
		}
		minor, err := strconv.ParseUint(minStr, 10, 32)
		if err != nil {
			return nil, fmt.Errorf("mountinfo line %d: %w", n+1, err)
		}
		out = append(out, Mount{
			Major: uint32(major), Minor: uint32(minor),
			Root: unescapeMountPath(f[3]), Target: unescapeMountPath(f[4]),
			FSType: f[sep+1], Source: unescapeMountPath(f[sep+2]),
		})
	}
	return out, nil
}

// unescapeMountPath decodes the \NNN octal escapes the kernel writes for
// space, tab, newline and backslash in mount paths.
func unescapeMountPath(s string) string {
	if !strings.Contains(s, `\`) {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+3 < len(s) {
			if v, err := strconv.ParseUint(s[i+1:i+4], 8, 8); err == nil {
				b.WriteByte(byte(v))
				i += 3
				continue
			}
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

// pseudoTypes are kernel-backed or layered filesystems that do not represent
// host disk capacity.
var pseudoTypes = map[string]bool{
	"proc": true, "sysfs": true, "cgroup": true, "cgroup2": true, "devtmpfs": true, "devpts": true,
	"mqueue": true, "debugfs": true, "tracefs": true, "securityfs": true, "pstore": true, "bpf": true,
	"configfs": true, "fusectl": true, "hugetlbfs": true, "autofs": true, "binfmt_misc": true,
	"efivarfs": true, "nsfs": true, "overlay": true, "squashfs": true, "rpc_pipefs": true, "nfsd": true,
	"ramfs": true, "selinuxfs": true, "aufs": true, "fuse.lxcfs": true, "fuse.gvfsd-fuse": true, "fuse.portal": true,
}

// IsPseudo reports whether a mount is dropped from filesystem metrics by type:
// kernel filesystems, overlays, snaps, and tmpfs under /run, /dev, /sys and /proc.
func IsPseudo(m Mount) bool {
	if pseudoTypes[m.FSType] {
		return true
	}
	if m.FSType == "tmpfs" {
		for _, prefix := range []string{"/run", "/dev", "/sys", "/proc"} {
			if m.Target == prefix || strings.HasPrefix(m.Target, prefix+"/") {
				return true
			}
		}
	}
	return false
}

// SelectFilesystems drops pseudo filesystems and keeps one mount per maj:min,
// the one with the shortest target. The filesystem whose device is keep is
// retained even when its type would be dropped.
func SelectFilesystems(mounts []Mount, keep string) []Mount {
	best := map[string]Mount{}
	for _, m := range mounts {
		dev := m.Device()
		if dev != keep && IsPseudo(m) {
			continue
		}
		current, ok := best[dev]
		if !ok || len(m.Target) < len(current.Target) || (len(m.Target) == len(current.Target) && m.Target < current.Target) {
			best[dev] = m
		}
	}
	out := make([]Mount, 0, len(best))
	for _, m := range best {
		out = append(out, m)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Target < out[j].Target })
	return out
}
