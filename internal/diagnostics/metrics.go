// SPDX-License-Identifier: AGPL-3.0-only
package diagnostics

import "sync/atomic"

type Metrics struct {
	QueueDepth     atomic.Int64
	DroppedBatches atomic.Uint64
	SSEClients     atomic.Int64
	DockerErrors   atomic.Uint64
}

func (m *Metrics) Snapshot() map[string]int64 {
	return map[string]int64{"queue_depth": m.QueueDepth.Load(), "dropped_batches": int64(m.DroppedBatches.Load()), "sse_clients": m.SSEClients.Load(), "docker_errors": int64(m.DockerErrors.Load())}
}
