// Package migrations embeds the SQL migration files so internal/store can
// apply them without relying on a file path that only exists at build time.
package migrations

import "embed"

// FS holds every *.sql file in this directory, applied in filename order
// (hence the numeric prefix on each file) by internal/store.
//
//go:embed *.sql
var FS embed.FS
