// SPDX-License-Identifier: AGPL-3.0-only
package host

import "strings"

type Mount struct{ Source, Target, FSType string }

func ParseMounts(input, dataPath string) []Mount {
	out := []Mount{}
	for _, line := range strings.Split(input, "\n") {
		f := strings.Fields(line)
		if len(f) < 3 {
			continue
		}
		m := Mount{f[0], f[1], f[2]}
		pseudo := strings.HasPrefix(m.FSType, "proc") || strings.HasPrefix(m.FSType, "sysfs") || strings.HasPrefix(m.FSType, "cgroup") || m.FSType == "tmpfs" || m.FSType == "overlay"
		if m.Target == "/" || m.Target == dataPath || !pseudo {
			out = append(out, m)
		}
	}
	return out
}
