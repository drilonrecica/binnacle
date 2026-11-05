// SPDX-License-Identifier: AGPL-3.0-only
package diagnostics

import (
	"testing"
	"time"
)

type notificationHealth struct{}

func (notificationHealth) HealthSnapshot() (int, int, int64, *time.Time) {
	at := time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC)
	return 3, 2, 1, &at
}

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

func TestMonitorReportsSanitizedNotificationHealth(t *testing.T) {
	monitor := &Monitor{DatabasePath: "/missing", DatabaseTarget: 100, QueueCapacity: 10, NotificationQueueCapacity: 3, Notifications: notificationHealth{}}
	byID := map[string]MonitorMetric{}
	for _, value := range monitor.Snapshot().Metrics {
		byID[value.ID] = value
	}
	if byID["notification_queue"].Value != 3 || byID["notification_queue"].Status != "critical" || byID["notification_failures"].Value != 2 || byID["notification_dropped"].Value != int64(1) || byID["notification_last_success"].Value != "2026-07-14T12:00:00Z" {
		t.Fatalf("notification health=%+v", byID)
	}
}

func TestMonitorMarksRawDockerSocket(t *testing.T) {
	for access, want := range map[string]string{"raw": "warning", "constrained": "normal", "unknown": "unavailable", "": "unavailable"} {
		monitor := &Monitor{DatabasePath: "/missing", DockerSocket: func() string { return access }}
		var found *MonitorMetric
		for _, value := range monitor.Snapshot().Metrics {
			if value.ID == "docker_socket" {
				value := value
				found = &value
			}
		}
		if found == nil || found.Status != want {
			t.Fatalf("access %q metric=%+v, want status %s", access, found, want)
		}
		if access == "" && found.Value != nil || access != "" && found.Value != access {
			t.Fatalf("access %q value=%v", access, found.Value)
		}
	}
	monitor := &Monitor{DatabasePath: "/missing"}
	for _, value := range monitor.Snapshot().Metrics {
		if value.ID == "docker_socket" {
			t.Fatal("docker_socket reported without a Docker client")
		}
	}
}
