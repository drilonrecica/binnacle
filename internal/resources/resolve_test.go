// SPDX-License-Identifier: AGPL-3.0-only
package resources

import "testing"

func TestResolveMetadataNamingAndContextPrecedence(t *testing.T) {
	tests := []struct {
		name, wantName, wantContext string
		metadata                    Metadata
	}{
		{"explicit labels", "Public API", "production", Metadata{Labels: map[string]string{"binnacle.name": "Public API", "binnacle.context": "production", "com.docker.compose.service": "api"}, ContainerName: "raw"}},
		{"compose service", "api", "project/prod", Metadata{Labels: map[string]string{"com.docker.compose.project": "project", "com.docker.compose.service": "api", "coolify.environment": "prod"}, ContainerName: "project-api-1"}},
		{"container", "worker", "registry.example/team/worker:stable", Metadata{ContainerName: "worker", Image: "registry.example/team/worker:stable"}},
		{"fqdn before image", "api.example.com", "api.example.com", Metadata{ContainerName: "f4c92c5d99cb4c4a9a001234", Image: "registry.example/team/api:stable", Environment: map[string]string{"COOLIFY_FQDN": "https://api.example.com"}}},
		{"image fallback", "postgres", "docker.io/library/postgres:16", Metadata{ContainerName: "f4c92c5d99cb4c4a9a001234", Image: "docker.io/library/postgres:16"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := ResolveMetadata(test.metadata)
			if got.Name != test.wantName || got.Context != test.wantContext {
				t.Fatalf("got name=%q context=%q", got.Name, got.Context)
			}
		})
	}
}

func TestResolveMetadataKeepsStableKeyIndependentFromDisplayName(t *testing.T) {
	base := Metadata{Labels: map[string]string{"coolify.resource.uuid": "resource-uuid", "binnacle.name": "First"}, ContainerName: "generated"}
	first := ResolveMetadata(base)
	base.Labels["binnacle.name"] = "Renamed"
	second := ResolveMetadata(base)
	if first.StableKey != second.StableKey || second.Name != "Renamed" {
		t.Fatalf("first=%+v second=%+v", first, second)
	}
}

func TestGeneratedNamesAndFQDN(t *testing.T) {
	if got := CleanGeneratedName("api-01jz7c2z5m8w9x3v6n4p2q1r0s"); got != "api" {
		t.Fatalf("cleaned name=%q", got)
	}
	if got := CleanGeneratedName("api-550e8400-e29b-41d4-a716-446655440000"); got != "api" {
		t.Fatalf("cleaned UUID name=%q", got)
	}
	if got := CleanGeneratedName("api-somethingverylonghumanreadable"); got != "api-somethingverylonghumanreadable" {
		t.Fatalf("human name was stripped: %q", got)
	}
	if got := CoolifyHostname("https://*.Example.COM/path,https://other.example"); got != "example.com" {
		t.Fatalf("hostname=%q", got)
	}
}
