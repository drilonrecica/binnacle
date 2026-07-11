// SPDX-License-Identifier: AGPL-3.0-only
package diagnostics

import (
	"testing"
	"time"
)

func TestMonitorReportsUnavailableAndThresholdStates(t *testing.T) {
	monitor := &Monitor{DatabasePath: "/missing", DatabaseTarget: 100, QueueCapacity: 10}
	values := monitor.Snapshot().Metrics
	byID := map[string]MonitorMetric{}
	for _, value := range values {
		byID[value.ID] = value
	}
	if byID["database"].Status != "unavailable" || byID["rollup_duration"].Value != nil || byID["docker"].Status != "unavailable" {
		t.Fatalf("metrics=%+v", values)
	}
	if durationStatus(60*time.Millisecond, 50*time.Millisecond) != "warning" || queueStatus(10, 10) != "critical" {
		t.Fatal("thresholds not applied")
	}
}
