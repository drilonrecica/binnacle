// SPDX-License-Identifier: AGPL-3.0-only
package metrics

// Event types Binnacle writes to the events table. This catalogue is the one
// source of the list: emitters use these constants, and the web UI's list
// (web/src/lib/event-types.generated.ts) is generated from EventTypes by
// TestWebEventTypesAreCurrent (UI-11). A type that is only read somewhere but
// never written does not belong here.
const (
	EventContainerCreate       = "container_create"
	EventContainerStart        = "container_start"
	EventContainerStop         = "container_stop"
	EventContainerDie          = "container_die"
	EventContainerDestroy      = "container_destroy"
	EventContainerRename       = "container_rename"
	EventContainerOOM          = "container_oom"
	EventContainerRestart      = "container_restart"
	EventContainerHealthChange = "container_health_status_change"
	EventDeployment            = "deployment"
	EventHostReboot            = "host_reboot"
	// EventCollectorDegraded is written by demo mode only today; production
	// records collector transitions in collector_state_events.
	EventCollectorDegraded    = "collector_degraded"
	EventPersistenceGap       = "persistence_gap"
	EventPersistenceEmergency = "persistence_emergency"
	EventPersistenceResumed   = "persistence_resumed"
	EventRetentionShortened   = "retention_shortened"
	EventAlertTriggered       = "alert_triggered"
	EventAlertRepeated        = "alert_repeated"
	EventAlertResolved        = "alert_resolved"
)

// EventTypeInfo describes one event type for the UI.
type EventTypeInfo struct {
	Type string
	// Annotate marks the types drawn as markers on metric charts: changes
	// that explain a bend in a curve (a deploy, an OOM kill, a reboot, a
	// history gap). Alert and bookkeeping events are left off the charts.
	Annotate bool
}

// EventTypes lists every type in the catalogue, in a stable order.
var EventTypes = []EventTypeInfo{
	{EventContainerCreate, false},
	{EventContainerStart, true},
	{EventContainerStop, true},
	{EventContainerDie, true},
	{EventContainerDestroy, false},
	{EventContainerRename, false},
	{EventContainerOOM, true},
	{EventContainerRestart, true},
	{EventContainerHealthChange, false},
	{EventDeployment, true},
	{EventHostReboot, true},
	{EventCollectorDegraded, true},
	{EventPersistenceGap, true},
	{EventPersistenceEmergency, true},
	{EventPersistenceResumed, true},
	{EventRetentionShortened, true},
	{EventAlertTriggered, false},
	{EventAlertRepeated, false},
	{EventAlertResolved, false},
}
