// SPDX-License-Identifier: AGPL-3.0-only
package dockerapi

import "testing"

func TestAllowedEnvironmentOnlyRetainsCoolifyMetadata(t *testing.T) {
	got := allowedEnvironment([]string{"COOLIFY_FQDN=api.example.com", "COOLIFY_URL=https://api.example.com", "COOLIFY_RESOURCE_UUID=uuid", "DATABASE_PASSWORD=secret", "INVALID"})
	if len(got) != 3 || got["COOLIFY_FQDN"] != "api.example.com" || got["DATABASE_PASSWORD"] != "" {
		t.Fatalf("allowed environment=%v", got)
	}
}
