// SPDX-License-Identifier: AGPL-3.0-only
package events

import (
	"github.com/drilonrecica/binnacle/internal/dockerapi"
	"github.com/drilonrecica/binnacle/internal/metrics"
	"strings"
	"time"
)

func NormalizeDocker(e dockerapi.Event, oomKilled bool) (metrics.Event, bool) {
	action := strings.ToLower(e.Action)
	kind := map[string]string{"start": metrics.EventContainerStart, "stop": metrics.EventContainerStop, "die": metrics.EventContainerDie, "destroy": metrics.EventContainerDestroy, "create": metrics.EventContainerCreate, "rename": metrics.EventContainerRename, "oom": metrics.EventContainerOOM, "restart": metrics.EventContainerRestart}[action]
	if strings.HasPrefix(action, "health_status") {
		kind = metrics.EventContainerHealthChange
	}
	if kind == "" {
		return metrics.Event{}, false
	}
	if oomKilled || action == "oom" {
		kind = metrics.EventContainerOOM
	}
	at, err := time.Parse(time.RFC3339, e.Time)
	if err != nil {
		// dockerapi always formats Time as RFC 3339; an unparsable value is
		// kept as the zero time rather than a guessed instant.
		at = time.Time{}
	}
	return metrics.Event{At: at.UTC(), Type: kind, Message: kind + " for container", ContainerInstance: metrics.ContainerID(e.ID), Details: `{"action":"` + e.Action + `"}`, CorrelationKey: e.ID}, true
}
