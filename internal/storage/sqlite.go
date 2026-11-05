// SPDX-License-Identifier: AGPL-3.0-only

package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/drilonrecica/binnacle/migrations"
	"github.com/mattn/go-sqlite3"
	"golang.org/x/sys/unix"
)

const MinimumFreeSpace = 64 << 20

// Pool bounds and connection settings from ADR 021. They are constants, not
// settings, so the bounds cannot be configured away.
const (
	driverName            = "sqlite3_binnacle"
	readerPoolSize        = 4
	busyTimeoutMillis     = 5000
	journalSizeLimit      = 64 << 20
	openMarkerName        = "storage.open"
	IntegrityCheckTimeout = 2 * time.Minute
)

func init() {
	// journal_size_limit has no DSN parameter in the driver, so it is the one
	// pragma applied by a connect hook. The hook runs for every connection the
	// pools open, including reconnects, so nothing is lost after ErrBadConn.
	sql.Register(driverName, &sqlite3.SQLiteDriver{ConnectHook: func(conn *sqlite3.SQLiteConn) error {
		_, err := conn.Exec(fmt.Sprintf("PRAGMA journal_size_limit=%d", journalSizeLimit), nil)
		return err
	}})
}

// Manager owns Binnacle's SQLite pools and acts as an app lifecycle component.
type Manager struct {
	Path, RuntimeDir string
	// Logger receives storage warnings; nil falls back to slog.Default().
	Logger *slog.Logger
	db     *DB
	writer *sql.DB
	reader *sql.DB
	// uncleanShutdown is set when the previous process did not close storage.
	uncleanShutdown bool
	mu              sync.Mutex
	workerCancel    context.CancelFunc
	workerWG        sync.WaitGroup
	prevCollectors  map[string]string
	// lastSampleAt is the newest resource observation (LastSeenAt) committed;
	// resources observed at or before it were already stored.
	lastSampleAt   time.Time
	retention      RetentionCutoffs
	budget         DatabaseBudget
	emergencyPause bool
	// rollupMu serializes rollup passes (the worker and RebuildRollups).
	rollupMu sync.Mutex
	// rollupDuration and retentionDuration are the wall-clock time of the
	// latest pass of each worker, zero until one completed; guarded by mu.
	rollupDuration    time.Duration
	retentionDuration time.Duration
}

// RetentionCutoffs configures how long each resolution tier is kept.
type RetentionCutoffs struct {
	Raw, OneMinute, FifteenMinute, OneHour time.Duration
}

// DatabaseBudget exposes the current budget state to callers.
type DatabaseBudget struct {
	UsedBytes   int64
	TargetBytes int64
	State       BudgetState
}

func New(path, runtimeDir string) *Manager { return &Manager{Path: path, RuntimeDir: runtimeDir} }
func (m *Manager) Start(ctx context.Context) error {
	if err := m.Open(ctx); err != nil {
		return err
	}
	workerCtx, cancel := context.WithCancel(ctx)
	if err := m.recoverDeletionJobs(ctx); err != nil {
		cancel()
		_ = m.Close()
		return err
	}
	m.mu.Lock()
	m.workerCancel = cancel
	m.mu.Unlock()
	m.workerWG.Add(3)
	go func() { defer m.workerWG.Done(); m.runDeletionWorker(workerCtx) }()
	go func() { defer m.workerWG.Done(); m.runRollups(workerCtx) }()
	go func() { defer m.workerWG.Done(); m.runRetentionAndBudget(workerCtx) }()
	return nil
}
func (m *Manager) Stop(context.Context) error {
	m.mu.Lock()
	cancel := m.workerCancel
	m.workerCancel = nil
	m.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	m.workerWG.Wait()
	return m.Close()
}

// DB is the routed handle: reads go to the reader pool, writes and
// transactions to the writer. Nil until Open succeeds.
func (m *Manager) DB() *DB { return m.db }

// Writer is the single-connection write pool. Nil until Open succeeds.
func (m *Manager) Writer() *sql.DB { return m.writer }

// Reader is the read-only pool of four connections. Nil until Open succeeds.
func (m *Manager) Reader() *sql.DB { return m.reader }

// PoolStats is the bounded view of one pool exposed in monitor-health.
type PoolStats struct {
	MaxOpen, Open, InUse int
	WaitCount            int64
	WaitDuration         time.Duration
}

// Health reports the pool bounds and whether the previous process exited
// without closing storage.
type Health struct {
	UncleanShutdown bool
	Writer, Reader  PoolStats
	// RollupDuration and RetentionDuration are the wall-clock time of the
	// latest pass of each worker; zero until the first pass completes.
	RollupDuration, RetentionDuration time.Duration
}

func (m *Manager) Health() Health {
	m.mu.Lock()
	defer m.mu.Unlock()
	return Health{UncleanShutdown: m.uncleanShutdown, Writer: poolStats(m.writer), Reader: poolStats(m.reader), RollupDuration: m.rollupDuration, RetentionDuration: m.retentionDuration}
}
func poolStats(db *sql.DB) PoolStats {
	if db == nil {
		return PoolStats{}
	}
	s := db.Stats()
	return PoolStats{MaxOpen: s.MaxOpenConnections, Open: s.OpenConnections, InUse: s.InUse, WaitCount: s.WaitCount, WaitDuration: s.WaitDuration}
}

// IntegrityCheck runs the full PRAGMA integrity_check on a reader connection,
// bounded by IntegrityCheckTimeout. It returns "ok" or the problems SQLite
// reported, one per line, as a result for the diagnostics bundle; it never
// runs at boot (ADR 021 §3).
func (m *Manager) IntegrityCheck(ctx context.Context) (string, error) {
	reader := m.Reader()
	if reader == nil {
		return "", errors.New("storage is not open")
	}
	ctx, cancel := context.WithTimeout(ctx, IntegrityCheckTimeout)
	defer cancel()
	rows, err := reader.QueryContext(ctx, "PRAGMA integrity_check")
	if err != nil {
		return "", err
	}
	defer rows.Close()
	var lines []string
	for rows.Next() {
		var line string
		if err := rows.Scan(&line); err != nil {
			return "", err
		}
		lines = append(lines, line)
	}
	if err := rows.Err(); err != nil {
		return "", err
	}
	return strings.Join(lines, "\n"), nil
}

func (m *Manager) SchemaVersion(ctx context.Context) (int, error) {
	if m.db == nil {
		return 0, errors.New("storage is not open")
	}
	var v int
	err := m.db.QueryRowContext(ctx, "SELECT COALESCE(MAX(version), 0) FROM schema_migrations").Scan(&v)
	return v, err
}
func (m *Manager) Open(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.db != nil {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(m.Path), 0750); err != nil {
		return err
	}
	if err := os.MkdirAll(m.RuntimeDir, 0750); err != nil {
		return err
	}
	marker := m.markerPath()
	if _, err := os.Stat(marker); err == nil {
		return fmt.Errorf("migration failure marker exists at %s; resolve it before retrying", marker)
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := freeSpace(filepath.Dir(m.Path)); err != nil {
		return err
	}
	if err := ensurePrivateFile(m.Path); err != nil {
		return err
	}
	openMarker := m.openMarkerPath()
	if _, err := os.Stat(openMarker); err == nil {
		m.uncleanShutdown = true
		m.logger().Warn("storage was not closed by the previous process; SQLite recovered the WAL, run the full integrity check from diagnostics to verify the database", "marker", openMarker)
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	writer, err := sql.Open(driverName, writerDSN(m.Path))
	if err != nil {
		return err
	}
	writer.SetMaxOpenConns(1)
	writer.SetMaxIdleConns(1)
	fail := func(err error) error { _ = writer.Close(); return err }
	// quick_check is bounded by the tree walk; the O(size) integrity_check is
	// an operator tool reached through diagnostics (ADR 021 §3).
	var integrity string
	if err := writer.QueryRowContext(ctx, "PRAGMA quick_check").Scan(&integrity); err != nil {
		return fail(fmt.Errorf("sqlite quick check: %w", err))
	}
	if integrity != "ok" {
		return fail(fmt.Errorf("sqlite quick check failed: %s", sanitize(integrity)))
	}
	if err := m.migrate(ctx, writer); err != nil {
		_ = writeMarker(marker)
		return fail(err)
	}
	reader, err := sql.Open(driverName, readerDSN(m.Path))
	if err != nil {
		return fail(err)
	}
	reader.SetMaxOpenConns(readerPoolSize)
	reader.SetMaxIdleConns(readerPoolSize)
	if err := reader.PingContext(ctx); err != nil {
		_ = reader.Close()
		return fail(fmt.Errorf("open read-only pool: %w", err))
	}
	if err := os.WriteFile(openMarker, []byte("binnacle storage is open; this file is removed on a clean close\n"), 0600); err != nil {
		_ = reader.Close()
		return fail(fmt.Errorf("write open marker: %w", err))
	}
	m.writer, m.reader, m.db = writer, reader, NewDB(writer, reader)
	return nil
}
func (m *Manager) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.db == nil {
		return nil
	}
	if _, err := m.writer.Exec("PRAGMA wal_checkpoint(PASSIVE)"); err != nil {
		m.logger().Warn("WAL checkpoint on close failed", "error", err)
	}
	err := m.reader.Close()
	if writerErr := m.writer.Close(); err == nil {
		err = writerErr
	}
	m.db, m.writer, m.reader = nil, nil, nil
	if removeErr := os.Remove(m.openMarkerPath()); err == nil && removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
		err = removeErr
	}
	return err
}
func (m *Manager) logger() *slog.Logger {
	if m.Logger != nil {
		return m.Logger
	}
	return slog.Default()
}

// writerDSN and readerDSN carry every connection setting so that each
// connection the pools open, including reconnects, has them (ADR 021 §2).
func writerDSN(path string) string {
	return fmt.Sprintf("file:%s?_txlock=immediate&_journal_mode=WAL&_synchronous=NORMAL&_busy_timeout=%d&_foreign_keys=on&_auto_vacuum=incremental", uriPath(path), busyTimeoutMillis)
}
func readerDSN(path string) string {
	return fmt.Sprintf("file:%s?mode=ro&_query_only=true&_journal_mode=WAL&_synchronous=NORMAL&_busy_timeout=%d&_foreign_keys=on", uriPath(path), busyTimeoutMillis)
}

// uriPath escapes the characters SQLite's URI parser would otherwise read as
// query or fragment delimiters.
func uriPath(path string) string {
	return strings.NewReplacer("%", "%25", "?", "%3F", "#", "%23").Replace(path)
}
func (m *Manager) openMarkerPath() string { return filepath.Join(m.RuntimeDir, openMarkerName) }
func (m *Manager) migrate(ctx context.Context, db *sql.DB) error {
	if _, err := db.ExecContext(ctx, "CREATE TABLE IF NOT EXISTS schema_migrations (version INTEGER PRIMARY KEY, applied_at TEXT NOT NULL)"); err != nil {
		return err
	}
	entries, err := fs.Glob(migrations.FS(), "*.sql")
	if err != nil {
		return err
	}
	sort.Strings(entries)
	for _, entry := range entries {
		base := filepath.Base(entry)
		var version int
		if _, err := fmt.Sscanf(base, "%03d_", &version); err != nil {
			return fmt.Errorf("invalid migration name %s", base)
		}
		var present int
		err = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM schema_migrations WHERE version=?", version).Scan(&present)
		if err != nil {
			return err
		}
		if present > 0 {
			continue
		}
		sqlBytes, err := migrations.FS().ReadFile(entry)
		if err != nil {
			return err
		}
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, string(sqlBytes)); err == nil {
			_, err = tx.ExecContext(ctx, "INSERT INTO schema_migrations(version, applied_at) VALUES(?, datetime('now'))", version)
		}
		if err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("migration %03d failed: %w", version, err)
		}
		if err = tx.Commit(); err != nil {
			return fmt.Errorf("migration %03d commit: %w", version, err)
		}
	}
	return nil
}

// ensurePrivateFile creates the database file 0600 before SQLite opens it and
// tightens an existing file that other users could read. SQLite gives the WAL
// and shared-memory files the main file's mode, so they follow.
func ensurePrivateFile(path string) error {
	file, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0600)
	if err == nil {
		return file.Close()
	}
	if !errors.Is(err, os.ErrExist) {
		return fmt.Errorf("create database file: %w", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if info.Mode().Perm()&0077 != 0 {
		if err := os.Chmod(path, info.Mode().Perm()&0700); err != nil {
			return fmt.Errorf("restrict database file mode: %w", err)
		}
	}
	return nil
}
func (m *Manager) markerPath() string { return filepath.Join(m.RuntimeDir, "migration.failed") }
func freeSpace(path string) error {
	var stat unix.Statfs_t
	if err := unix.Statfs(path, &stat); err != nil {
		return err
	}
	if stat.Bavail*uint64(stat.Bsize) < MinimumFreeSpace {
		return fmt.Errorf("insufficient free disk space for migration (need %d bytes)", MinimumFreeSpace)
	}
	return nil
}
func writeMarker(path string) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte("migration failed; inspect application logs and restore or repair the database before removing this marker\n"), 0600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
func sanitize(s string) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) > 200 {
		return s[:200]
	}
	return s
}
