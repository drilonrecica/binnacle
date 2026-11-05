// SPDX-License-Identifier: AGPL-3.0-only
package storage

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"time"
)

func HostIdentity(machineID, installationID string) string {
	v := machineID
	if v == "" {
		v = installationID
	}
	s := sha256.Sum256([]byte(v))
	return hex.EncodeToString(s[:])
}
func (m *Manager) UpsertHost(ctx context.Context, id, identity, name string) error {
	if m.db == nil {
		return fmt.Errorf("storage is not open")
	}
	return upsertHost(ctx, m.db, id, identity, name, time.Now())
}

func upsertHost(ctx context.Context, q querier, id, identity, name string, at time.Time) error {
	_, e := q.ExecContext(ctx, "INSERT INTO hosts(id,identity_hash,name,updated_at) VALUES(?,?,?,?) ON CONFLICT(id) DO UPDATE SET name=excluded.name,updated_at=excluded.updated_at", id, identity, name, at.UTC().Format(time.RFC3339Nano))
	return e
}

// OpenBoot records boot as the host's current boot session and ends any
// other open one.
func (m *Manager) OpenBoot(ctx context.Context, hostID, boot string) error {
	if m.db == nil {
		return fmt.Errorf("storage is not open")
	}
	tx, err := m.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, _, err = openBoot(ctx, tx, hostID, boot, time.Now()); err != nil {
		return err
	}
	return tx.Commit()
}

// openBoot ends every other open boot session of the host and opens boot. It
// returns the identity of the session it ended ("" when none was open) and
// whether boot is now recorded; it is not when the host row does not exist
// yet, which the boot_sessions foreign key requires.
func openBoot(ctx context.Context, tx *sql.Tx, hostID, boot string, at time.Time) (previous string, opened bool, err error) {
	var exists bool
	if err = tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM hosts WHERE id=?)", hostID).Scan(&exists); err != nil || !exists {
		return "", false, err
	}
	stamp := at.UTC().Format(time.RFC3339Nano)
	err = tx.QueryRowContext(ctx, "SELECT boot_identity FROM boot_sessions WHERE host_id=? AND ended_at IS NULL AND boot_identity<>? ORDER BY id DESC LIMIT 1", hostID, boot).Scan(&previous)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return "", false, err
	}
	if _, err = tx.ExecContext(ctx, "UPDATE boot_sessions SET ended_at=? WHERE host_id=? AND ended_at IS NULL AND boot_identity<>?", stamp, hostID, boot); err != nil {
		return "", false, err
	}
	if _, err = tx.ExecContext(ctx, "INSERT OR IGNORE INTO boot_sessions(host_id,boot_identity,started_at) VALUES(?,?,?)", hostID, boot, stamp); err != nil {
		return "", false, err
	}
	return previous, true, nil
}
