// SPDX-License-Identifier: AGPL-3.0-only
package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/drilonrecica/binnacle/internal/auth"
	"github.com/drilonrecica/binnacle/internal/storage"
)

func TestResourceSparklinesReturnFixedSlotsPerResource(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	manager := storage.New(filepath.Join(dir, "db"), filepath.Join(dir, "run"))
	if err := manager.Open(ctx); err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	now := time.Now().UTC()
	for minute := 1; minute <= 30; minute++ {
		ts := now.Add(-time.Duration(minute) * time.Minute).UnixMilli()
		if _, err := manager.DB().ExecContext(ctx, "INSERT INTO resource_samples_10s(ts,resource_id,cpu_host_pct,memory_working_set_bytes,active_instance_count) VALUES(?,?,?,?,1)", ts, "res_a", float64(minute), int64(minute)<<20); err != nil {
			t.Fatal(err)
		}
	}
	// Inactive samples must not contribute.
	if _, err := manager.DB().ExecContext(ctx, "INSERT INTO resource_samples_10s(ts,resource_id,cpu_host_pct,memory_working_set_bytes,active_instance_count) VALUES(?,?,?,?,0)", now.Add(-2*time.Minute).UnixMilli(), "res_inactive", 99.0, int64(1)<<30); err != nil {
		t.Fatal(err)
	}
	server := New()
	server.EnableMetrics(manager, DemoAuthorizer(true), auth.NewProtection(4096, auth.TrustedProxies{}))

	request := httptest.NewRequest(http.MethodGet, "http://binnacle.test/api/v1/metrics/sparklines?metrics=cpu,memory&range=1h", nil)
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != 200 {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	var body struct {
		StepSeconds int                              `json:"stepSeconds"`
		Resources   map[string]map[string][]*float64 `json:"resources"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.StepSeconds != 60 {
		t.Fatalf("step=%d", body.StepSeconds)
	}
	if _, ok := body.Resources["res_inactive"]; ok {
		t.Fatal("inactive resource should be excluded")
	}
	series := body.Resources["res_a"]
	if len(series["cpu"]) != 61 || len(series["memory"]) != 61 {
		t.Fatalf("slots cpu=%d memory=%d", len(series["cpu"]), len(series["memory"]))
	}
	filled := 0
	for _, value := range series["cpu"] {
		if value != nil {
			filled++
		}
	}
	if filled < 28 || filled > 31 {
		t.Fatalf("filled=%d", filled)
	}
	if series["cpu"][0] != nil {
		t.Fatal("slots before the first sample should be null")
	}

	for _, path := range []string{"/api/v1/metrics/sparklines?metrics=block_read", "/api/v1/metrics/sparklines?metrics=cpu&range=24h", "/api/v1/metrics/sparklines?metrics=cpu,memory,network_rx"} {
		request = httptest.NewRequest(http.MethodGet, "http://binnacle.test"+path, nil)
		response = httptest.NewRecorder()
		server.Handler().ServeHTTP(response, request)
		if response.Code != 400 {
			t.Errorf("%s status=%d", path, response.Code)
		}
	}
}
