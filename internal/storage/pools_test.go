// SPDX-License-Identifier: AGPL-3.0-only
package storage

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func pragma(t *testing.T, db interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, name string) string {
	t.Helper()
	var value string
	if err := db.QueryRowContext(context.Background(), "PRAGMA "+name).Scan(&value); err != nil {
		t.Fatalf("PRAGMA %s: %v", name, err)
	}
	return value
}

func TestWriterPoolCarriesPragmasInDSN(t *testing.T) {
	manager := openTestManager(t)
	writer := manager.Writer()
	for name, want := range map[string]string{"journal_mode": "wal", "foreign_keys": "1", "busy_timeout": "5000", "synchronous": "1", "journal_size_limit": "67108864", "query_only": "0"} {
		if got := pragma(t, writer, name); got != want {
			t.Fatalf("writer PRAGMA %s=%s, want %s", name, got, want)
		}
	}
	if stats := writer.Stats(); stats.MaxOpenConnections != 1 {
		t.Fatalf("writer MaxOpenConnections=%d, want 1", stats.MaxOpenConnections)
	}
}

func TestReaderPoolIsReadOnlyWithPragmasInDSN(t *testing.T) {
	manager := openTestManager(t)
	reader := manager.Reader()
	for name, want := range map[string]string{"journal_mode": "wal", "foreign_keys": "1", "busy_timeout": "5000", "synchronous": "1", "journal_size_limit": "67108864", "query_only": "1"} {
		if got := pragma(t, reader, name); got != want {
			t.Fatalf("reader PRAGMA %s=%s, want %s", name, got, want)
		}
	}
	if stats := reader.Stats(); stats.MaxOpenConnections != 4 {
		t.Fatalf("reader MaxOpenConnections=%d, want 4", stats.MaxOpenConnections)
	}
	if _, err := reader.ExecContext(context.Background(), "INSERT INTO hosts(id,identity_hash,name,updated_at) VALUES('h','i','n','now')"); err == nil {
		t.Fatal("write on the reader pool succeeded; it must fail loudly")
	}
}

func TestPragmasSurviveReconnect(t *testing.T) {
	manager := openTestManager(t)
	for _, pool := range []*sql.DB{manager.Writer(), manager.Reader()} {
		pool.SetConnMaxLifetime(time.Nanosecond)
		time.Sleep(2 * time.Millisecond)
		if got := pragma(t, pool, "busy_timeout"); got != "5000" {
			t.Fatalf("busy_timeout after reconnect=%s, want 5000", got)
		}
		if got := pragma(t, pool, "journal_size_limit"); got != "67108864" {
			t.Fatalf("journal_size_limit after reconnect=%s, want 67108864", got)
		}
		if got := pragma(t, pool, "foreign_keys"); got != "1" {
			t.Fatalf("foreign_keys after reconnect=%s, want 1", got)
		}
		pool.SetConnMaxLifetime(0)
	}
}

func TestRoutedHandleSendsReadsToReaderAndWritesToWriter(t *testing.T) {
	manager := openTestManager(t)
	db := manager.DB()
	if got := pragma(t, db, "query_only"); got != "1" {
		t.Fatalf("DB().QueryRowContext ran on a connection with query_only=%s, want the reader pool", got)
	}
	if _, err := db.ExecContext(context.Background(), "INSERT INTO hosts(id,identity_hash,name,updated_at) VALUES('h','i','n','now')"); err != nil {
		t.Fatalf("DB().ExecContext must run on the writer: %v", err)
	}
	tx, err := db.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	var queryOnly string
	if err := tx.QueryRowContext(context.Background(), "PRAGMA query_only").Scan(&queryOnly); err != nil {
		t.Fatal(err)
	}
	if queryOnly != "0" {
		t.Fatal("DB().BeginTx must open the transaction on the writer")
	}
	var count int
	if err := tx.QueryRowContext(context.Background(), "SELECT COUNT(*) FROM hosts WHERE id='h'").Scan(&count); err != nil || count != 1 {
		t.Fatalf("committed write not visible on the writer: count=%d err=%v", count, err)
	}
}

func TestLongReadDoesNotBlockWrite(t *testing.T) {
	manager := openTestManager(t)
	ctx := context.Background()
	if _, err := manager.Writer().ExecContext(ctx, "INSERT INTO hosts(id,identity_hash,name,updated_at) VALUES('h','i','n','now')"); err != nil {
		t.Fatal(err)
	}
	// A read transaction with an open snapshot stands in for a long export.
	readTx, err := manager.Reader().BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer readTx.Rollback()
	var name string
	if err := readTx.QueryRowContext(ctx, "SELECT name FROM hosts WHERE id='h'").Scan(&name); err != nil {
		t.Fatal(err)
	}
	writeCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	started := time.Now()
	if _, err := manager.Writer().ExecContext(writeCtx, "UPDATE hosts SET name='renamed' WHERE id='h'"); err != nil {
		t.Fatalf("write blocked behind an open read: %v", err)
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("write waited %s behind an open read", elapsed)
	}
	// The read snapshot stays consistent while the write lands.
	if err := readTx.QueryRowContext(ctx, "SELECT name FROM hosts WHERE id='h'").Scan(&name); err != nil || name != "n" {
		t.Fatalf("read snapshot changed under an open transaction: name=%q err=%v", name, err)
	}
}

func TestOpenWriteDoesNotBlockRead(t *testing.T) {
	manager := openTestManager(t)
	ctx := context.Background()
	writeTx, err := manager.Writer().BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer writeTx.Rollback()
	if _, err := writeTx.ExecContext(ctx, "INSERT INTO hosts(id,identity_hash,name,updated_at) VALUES('h','i','n','now')"); err != nil {
		t.Fatal(err)
	}
	readCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	var count int
	if err := manager.Reader().QueryRowContext(readCtx, "SELECT COUNT(*) FROM hosts WHERE id='h'").Scan(&count); err != nil {
		t.Fatalf("read blocked behind an open write: %v", err)
	}
	if count != 0 {
		t.Fatalf("reader saw an uncommitted row: count=%d", count)
	}
}

func TestBootRejectsCorruptPageViaQuickCheck(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "binnacle.db")
	manager := New(path, filepath.Join(dir, "run"))
	ctx := context.Background()
	if err := manager.Open(ctx); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2000; i++ {
		if _, err := manager.Writer().ExecContext(ctx, "INSERT INTO events(id,ts,host_id,type,severity,summary,source,created_at) VALUES(?,?,?,?,?,?,?,?)", i, i, "host", "t", "info", strings.Repeat("x", 100), "s", i); err != nil {
			t.Fatal(err)
		}
	}
	if err := manager.Close(); err != nil {
		t.Fatal(err)
	}
	file, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	info, err := file.Stat()
	if err != nil {
		t.Fatal(err)
	}
	// Scribble over the page headers of every page after the first.
	garbage := []byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff}
	for offset := int64(4096); offset+int64(len(garbage)) < info.Size(); offset += 4096 {
		if _, err := file.WriteAt(garbage, offset); err != nil {
			t.Fatal(err)
		}
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	again := New(path, filepath.Join(dir, "run"))
	err = again.Open(ctx)
	if err == nil {
		_ = again.Close()
		t.Fatal("open succeeded on a corrupt database")
	}
	if !strings.Contains(err.Error(), "quick check") {
		t.Fatalf("error=%v, want the boot quick check to report the corruption", err)
	}
}

func TestIntegrityCheckRunsOnDemandOnReader(t *testing.T) {
	manager := openTestManager(t)
	result, err := manager.IntegrityCheck(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result != "ok" {
		t.Fatalf("integrity_check=%q, want ok", result)
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := manager.IntegrityCheck(cancelled); err == nil {
		t.Fatal("integrity check ignored a cancelled context")
	}
}

func TestUncleanShutdownIsReportedUntilCleanClose(t *testing.T) {
	dir := t.TempDir()
	path, run := filepath.Join(dir, "binnacle.db"), filepath.Join(dir, "run")
	ctx := context.Background()
	first := New(path, run)
	if err := first.Open(ctx); err != nil {
		t.Fatal(err)
	}
	if first.Health().UncleanShutdown {
		t.Fatal("fresh database reported an unclean shutdown")
	}
	// A crash leaves the open marker behind; simulate it by not closing.
	second := New(path, run)
	if err := second.Open(ctx); err != nil {
		t.Fatal(err)
	}
	if !second.Health().UncleanShutdown {
		t.Fatal("unclean shutdown not reported")
	}
	if err := second.Close(); err != nil {
		t.Fatal(err)
	}
	_ = first.Close()
	third := New(path, run)
	if err := third.Open(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = third.Close() })
	if third.Health().UncleanShutdown {
		t.Fatal("clean close still reported an unclean shutdown")
	}
}

func TestHealthExposesPoolBounds(t *testing.T) {
	manager := openTestManager(t)
	health := manager.Health()
	if health.Reader.MaxOpen != 4 || health.Writer.MaxOpen != 1 {
		t.Fatalf("health=%+v", health)
	}
	if health.Reader.WaitCount < 0 || health.Writer.WaitDuration < 0 {
		t.Fatalf("health=%+v", health)
	}
}

// STO-4: WriteTransactions counts commits that changed rows, not reads,
// read-only transactions, no-op writes or rollbacks.
func TestWriteTransactionsCountsOnlyRowChangingCommits(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	m := New(filepath.Join(dir, "binnacle.db"), filepath.Join(dir, "run"))
	if err := m.Open(ctx); err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	count := func(name string, want uint64, run func() error) {
		t.Helper()
		before := WriteTransactions()
		if err := run(); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if got := WriteTransactions() - before; got != want {
			t.Fatalf("%s counted %d, want %d", name, got, want)
		}
	}
	var n int
	count("reader query", 0, func() error { return m.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM hosts").Scan(&n) })
	count("read-only writer transaction", 0, func() error {
		tx, err := m.db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		if err = tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM hosts").Scan(&n); err != nil {
			return err
		}
		return tx.Commit()
	})
	count("no-op update", 0, func() error {
		_, err := m.db.ExecContext(ctx, "UPDATE hosts SET name='x' WHERE id='missing'")
		return err
	})
	count("rolled back insert", 0, func() error {
		tx, err := m.db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, "INSERT INTO hosts(id,identity_hash,name,updated_at) VALUES('a','a','a','now')"); err != nil {
			return err
		}
		return tx.Rollback()
	})
	count("autocommit insert", 1, func() error { return m.UpsertHost(ctx, "host", "identity", "Server") })
	count("two-statement transaction", 1, func() error {
		tx, err := m.db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, "UPDATE hosts SET name='a' WHERE id='host'"); err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, "UPDATE hosts SET name='b' WHERE id='host'"); err != nil {
			return err
		}
		return tx.Commit()
	})
}
