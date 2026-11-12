// Package migrations holds the SQL files that build the database. They are applied in the order
// of their names by store.Migrate (go -C server run ./cmd/migrate). A file is never edited after it
// has reached production: a change is a new file, written so the code already running still works.
package migrations

import "embed"

//go:embed *.sql
var FS embed.FS
