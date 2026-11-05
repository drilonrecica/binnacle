// SPDX-License-Identifier: AGPL-3.0-only
package storage

import (
	"context"
	"fmt"
	"time"
)

const (
	// defaultOptimizeInterval is SQLite's advice for long-lived connections:
	// PRAGMA optimize every few hours, so planner statistics follow the data.
	defaultOptimizeInterval = 6 * time.Hour
	// defaultOptimizeBootDelay keeps the first pass off the boot path.
	defaultOptimizeBootDelay = 2 * time.Minute
	// optimizeAnalysisLimit bounds the rows ANALYZE reads per index, so a
	// pass costs the same at 10 MB and at the 1 GB budget.
	optimizeAnalysisLimit = 400
)

// runOptimize refreshes the query planner statistics shortly after boot and
// then every optimizeInterval (STO-7).
func (m *Manager) runOptimize(ctx context.Context) {
	m.mu.Lock()
	interval, bootDelay := m.optimizeInterval, m.optimizeBootDelay
	m.mu.Unlock()
	if interval <= 0 {
		interval = defaultOptimizeInterval
	}
	if bootDelay <= 0 {
		bootDelay = defaultOptimizeBootDelay
	}
	boot := time.NewTimer(bootDelay)
	defer boot.Stop()
	tick := time.NewTicker(interval)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-boot.C:
		case <-tick.C:
		}
		if err := m.optimize(ctx); err != nil && ctx.Err() == nil {
			m.errorLog().Warn("PRAGMA optimize failed", "error", err)
		}
	}
}

// optimize runs PRAGMA optimize on the writer, the only connection allowed
// to store statistics. The readers run the history queries, so the writer's
// own query log says nothing about them: mask 0x10002 makes SQLite consider
// every table instead of only those this connection used. New statistics
// change the schema cookie, so each reader reloads them on its next query.
func (m *Manager) optimize(ctx context.Context) error {
	conn, err := m.writer.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	if _, err := conn.ExecContext(ctx, fmt.Sprintf("PRAGMA analysis_limit=%d", optimizeAnalysisLimit)); err != nil {
		return err
	}
	_, err = conn.ExecContext(ctx, "PRAGMA optimize=0x10002")
	return err
}
