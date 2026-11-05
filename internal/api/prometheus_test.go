// SPDX-License-Identifier: AGPL-3.0-only
package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/drilonrecica/binnacle/internal/auth"
	"github.com/drilonrecica/binnacle/internal/checks"
	"github.com/drilonrecica/binnacle/internal/diagnostics"
	"github.com/drilonrecica/binnacle/internal/metrics"
	"github.com/drilonrecica/binnacle/internal/storage"
)

func TestPrometheusDisabledIsNotSPA(t *testing.T) {
	handler := &PrometheusHandler{}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "http://binnacle.test/metrics", nil))
	if response.Code != 404 {
		t.Fatalf("status=%d", response.Code)
	}
}

func TestPrometheusRequiresScopeAndEscapesLabels(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	manager := storage.New(filepath.Join(dir, "db"), filepath.Join(dir, "run"))
	if err := manager.Open(ctx); err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	credentials := auth.NewCredentials(manager.DB())
	user, _ := credentials.CreateAdmin(ctx, "admin", "correct horse battery staple")
	tokens := auth.NewAPITokenRepository(manager.DB())
	_, metricsToken, err := tokens.Create(ctx, user.ID, "metrics", []auth.APIScope{auth.ScopeMetricsRead}, nil)
	if err != nil {
		t.Fatal(err)
	}
	_, wrongToken, err := tokens.Create(ctx, user.ID, "events", []auth.APIScope{auth.ScopeEventsRead}, nil)
	if err != nil {
		t.Fatal(err)
	}
	cpu := 12.5
	memory := int64(1024)
	engine := metrics.NewEngine(10)
	engine.Publish(metrics.Snapshot{At: time.Now(), Host: metrics.HostObservation{CPUPercent: &cpu}, Resources: []metrics.ResourceSnapshot{{ID: "res_quote\"line\n", Category: "application", CPUHostPercent: &cpu, MemoryBytes: &memory}}, Collectors: map[string]metrics.CollectorHealth{"docker": {State: metrics.CollectorHealthy}}})
	handler := &PrometheusHandler{Enabled: true, Tokens: tokens, Engine: engine, Checks: checks.NewRepository(manager.DB())}
	for token, want := range map[string]int{"": 401, wrongToken: 403, metricsToken: 200} {
		request := httptest.NewRequest(http.MethodGet, "http://binnacle.test/metrics", nil)
		if token != "" {
			request.Header.Set("Authorization", "Bearer "+token)
		}
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != want {
			t.Errorf("token=%q status=%d want=%d body=%s", token, response.Code, want, response.Body.String())
		}
		if want == 200 {
			body := response.Body.String()
			if !strings.Contains(body, `resource_id="res_quote\"line\n"`) || strings.Contains(body, "example.com") {
				t.Fatalf("output=%s", body)
			}
			if !strings.HasPrefix(response.Header().Get("Content-Type"), "text/plain") {
				t.Fatalf("content type=%s", response.Header().Get("Content-Type"))
			}
		}
	}
}

// RUN-7: the worker failure counters and latency windows of monitor-health
// are scrapeable, labelled by operation; a window with no run yet is left out.
func TestPrometheusExposesWorkerFailuresAndLatency(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	manager := storage.New(filepath.Join(dir, "db"), filepath.Join(dir, "run"))
	if err := manager.Open(ctx); err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	if err := manager.RollupOnce(ctx, time.Now()); err != nil {
		t.Fatal(err)
	}
	credentials := auth.NewCredentials(manager.DB())
	user, err := credentials.CreateAdmin(ctx, "admin", "correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	tokens := auth.NewAPITokenRepository(manager.DB())
	_, token, err := tokens.Create(ctx, user.ID, "metrics", []auth.APIScope{auth.ScopeMetricsRead}, nil)
	if err != nil {
		t.Fatal(err)
	}
	handler := &PrometheusHandler{Enabled: true, Tokens: tokens, Engine: metrics.NewEngine(4), Monitor: &diagnostics.Monitor{DatabasePath: filepath.Join(dir, "db"), Storage: manager, DockerFailures: func() uint64 { return 3 }}}
	request := httptest.NewRequest(http.MethodGet, "http://binnacle.test/metrics", nil)
	request.Header.Set("Authorization", "Bearer "+token)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	body := response.Body.String()
	for _, want := range []string{
		`binnacle_self_failures_total{operation="docker"} 3`,
		`binnacle_self_failures_total{operation="rollup"} 0`,
		`binnacle_self_latency_milliseconds{operation="rollup",quantile="0.95"} `,
		`binnacle_self_latency_milliseconds{operation="rollup",quantile="1"} `,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %q in:\n%s", want, body)
		}
	}
	if strings.Contains(body, `operation="vacuum",quantile`) {
		t.Errorf("a latency window with no run is exported:\n%s", body)
	}
}

func TestPrometheusEscaping(t *testing.T) {
	if got := promEscape("a\\b\n\"c"); got != `a\\b\n\"c` {
		t.Fatalf("escaped=%q", got)
	}
}
