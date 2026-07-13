// SPDX-License-Identifier: AGPL-3.0-only
package resources

import (
	"crypto/sha256"
	"encoding/hex"
	"net/url"
	"path"
	"regexp"
	"strings"
)

type Identity struct{ StableKey, Name, Context, Source, Project, Service string }

type Metadata struct {
	Labels, Environment      map[string]string
	ContainerName, Image     string
	FallbackCategory, Manual string
}

var uuidPart = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
var uuidSuffix = regexp.MustCompile(`(?i)[-_][0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
var timestampPart = regexp.MustCompile(`^\d{10,17}$`)
var opaquePart = regexp.MustCompile(`(?i)^[0-9a-z]{20,}$`)

var categories = map[string]bool{"application": true, "service": true, "database": true, "cache": true, "worker": true, "proxy": true, "infrastructure": true, "unmanaged": true}

func ValidCategory(v string) bool { return categories[strings.ToLower(v)] }
func Resolve(labels map[string]string, fallback, manual string) Identity {
	return ResolveMetadata(Metadata{Labels: labels, ContainerName: fallback, Manual: manual})
}

func ResolveMetadata(metadata Metadata) Identity {
	labels, environment := metadata.Labels, metadata.Environment
	fallback, manual := metadata.ContainerName, metadata.Manual
	if manual != "" {
		return withDisplay(Identity{StableKey: "manual:" + safe(manual), Source: "manual"}, metadata)
	}
	if id := first(labels["coolify.resource.uuid"], labels["coolify.uuid"], environment["COOLIFY_RESOURCE_UUID"]); id != "" {
		return withDisplay(Identity{StableKey: "coolify:" + safe(id), Source: "coolify"}, metadata)
	}
	identity := Compose(labels, fallback)
	return withDisplay(identity, metadata)
}

func Compose(labels map[string]string, fallback string) Identity {
	p, s := strings.TrimSpace(labels["com.docker.compose.project"]), strings.TrimSpace(labels["com.docker.compose.service"])
	if p != "" && s != "" {
		return Identity{StableKey: "compose:" + safe(p) + ":" + safe(s), Name: s, Source: "compose", Project: p, Service: s}
	}
	return Derived(labels, fallback)
}

func withDisplay(identity Identity, metadata Metadata) Identity {
	labels := metadata.Labels
	project := strings.TrimSpace(labels["com.docker.compose.project"])
	service := strings.TrimSpace(labels["com.docker.compose.service"])
	if identity.Project == "" {
		identity.Project = project
	}
	if identity.Service == "" {
		identity.Service = service
	}
	host := CoolifyHostname(first(metadata.Environment["COOLIFY_FQDN"], metadata.Environment["COOLIFY_URL"], labels["coolify.fqdn"], labels["coolify.url"]))
	container := cleanedContainerName(metadata.ContainerName, project, service)
	image := imageName(metadata.Image)
	coolifyName := CleanGeneratedName(labels["coolify.name"])
	identity.Name = first(
		labels["binnacle.name"],
		CleanGeneratedName(service),
		readable(container),
		coolifyName,
		host,
		labels["org.opencontainers.image.title"],
		image,
		metadata.ContainerName,
	)
	projectContext := ""
	if value := CleanGeneratedName(project); value != "" {
		projectContext = value
		if value := CleanGeneratedName(labels["coolify.environment"]); value != "" {
			projectContext += "/" + value
		}
	}
	identity.Context = first(labels["binnacle.context"], host, projectContext, metadata.Image, metadata.FallbackCategory)
	return identity
}

func CoolifyHostname(value string) string {
	value = strings.TrimSpace(strings.Split(value, ",")[0])
	if value == "" {
		return ""
	}
	if !strings.Contains(value, "://") {
		value = "https://" + value
	}
	parsed, err := url.Parse(value)
	if err != nil {
		return ""
	}
	return strings.TrimPrefix(strings.ToLower(parsed.Hostname()), "*.")
}

func CleanGeneratedName(value string) string {
	value = strings.TrimSpace(value)
	for {
		if cleaned := uuidSuffix.ReplaceAllString(value, ""); cleaned != value && cleaned != "" {
			value = cleaned
			continue
		}
		index := strings.LastIndexAny(value, "-_")
		if index <= 0 || !isGeneratedPart(value[index+1:]) {
			break
		}
		value = value[:index]
	}
	return readable(value)
}

func readable(value string) string {
	value = strings.Trim(strings.TrimSpace(value), "-_")
	if value == "" || isGeneratedPart(value) {
		return ""
	}
	return value
}

func cleanedContainerName(value, project, service string) string {
	value = strings.TrimPrefix(strings.TrimSpace(value), "/")
	for _, prefix := range []string{project + "-", project + "_"} {
		if project != "" {
			value = strings.TrimPrefix(value, prefix)
		}
	}
	if service != "" && (value == service+"-1" || value == service+"_1") {
		return service
	}
	value = regexp.MustCompile(`[-_]\d+$`).ReplaceAllString(value, "")
	return CleanGeneratedName(value)
}

func isGeneratedPart(value string) bool {
	if uuidPart.MatchString(value) || timestampPart.MatchString(value) {
		return true
	}
	if !opaquePart.MatchString(value) {
		return false
	}
	return strings.IndexFunc(value, func(r rune) bool { return r >= '0' && r <= '9' }) >= 0
}

func imageName(value string) string {
	value = strings.TrimSpace(strings.Split(value, "@")[0])
	base := path.Base(value)
	if index := strings.LastIndex(base, ":"); index > 0 {
		base = base[:index]
	}
	return readable(base)
}

func first(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}
func Derived(labels map[string]string, fallback string) Identity {
	h := sha256.New()
	for _, k := range []string{"com.docker.compose.project", "com.docker.compose.service", "org.opencontainers.image.source"} {
		h.Write([]byte(labels[k]))
		h.Write([]byte{0})
	}
	h.Write([]byte(fallback))
	return Identity{StableKey: "derived:" + hex.EncodeToString(h.Sum(nil))[:20], Name: fallback, Source: "derived"}
}
func safe(v string) string {
	return strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' {
			return r
		}
		return '-'
	}, v)
}
