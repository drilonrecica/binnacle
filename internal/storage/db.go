// SPDX-License-Identifier: AGPL-3.0-only
package storage

import (
	"context"
	"database/sql"
	"errors"
)

// DB routes statements between the two pools of ADR 021. Query and QueryRow
// run on the read-only reader pool; Exec and transactions run on the single
// writer connection. The reader pool is opened with mode=ro and query_only, so
// a write that is issued through Query or QueryRow fails loudly instead of
// competing for the write lock. Code that needs read-your-writes inside one
// request does both on the writer inside one transaction.
type DB struct {
	writer *sql.DB
	reader *sql.DB
}

// NewDB wraps an already opened writer and reader pool. The storage manager
// builds it in Open; tests that need a routed handle over their own pools can
// build one too.
func NewDB(writer, reader *sql.DB) *DB { return &DB{writer: writer, reader: reader} }

// Writer is the single-connection pool every write and transaction runs on.
func (d *DB) Writer() *sql.DB { return d.writer }

// Reader is the read-only pool every stand-alone SELECT runs on.
func (d *DB) Reader() *sql.DB { return d.reader }

func (d *DB) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	return d.writer.ExecContext(ctx, query, args...)
}
func (d *DB) Exec(query string, args ...any) (sql.Result, error) {
	return d.writer.ExecContext(context.Background(), query, args...)
}
func (d *DB) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	return d.reader.QueryContext(ctx, query, args...)
}
func (d *DB) Query(query string, args ...any) (*sql.Rows, error) {
	return d.reader.QueryContext(context.Background(), query, args...)
}
func (d *DB) QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	return d.reader.QueryRowContext(ctx, query, args...)
}
func (d *DB) QueryRow(query string, args ...any) *sql.Row {
	return d.reader.QueryRowContext(context.Background(), query, args...)
}
func (d *DB) BeginTx(ctx context.Context, opts *sql.TxOptions) (*sql.Tx, error) {
	return d.writer.BeginTx(ctx, opts)
}
func (d *DB) Begin() (*sql.Tx, error) { return d.writer.BeginTx(context.Background(), nil) }

// PingContext verifies both pools can reach the database.
func (d *DB) PingContext(ctx context.Context) error {
	if d == nil || d.writer == nil || d.reader == nil {
		return errors.New("storage is not open")
	}
	if err := d.writer.PingContext(ctx); err != nil {
		return err
	}
	return d.reader.PingContext(ctx)
}
