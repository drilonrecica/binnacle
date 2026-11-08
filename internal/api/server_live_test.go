// SPDX-License-Identifier: AGPL-3.0-only
package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/drilonrecica/binnacle/internal/metrics"
)

// Container detail stays out of the live frames every page receives and is
// served to Server › Live on its own; identity rides on /api/v1/server.
func TestServerLiveDetailEndpoints(t *testing.T) {
	limit := int64(1 << 30)
	engine := metrics.NewEngine(10)
	engine.SetHostIdentity(metrics.HostIdentity{Hostname: "vps-01", CPUCores: 4})
	engine.Publish(metrics.Snapshot{At: time.Now().UTC(), Resources: []metrics.ResourceSnapshot{{
		ID: "res1", Name: "shop",
		Components: []metrics.ResourceComponent{{ID: "c1", Name: "web", MemoryLimitBytes: &limit, StartedAt: "2026-11-08T10:00:00Z", Image: "shop:1", Ports: []string{"80/tcp"}}},
	}}})

	frame, err := json.Marshal(engine.Snapshot())
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"memoryLimitBytes", "startedAt", "shop:1", "80/tcp"} {
		if strings.Contains(string(frame), field) {
			t.Fatalf("live frame carries %q: %s", field, frame)
		}
	}

	server := New()
	server.EnableCurrent(engine, DemoAuthorizer(true))
	get := func(path string) *httptest.ResponseRecorder {
		response := httptest.NewRecorder()
		server.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "http://binnacle.test"+path, nil))
		return response
	}

	var details struct {
		Containers []metrics.ContainerDetail `json:"containers"`
	}
	response := get("/api/v1/server/containers")
	if response.Code != 200 || json.Unmarshal(response.Body.Bytes(), &details) != nil || len(details.Containers) != 1 {
		t.Fatalf("containers status=%d body=%s", response.Code, response.Body.String())
	}
	if got := details.Containers[0]; got.ID != "c1" || got.Image != "shop:1" || got.StartedAt == "" || got.MemoryLimitBytes == nil || len(got.Ports) != 1 {
		t.Fatalf("detail %+v", got)
	}

	var serverBody struct {
		Identity metrics.HostIdentity `json:"identity"`
	}
	response = get("/api/v1/server")
	if response.Code != 200 || json.Unmarshal(response.Body.Bytes(), &serverBody) != nil || serverBody.Identity.Hostname != "vps-01" || serverBody.Identity.CPUCores != 4 {
		t.Fatalf("server status=%d body=%s", response.Code, response.Body.String())
	}

	empty := New()
	empty.EnableCurrent(metrics.NewEngine(10), DemoAuthorizer(true))
	response = httptest.NewRecorder()
	empty.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "http://binnacle.test/api/v1/server/containers", nil))
	if response.Code != 200 || !strings.Contains(response.Body.String(), `"containers":[]`) {
		t.Fatalf("empty status=%d body=%s", response.Code, response.Body.String())
	}
}
