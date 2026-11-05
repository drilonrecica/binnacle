// SPDX-License-Identifier: AGPL-3.0-only
package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/drilonrecica/binnacle/internal/metrics"
)

// P3-12 contract (SPEC §P3-12): /api/v1/server carries the host's memoryTrend
// and every filesystem with its role and trend, in the field names P4 builds
// against.
func TestServerEndpointCarriesTrends(t *testing.T) {
	engine := metrics.NewEngine(4)
	total, used, available := int64(100<<30), int64(60<<30), int64(40<<30)
	engine.PublishFilesystems([]metrics.FilesystemObservation{
		{MountKey: "root", MountPoint: "/", Role: metrics.FilesystemRoot, TotalBytes: &total, UsedBytes: &used, AvailableBytes: &available},
		{MountKey: "docker", MountPoint: "/var/lib/docker", Role: metrics.FilesystemDocker},
	})
	engine.Publish(metrics.Snapshot{At: time.Now().UTC()})
	days := 6.5
	fullAt := time.Date(2026, 11, 12, 0, 0, 0, 0, time.UTC)
	engine.SetTrends(24, metrics.Trend{State: metrics.TrendSteady, Confidence: 0.95, WindowHours: 24}, map[string]metrics.Trend{
		"root": {State: metrics.TrendFilling, DaysToFull: &days, FullAt: &fullAt, Confidence: 0.8, WindowHours: 24},
	})
	server := New()
	server.EnableCurrent(engine, DemoAuthorizer(true))
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "http://binnacle.test/api/v1/server", nil))
	if response.Code != 200 {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	var body struct {
		Host struct {
			MemoryTrend map[string]any `json:"memoryTrend"`
		} `json:"host"`
		Filesystems []struct {
			MountKey string         `json:"mountKey"`
			Role     string         `json:"role"`
			Trend    map[string]any `json:"trend"`
		} `json:"filesystems"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"state", "daysToFull", "fullAt", "confidence", "windowHours"} {
		if _, ok := body.Host.MemoryTrend[key]; !ok {
			t.Fatalf("memoryTrend lacks %q: %v", key, body.Host.MemoryTrend)
		}
	}
	if body.Host.MemoryTrend["state"] != "steady" || body.Host.MemoryTrend["daysToFull"] != nil || body.Host.MemoryTrend["fullAt"] != nil {
		t.Fatalf("memoryTrend=%v", body.Host.MemoryTrend)
	}
	if len(body.Filesystems) != 2 {
		t.Fatalf("filesystems=%+v", body.Filesystems)
	}
	root, docker := body.Filesystems[0], body.Filesystems[1]
	if root.Role != "root" || root.Trend["state"] != "filling" || root.Trend["daysToFull"] != 6.5 || root.Trend["fullAt"] != "2026-11-12T00:00:00Z" || root.Trend["confidence"] != 0.8 || root.Trend["windowHours"] != float64(24) {
		t.Fatalf("root=%+v", root)
	}
	if docker.Role != "docker" || docker.Trend["state"] != "unknown" || docker.Trend["confidence"] != float64(0) {
		t.Fatalf("docker=%+v", docker)
	}

	empty := metrics.NewEngine(4)
	server = New()
	server.EnableCurrent(empty, DemoAuthorizer(true))
	response = httptest.NewRecorder()
	server.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "http://binnacle.test/api/v1/server", nil))
	var bare map[string]json.RawMessage
	if err := json.Unmarshal(response.Body.Bytes(), &bare); err != nil || string(bare["filesystems"]) != "[]" {
		t.Fatalf("empty filesystems=%s err=%v", bare["filesystems"], err)
	}
}
