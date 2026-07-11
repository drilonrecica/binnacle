// SPDX-License-Identifier: AGPL-3.0-only
package coolify

import "strings"

type Metadata struct {
	UUID, Project, Environment, Name string
	Infrastructure                   bool
}

func Resolve(labels map[string]string) (Metadata, bool) {
	uuid := strings.TrimSpace(labels["coolify.resource.uuid"])
	if uuid == "" {
		uuid = strings.TrimSpace(labels["coolify.uuid"])
	}
	if uuid == "" {
		return Metadata{}, false
	}
	m := Metadata{UUID: uuid, Project: labels["coolify.project"], Environment: labels["coolify.environment"], Name: labels["coolify.name"]}
	m.Infrastructure = labels["coolify.type"] == "infrastructure"
	return m, true
}
