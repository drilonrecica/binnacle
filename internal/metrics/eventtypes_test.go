// SPDX-License-Identifier: AGPL-3.0-only
package metrics

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// webEventTypesPath is the generated list the web UI imports.
var webEventTypesPath = filepath.Join("..", "..", "web", "src", "lib", "event-types.generated.ts")

// renderWebEventTypes renders the catalogue as the TypeScript module the web
// UI imports, already in the repository's Prettier style.
func renderWebEventTypes() string {
	var b strings.Builder
	b.WriteString("// Generated from internal/metrics/eventtypes.go; do not edit by hand.\n")
	b.WriteString("// Regenerate with: BINNACLE_UPDATE_GENERATED=1 go test ./internal/metrics -run TestWebEventTypesAreCurrent\n\n")
	b.WriteString("/** Every event type the backend writes to the events table. */\n")
	b.WriteString("export const eventTypes = [\n")
	for _, info := range EventTypes {
		b.WriteString("  '" + info.Type + "',\n")
	}
	b.WriteString("] as const;\n\n")
	b.WriteString("export type EventType = (typeof eventTypes)[number];\n\n")
	b.WriteString("/** The event types drawn as markers on metric charts. */\n")
	b.WriteString("export const annotatedEventTypes: readonly EventType[] = [\n")
	for _, info := range EventTypes {
		if info.Annotate {
			b.WriteString("  '" + info.Type + "',\n")
		}
	}
	b.WriteString("];\n")
	return b.String()
}

// TestWebEventTypesAreCurrent keeps the web UI's event list generated from
// the catalogue, so chart markers listen for the types the backend really
// writes (UI-11).
func TestWebEventTypesAreCurrent(t *testing.T) {
	want := renderWebEventTypes()
	if os.Getenv("BINNACLE_UPDATE_GENERATED") == "1" {
		if err := os.WriteFile(webEventTypesPath, []byte(want), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	got, err := os.ReadFile(webEventTypesPath)
	if err != nil {
		t.Fatalf("read %s: %v", webEventTypesPath, err)
	}
	if string(got) != want {
		t.Fatalf("%s is out of date with EventTypes; regenerate it with BINNACLE_UPDATE_GENERATED=1 go test ./internal/metrics -run TestWebEventTypesAreCurrent", webEventTypesPath)
	}
}

func TestEventTypesAreUnique(t *testing.T) {
	seen := map[string]bool{}
	for _, info := range EventTypes {
		if info.Type == "" || seen[info.Type] {
			t.Fatalf("event type %q is empty or listed twice", info.Type)
		}
		seen[info.Type] = true
	}
}
