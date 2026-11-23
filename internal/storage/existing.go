// SPDX-License-Identifier: AGPL-3.0-only

package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/drilonrecica/binnacle/migrations"
)

// ErrSchemaVersion reports that a database opened with OpenExisting is not at
// the schema version this binary was built for.
var ErrSchemaVersion = errors.New("database schema version does not match this binary")

// OpenExisting opens a database that another process may own, for a one-shot
// operator command run beside the server. Unlike Manager.Open it creates
// nothing and changes nothing on the way in: no migration, no integrity
// check, no auto-vacuum conversion and no open marker, all of which belong to
// the server. It uses one writer connection with the server's settings, so a
// write waits out the server's own transactions up to the busy timeout.
//
// The database must already be at this binary's schema version; the server
// migrates it, this does not. The returned function closes the connection.
func OpenExisting(ctx context.Context, path string) (*DB, func() error, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, nil, fmt.Errorf("open database: %w", err)
	}
	if !info.Mode().IsRegular() {
		return nil, nil, fmt.Errorf("open database: %s is not a regular file", path)
	}
	conn, err := sql.Open(driverName, writerDSN(path))
	if err != nil {
		return nil, nil, err
	}
	conn.SetMaxOpenConns(1)
	conn.SetMaxIdleConns(1)
	fail := func(err error) (*DB, func() error, error) { return nil, nil, errors.Join(err, conn.Close()) }
	want, err := latestMigration()
	if err != nil {
		return fail(err)
	}
	var have int
	// A database the server never opened has no schema_migrations table; that
	// reads as version 0 rather than as a query error.
	var tables int
	if err := conn.QueryRowContext(ctx, "SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='schema_migrations'").Scan(&tables); err != nil {
		return fail(fmt.Errorf("read database schema: %w", err))
	}
	if tables > 0 {
		if err := conn.QueryRowContext(ctx, "SELECT COALESCE(MAX(version), 0) FROM schema_migrations").Scan(&have); err != nil {
			return fail(fmt.Errorf("read database schema version: %w", err))
		}
	}
	if have != want {
		return fail(fmt.Errorf("%w: database is at version %d, this binary expects %d; start Binnacle once with this version so it migrates the database", ErrSchemaVersion, have, want))
	}
	return NewDB(conn, conn), conn.Close, nil
}

// migrationVersion parses the numeric prefix of a migration file name.
func migrationVersion(name string) (int, error) {
	var version int
	if _, err := fmt.Sscanf(filepath.Base(name), "%03d_", &version); err != nil {
		return 0, fmt.Errorf("invalid migration name %s", filepath.Base(name))
	}
	return version, nil
}

// latestMigration is the highest version among the embedded migrations.
func latestMigration() (int, error) {
	entries, err := fs.Glob(migrations.FS(), "*.sql")
	if err != nil {
		return 0, err
	}
	latest := 0
	for _, entry := range entries {
		version, err := migrationVersion(entry)
		if err != nil {
			return 0, err
		}
		if version > latest {
			latest = version
		}
	}
	return latest, nil
}
