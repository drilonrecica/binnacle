// SPDX-License-Identifier: AGPL-3.0-only
package diagnostics

import (
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/drilonrecica/binnacle/internal/metrics"
	"github.com/drilonrecica/binnacle/internal/storage"
)

type MonitorMetric struct {
	ID     string `json:"id"`
	Label  string `json:"label"`
	Value  any    `json:"value"`
	Unit   string `json:"unit,omitempty"`
	Status string `json:"status"`
	Help   string `json:"help"`
}
type MonitorSnapshot struct {
	At      time.Time       `json:"at"`
	Metrics []MonitorMetric `json:"metrics"`
}
type PersistenceMetrics interface {
	Queue() int64
	DroppedCount() uint64
	WriteLatency() time.Duration
}

// EventLossProvider reports events of lost batches that persistence could
// not keep for the next batch (EVT-6).
type EventLossProvider interface{ EventsLostCount() uint64 }
type DurationProvider interface{ CollectionDuration() time.Duration }
type NotificationHealthProvider interface {
	HealthSnapshot() (queue int, permanent int, dropped int64, lastSuccess *time.Time)
}

// StorageHealthProvider is the storage manager's view of its pools (ADR 021).
type StorageHealthProvider interface{ Health() storage.Health }

type Monitor struct {
	DatabasePath              string
	DatabaseTarget            int64
	QueueCapacity             int
	Engine                    *metrics.Engine
	Persistence               PersistenceMetrics
	Collector                 DurationProvider
	Notifications             NotificationHealthProvider
	NotificationQueueCapacity int
	// Storage reports pool waits and unclean shutdowns. Nil omits the metrics.
	Storage StorageHealthProvider
	// DockerSocket reports the startup probe result: "constrained", "raw" or
	// "unknown" ("" before the probe ran). Nil omits the metric (demo mode).
	DockerSocket func() string
	mu           sync.Mutex
	previousCPU  time.Duration
	previousAt   time.Time
	readCPU      func() time.Duration
}

func (m *Monitor) Snapshot() MonitorSnapshot {
	now := time.Now().UTC()
	var memory runtime.MemStats
	runtime.ReadMemStats(&memory)
	rss, cpu := processRSS(), m.processCPU(now)
	dbSize, walSize := fileSize(m.DatabasePath), fileSize(m.DatabasePath+"-wal")
	queue, dropped, writeLatency := int64(0), uint64(0), time.Duration(0)
	if m.Persistence != nil {
		queue, dropped, writeLatency = m.Persistence.Queue(), m.Persistence.DroppedCount(), m.Persistence.WriteLatency()
	}
	eventsDropped := uint64(0)
	if m.Engine != nil {
		eventsDropped += m.Engine.EventsDropped()
	}
	if lost, ok := m.Persistence.(EventLossProvider); ok {
		eventsDropped += lost.EventsLostCount()
	}
	collectionDuration := time.Duration(0)
	if m.Collector != nil {
		collectionDuration = m.Collector.CollectionDuration()
	}
	var health storage.Health
	if m.Storage != nil {
		health = m.Storage.Health()
	}
	dockerStatus := "unavailable"
	if m.Engine != nil {
		if value, ok := m.Engine.Snapshot().Collectors["docker"]; ok {
			dockerStatus = string(value.State)
		}
	}
	values := []MonitorMetric{
		metric("cpu", "Binnacle CPU", cpu, "percent", statusNumber(cpu, 5, 20), "Process CPU over the latest interval."),
		metric("rss", "Resident memory", rss, "bytes", available(rss), "Resident working memory reported by Linux."),
		metric("heap", "Go heap", int64(memory.HeapAlloc), "bytes", "normal", "Currently allocated Go heap."),
		metric("goroutines", "Goroutines", runtime.NumGoroutine(), "count", "normal", "Active Go goroutines."),
		metric("database", "SQLite database", dbSize, "bytes", budgetStatus(dbSize, m.DatabaseTarget), "Main SQLite file size."),
		metric("wal", "SQLite WAL", walSize, "bytes", available(walSize), "Write-ahead log size."),
		metric("queue", "Persistence queue", queue, "batches", queueStatus(queue, m.QueueCapacity), "Queued persistence batches."),
		metric("dropped", "Dropped batches", int64(dropped), "batches", nonzeroStatus(dropped), "History batches not stored after queue overflow, the emergency pause or a failed write; each gap is recorded as a persistence_gap event."),
		metric("events_dropped", "Dropped events", int64(eventsDropped), "events", nonzeroStatus(eventsDropped), "Events shown live but never stored: evicted from the 128-event ring before a batch took them, or beyond the 128 events kept across lost batches."),
		metric("write_latency", "Persistence write latency", durationValue(writeLatency), "milliseconds", durationStatus(writeLatency, 50*time.Millisecond), "Latest SQLite persistence operation."),
		metric("rollup_duration", "Rollup duration", durationValue(health.RollupDuration), "milliseconds", durationStatus(health.RollupDuration, 500*time.Millisecond), "Latest incremental rollup pass over every tier; unavailable until the first pass."),
		metric("retention_duration", "Retention duration", durationValue(health.RetentionDuration), "milliseconds", durationStatus(health.RetentionDuration, 2*time.Second), "Latest retention pass; unavailable until the first pass."),
		metric("collection_duration", "Collector duration", durationValue(collectionDuration), "milliseconds", durationStatus(collectionDuration, 2*time.Second), "Latest host and Docker collection cycle."),
		metric("sse_clients", "SSE clients", sseClients(m.Engine), "clients", "normal", "Currently connected live clients."),
		metric("docker", "Docker API health", dockerStatus, "", dockerStatusValue(dockerStatus), "Current Docker collector state."),
	}
	if m.DockerSocket != nil {
		access := m.DockerSocket()
		var value any
		if access != "" {
			value = access
		}
		values = append(values, metric("docker_socket", "Docker socket access", value, "", socketStatus(access), "Startup probe of a Docker path Binnacle never needs. \"raw\" means the full Engine API is reachable; put a constrained socket proxy in front."))
	}
	if m.Notifications != nil {
		queue, permanent, notificationDropped, lastSuccess := m.Notifications.HealthSnapshot()
		values = append(values,
			metric("notification_queue", "Notification queue", queue, "deliveries", queueStatus(int64(queue), m.NotificationQueueCapacity), "Durable notification deliveries waiting to run."),
			metric("notification_failures", "Permanent notification failures", permanent, "deliveries", nonzeroStatus(uint64(permanent)), "Deliveries requiring configuration changes or manual retry."),
			metric("notification_dropped", "Dropped notification dispatches", notificationDropped, "dispatches", nonzeroStatus(uint64(notificationDropped)), "Worker dispatches deferred because the bounded queue was full."),
			metric("notification_last_success", "Last successful notification", lastSuccessValue(lastSuccess), "", available(lastSuccessValue(lastSuccess)), "Most recent successful outbound delivery."),
		)
	}
	if m.Storage != nil {
		values = append(values,
			metric("writer_pool_waits", "Writer pool waits", health.Writer.WaitCount, "count", "normal", "Statements that waited for the single SQLite write connection since start."),
			metric("writer_pool_wait_time", "Writer pool wait time", float64(health.Writer.WaitDuration)/float64(time.Millisecond), "milliseconds", "normal", "Total time statements spent waiting for the write connection since start."),
			metric("reader_pool_waits", "Reader pool waits", health.Reader.WaitCount, "count", "normal", "Statements that waited for one of the four read-only SQLite connections since start."),
			metric("reader_pool_wait_time", "Reader pool wait time", float64(health.Reader.WaitDuration)/float64(time.Millisecond), "milliseconds", "normal", "Total time statements spent waiting for a read-only connection since start."),
			metric("write_transactions", "Write transactions", health.WriteTransactions, "count", "normal", "SQLite write transactions committed since start; the benchmark reports the rate."),
			metric("unclean_shutdown", "Unclean shutdown", health.UncleanShutdown, "", uncleanShutdownStatus(health.UncleanShutdown), "The previous process did not close storage. SQLite recovered the WAL; run a diagnostics bundle for the full integrity check."),
			// ADR 021 §7: the budget compares live pages, not file size, and
			// shortened retention and the pause are reported here and as events.
			metric("database_budget", "Database budget used", health.Budget.UsedBytes, "bytes", budgetStateStatus(health.Budget.State), "Live data, (page_count - freelist_count) x page_size, against the configured budget; the WAL is bounded separately."),
			metric("persistence_paused", "Raw persistence paused", health.Budget.Paused, "", flagStatus(health.Budget.Paused, "critical"), "Raw 10 s history is not being stored because the budget stayed at emergency after retention was shortened; it resumes below the critical threshold."),
			metric("retention_shortened", "Retention shortened", shortenedValue(health.Budget.Shortened), "", flagStatus(len(health.Budget.Shortened) > 0, "warning"), "Tiers the latest retention pass cut to half their configured length because the budget was at emergency; 15 m and 1 h history are never shortened."),
			metric("retention_incomplete", "Retention pass incomplete", health.RetentionIncomplete, "", flagStatus(health.RetentionIncomplete, "warning"), "The latest retention pass stopped at its duration budget and resumes on the next tick."),
			metric("vacuum_duration", "Vacuum duration", durationValue(health.VacuumDuration), "milliseconds", durationStatus(health.VacuumDuration, 2*time.Second), "Latest incremental vacuum after retention; unavailable until the first pass."),
			metric("vacuum_pending", "Vacuum conversion pending", health.VacuumPending, "", flagStatus(health.VacuumPending, "warning"), "The database still uses auto_vacuum=none because the one-time conversion found no free disk space; space is not reclaimed until it runs at a later start."),
		)
	}
	return MonitorSnapshot{At: now, Metrics: values}
}
func budgetStateStatus(state storage.BudgetState) string {
	switch state {
	case storage.BudgetEmergency, storage.BudgetCritical:
		return "critical"
	case storage.BudgetWarning:
		return "warning"
	case "":
		return "unavailable"
	}
	return "normal"
}
func flagStatus(set bool, status string) string {
	if set {
		return status
	}
	return "normal"
}
func shortenedValue(tiers []string) any {
	if len(tiers) == 0 {
		return "none"
	}
	return strings.Join(tiers, ", ")
}
func uncleanShutdownStatus(unclean bool) string {
	if unclean {
		return "warning"
	}
	return "normal"
}
func lastSuccessValue(value *time.Time) any {
	if value == nil {
		return nil
	}
	return value.UTC().Format(time.RFC3339)
}

func (m *Monitor) processCPU(now time.Time) any {
	m.mu.Lock()
	defer m.mu.Unlock()
	read := m.readCPU
	if read == nil {
		read = processCPUTime
	}
	current := read()
	if m.previousAt.IsZero() {
		m.previousAt, m.previousCPU = now, current
		return nil
	}
	elapsed, delta := now.Sub(m.previousAt), current-m.previousCPU
	m.previousAt, m.previousCPU = now, current
	if elapsed <= 0 || delta < 0 {
		return nil
	}
	return delta.Seconds() * 100 / elapsed.Seconds()
}
func processCPUTime() time.Duration {
	value, err := os.ReadFile("/proc/self/stat")
	if err != nil {
		return 0
	}
	fields := strings.Fields(string(value))
	if len(fields) < 15 {
		return 0
	}
	user, _ := strconv.ParseInt(fields[13], 10, 64)
	system, _ := strconv.ParseInt(fields[14], 10, 64)
	return time.Duration(user+system) * time.Second / 100
}
func processRSS() any {
	value, err := os.ReadFile("/proc/self/statm")
	if err != nil {
		return nil
	}
	fields := strings.Fields(string(value))
	if len(fields) < 2 {
		return nil
	}
	pages, err := strconv.ParseInt(fields[1], 10, 64)
	if err != nil {
		return nil
	}
	return pages * int64(os.Getpagesize())
}
func fileSize(path string) any {
	info, err := os.Stat(path)
	if err != nil {
		return nil
	}
	return info.Size()
}
func metric(id, label string, value any, unit, status, help string) MonitorMetric {
	return MonitorMetric{ID: id, Label: label, Value: value, Unit: unit, Status: status, Help: help}
}
func available(value any) string {
	if value == nil {
		return "unavailable"
	}
	return "normal"
}
func numeric(value any) (float64, bool) {
	switch typed := value.(type) {
	case float64:
		return typed, true
	case int64:
		return float64(typed), true
	case int:
		return float64(typed), true
	}
	return 0, false
}
func statusNumber(value any, warning, critical float64) string {
	number, ok := numeric(value)
	if !ok {
		return "unavailable"
	}
	if number >= critical {
		return "critical"
	}
	if number >= warning {
		return "warning"
	}
	return "normal"
}
func budgetStatus(value any, target int64) string {
	number, ok := numeric(value)
	if !ok {
		return "unavailable"
	}
	switch storage.EvaluateBudget(int64(number), target, .8, .95, .98) {
	case storage.BudgetEmergency, storage.BudgetCritical:
		return "critical"
	case storage.BudgetWarning:
		return "warning"
	}
	return "normal"
}
func queueStatus(value int64, capacity int) string {
	if capacity <= 0 {
		return "unavailable"
	}
	ratio := float64(value) / float64(capacity)
	if ratio >= .95 {
		return "critical"
	}
	if ratio >= .8 {
		return "warning"
	}
	return "normal"
}
func nonzeroStatus(value uint64) string {
	if value > 0 {
		return "warning"
	}
	return "normal"
}
func durationValue(value time.Duration) any {
	if value <= 0 {
		return nil
	}
	return float64(value) / float64(time.Millisecond)
}
func durationStatus(value, warning time.Duration) string {
	if value <= 0 {
		return "unavailable"
	}
	if value > warning {
		return "warning"
	}
	return "normal"
}
func sseClients(engine *metrics.Engine) int {
	if engine == nil {
		return 0
	}
	return engine.SSEClients()
}
func socketStatus(value string) string {
	switch value {
	case "constrained":
		return "normal"
	case "raw":
		return "warning"
	}
	return "unavailable"
}
func dockerStatusValue(value string) string {
	if value == "healthy" {
		return "normal"
	}
	if value == "degraded" {
		return "warning"
	}
	if value == "down" {
		return "critical"
	}
	return "unavailable"
}
