// SPDX-License-Identifier: AGPL-3.0-only

// Package migrations exposes the SQL schema migration files.
// The files live at the repository root so they are easy to inspect and manage.
package migrations

import "embed"

//go:embed *.sql
var files embed.FS

// FS returns the embedded migration files.
func FS() embed.FS { return files }
