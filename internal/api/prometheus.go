// SPDX-License-Identifier: AGPL-3.0-only
package api

import (
	"bytes"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/drilonrecica/binnacle/internal/auth"
	"github.com/drilonrecica/binnacle/internal/checks"
	"github.com/drilonrecica/binnacle/internal/diagnostics"
	"github.com/drilonrecica/binnacle/internal/metrics"
)

type PrometheusHandler struct {
	Enabled bool
	Tokens  *auth.APITokenRepository
	Engine  *metrics.Engine
	Checks  *checks.Repository
	Monitor *diagnostics.Monitor
}

func (p *PrometheusHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !p.Enabled {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", "GET")
		w.WriteHeader(405)
		return
	}
	header := r.Header.Get("Authorization")
	parts := strings.Fields(header)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		w.Header().Set("WWW-Authenticate", `Bearer realm="binnacle", scope="metrics:read"`)
		WriteError(w, 401, Error{Code: "invalid_token", Message: "A metrics:read API token is required."})
		return
	}
	if err := p.Tokens.Authenticate(r.Context(), parts[1], auth.ScopeMetricsRead); err != nil {
		if errors.Is(err, auth.ErrAPIScopeInsufficient) {
			WriteError(w, 403, Error{Code: "insufficient_scope", Message: "The API token does not grant metrics:read."})
			return
		}
		w.Header().Set("WWW-Authenticate", `Bearer realm="binnacle", scope="metrics:read"`)
		WriteError(w, 401, Error{Code: "invalid_token", Message: "A metrics:read API token is required."})
		return
	}
	var output bytes.Buffer
	snapshot := p.Engine.Snapshot()
	sample := func(name string, value any, labels string) {
		number, ok := promNumber(value)
		if !ok {
			return
		}
		fmt.Fprintf(&output, "%s%s %s\n", name, labels, strconv.FormatFloat(number, 'g', -1, 64))
	}
	sample("binnacle_host_cpu_percent", snapshot.Host.CPUPercent, "")
	sample("binnacle_host_memory_used_bytes", snapshot.Host.MemoryUsedBytes, "")
	sample("binnacle_host_memory_total_bytes", snapshot.Host.MemoryTotalBytes, "")
	sample("binnacle_host_disk_used_bytes", snapshot.Host.DiskUsedBytes, "")
	sample("binnacle_host_disk_total_bytes", snapshot.Host.DiskTotalBytes, "")
	sample("binnacle_host_disk_read_bytes_per_second", snapshot.Host.DiskReadBPS, "")
	sample("binnacle_host_disk_write_bytes_per_second", snapshot.Host.DiskWriteBPS, "")
	sample("binnacle_host_network_receive_bytes_per_second", snapshot.Host.NetworkRXBPS, "")
	sample("binnacle_host_network_transmit_bytes_per_second", snapshot.Host.NetworkTXBPS, "")
	for _, resource := range snapshot.Resources {
		labels := promLabels(map[string]string{"resource_id": string(resource.ID), "category": resource.Category})
		sample("binnacle_resource_cpu_percent", resource.CPUHostPercent, labels)
		sample("binnacle_resource_memory_bytes", resource.MemoryBytes, labels)
		sample("binnacle_resource_network_receive_bytes_per_second", resource.RXBPS, labels)
		sample("binnacle_resource_network_transmit_bytes_per_second", resource.TXBPS, labels)
		sample("binnacle_resource_disk_read_bytes_per_second", resource.BlockReadBPS, labels)
		sample("binnacle_resource_disk_write_bytes_per_second", resource.BlockWriteBPS, labels)
	}
	for name, state := range snapshot.Collectors {
		sample("binnacle_collector_state", 1, promLabels(map[string]string{"collector": name, "state": string(state.State)}))
	}
	if p.Checks != nil {
		states, _ := p.Checks.PrometheusStates(r.Context(), 1000)
		for _, state := range states {
			value := 0.0
			if state.Status == "success" {
				value = 1
			} else if state.Status == "unknown" {
				value = -1
			}
			sample("binnacle_health_check_state", value, promLabels(map[string]string{"check_id": state.CheckID, "resource_id": state.ResourceID}))
		}
	}
	if p.Monitor != nil {
		for _, metric := range p.Monitor.Snapshot().Metrics {
			sample("binnacle_self_"+promMetricName(metric.ID), metric.Value, "")
		}
	}
	if output.Len() > 4<<20 {
		WriteError(w, 503, Error{Code: "metrics_too_large", Message: "Prometheus output exceeds the configured bound."})
		return
	}
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(200)
	_, _ = w.Write(output.Bytes())
}
func promNumber(value any) (float64, bool) {
	switch value := value.(type) {
	case *float64:
		if value == nil {
			return 0, false
		}
		return *value, true
	case *int64:
		if value == nil {
			return 0, false
		}
		return float64(*value), true
	case float64:
		return value, true
	case int:
		return float64(value), true
	case int64:
		return float64(value), true
	case uint64:
		return float64(value), true
	}
	return 0, false
}
func promLabels(values map[string]string) string {
	if len(values) == 0 {
		return ""
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, key+`="`+promEscape(values[key])+`"`)
	}
	return "{" + strings.Join(parts, ",") + "}"
}
func promEscape(value string) string {
	return strings.NewReplacer("\\", "\\\\", "\n", "\\n", "\"", "\\\"").Replace(value)
}

var invalidMetric = regexp.MustCompile(`[^a-zA-Z0-9_:]`)

func promMetricName(value string) string { return invalidMetric.ReplaceAllString(value, "_") }
