// SPDX-License-Identifier: AGPL-3.0-only
package storage

import (
	"context"
	"github.com/drilonrecica/talos/internal/metrics"
)

func (m *Manager) WriteBatch(ctx context.Context, b metrics.PersistenceBatch) error {
	tx, e := m.db.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	s := b.Snapshot
	if s.Sequence > 0 {
		_, e = tx.ExecContext(ctx, "INSERT OR REPLACE INTO host_samples_10s(ts,host_id,cpu_busy_pct,memory_used_bytes,network_rx_bps,network_tx_bps) VALUES(?,?,?,?,?,?)", s.At.UnixMilli(), "host", s.Host.CPUPercent, s.Host.MemoryUsedBytes, s.Host.NetworkRXBPS, s.Host.NetworkTXBPS)
		if e != nil {
			return e
		}
	}
	return tx.Commit()
}
func (m *Manager) WriteEvent(ctx context.Context, e metrics.Event) error {
	_, err := m.db.ExecContext(ctx, "INSERT OR IGNORE INTO events(id,ts,type,severity,summary,source,created_at) VALUES(?,?,?,?,?,?,?)", e.ID, e.At.UnixMilli(), e.Type, "info", e.Message, "talos", e.At.UnixMilli())
	return err
}
