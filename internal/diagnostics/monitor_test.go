// SPDX-License-Identifier: AGPL-3.0-only
package diagnostics

import (
	"testing"
	"time"

	"github.com/drilonrecica/binnacle/internal/storage"
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

type storageHealth struct{ unclean bool }

func (s storageHealth) Health() storage.Health {
	return storage.Health{UncleanShutdown: s.unclean, Writer: storage.PoolStats{MaxOpen: 1, WaitCount: 7, WaitDuration: 350 * time.Millisecond}, Reader: storage.PoolStats{MaxOpen: 4, WaitCount: 2, WaitDuration: 20 * time.Millisecond}, WriteTransactions: 42}
}

func TestMonitorReportsPoolWaitsAndUncleanShutdown(t *testing.T) {
	byID := map[string]MonitorMetric{}
	for _, value := range (&Monitor{DatabasePath: "/missing", Storage: storageHealth{unclean: true}}).Snapshot().Metrics {
		byID[value.ID] = value
	}
	if byID["writer_pool_waits"].Value != int64(7) || byID["writer_pool_wait_time"].Value != float64(350) || byID["reader_pool_waits"].Value != int64(2) || byID["reader_pool_wait_time"].Value != float64(20) {
		t.Fatalf("pool metrics=%+v", byID)
	}
	if byID["write_transactions"].Value != uint64(42) {
		t.Fatalf("write_transactions=%+v", byID["write_transactions"])
	}
	if byID["unclean_shutdown"].Value != true || byID["unclean_shutdown"].Status != "warning" {
		t.Fatalf("unclean_shutdown=%+v", byID["unclean_shutdown"])
	}
	for _, value := range (&Monitor{DatabasePath: "/missing", Storage: storageHealth{}}).Snapshot().Metrics {
		if value.ID == "unclean_shutdown" && (value.Value != false || value.Status != "normal") {
			t.Fatalf("clean shutdown reported as %+v", value)
		}
	}
	for _, value := range (&Monitor{DatabasePath: "/missing"}).Snapshot().Metrics {
		if value.ID == "unclean_shutdown" || value.ID == "reader_pool_waits" {
			t.Fatal("storage metrics reported without a storage manager")
		}
	}
}

type durationHealth struct{ health storage.Health }

func (s durationHealth) Health() storage.Health { return s.health }

func TestMonitorReportsRollupAndRetentionDurations(t *testing.T) {
	monitor := &Monitor{DatabasePath: "/missing", DatabaseTarget: 100, QueueCapacity: 10, Storage: durationHealth{storage.Health{RollupDuration: 12 * time.Millisecond, RetentionDuration: 3 * time.Second}}}
	byID := map[string]MonitorMetric{}
	for _, value := range monitor.Snapshot().Metrics {
		byID[value.ID] = value
	}
	if byID["rollup_duration"].Value != float64(12) || byID["rollup_duration"].Status != "normal" {
		t.Fatalf("rollup_duration=%+v", byID["rollup_duration"])
	}
	if byID["retention_duration"].Value != float64(3000) || byID["retention_duration"].Status != "warning" {
		t.Fatalf("retention_duration=%+v", byID["retention_duration"])
	}
}

type fakeStorageHealth struct{ health storage.Health }

func (f fakeStorageHealth) Health() storage.Health { return f.health }

// STO-5 / STO-9: the budget measured from live pages, the persistence pause,
// shortened retention, pending vacuum and an incomplete pass are visible in
// monitor-health (ADR 021 §7).
func TestMonitorReportsBudgetPauseAndVacuumState(t *testing.T) {
	health := storage.Health{
		Budget:              storage.DatabaseBudget{UsedBytes: 990, TargetBytes: 1000, State: storage.BudgetEmergency, Paused: true, Shortened: []string{"raw", "1m"}},
		VacuumDuration:      40 * time.Millisecond,
		VacuumPending:       true,
		RetentionIncomplete: true,
	}
	monitor := &Monitor{DatabasePath: "/missing", DatabaseTarget: 1000, QueueCapacity: 10, Storage: fakeStorageHealth{health}}
	byID := map[string]MonitorMetric{}
	for _, value := range monitor.Snapshot().Metrics {
		byID[value.ID] = value
	}
	if byID["database_budget"].Value != int64(990) || byID["database_budget"].Status != "critical" {
		t.Fatalf("database_budget=%+v", byID["database_budget"])
	}
	if byID["persistence_paused"].Value != true || byID["persistence_paused"].Status != "critical" {
		t.Fatalf("persistence_paused=%+v", byID["persistence_paused"])
	}
	if byID["retention_shortened"].Value != "raw, 1m" || byID["retention_shortened"].Status != "warning" {
		t.Fatalf("retention_shortened=%+v", byID["retention_shortened"])
	}
	if byID["vacuum_duration"].Value != float64(40) || byID["vacuum_pending"].Status != "warning" || byID["retention_incomplete"].Status != "warning" {
		t.Fatalf("vacuum_duration=%+v vacuum_pending=%+v retention_incomplete=%+v", byID["vacuum_duration"], byID["vacuum_pending"], byID["retention_incomplete"])
	}

	quiet := &Monitor{DatabasePath: "/missing", DatabaseTarget: 1000, QueueCapacity: 10, Storage: fakeStorageHealth{storage.Health{Budget: storage.DatabaseBudget{UsedBytes: 10, TargetBytes: 1000, State: storage.BudgetOK}}}}
	byID = map[string]MonitorMetric{}
	for _, value := range quiet.Snapshot().Metrics {
		byID[value.ID] = value
	}
	if byID["database_budget"].Status != "normal" || byID["persistence_paused"].Status != "normal" || byID["retention_shortened"].Value != "none" || byID["retention_shortened"].Status != "normal" || byID["vacuum_pending"].Status != "normal" {
		t.Fatalf("quiet metrics: budget=%+v paused=%+v shortened=%+v pending=%+v", byID["database_budget"], byID["persistence_paused"], byID["retention_shortened"], byID["vacuum_pending"])
	}
}
