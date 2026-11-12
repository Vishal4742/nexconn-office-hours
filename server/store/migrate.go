package store

import (
	"context"
	"fmt"
	"io/fs"
	"slices"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Migrate applies every .sql file in files that the database hasn't seen yet, in name order, each
// in its own transaction, and returns the names it applied.
func Migrate(ctx context.Context, pool *pgxpool.Pool, files fs.FS) ([]string, error) {
	if _, err := pool.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		name TEXT PRIMARY KEY, applied_at TIMESTAMPTZ NOT NULL DEFAULT now())`); err != nil {
		return nil, fmt.Errorf("store: migrations table: %w", err)
	}
	names, err := fs.Glob(files, "*.sql")
	if err != nil {
		return nil, fmt.Errorf("store: list migrations: %w", err)
	}
	slices.Sort(names)

	var applied []string
	for _, name := range names {
		ok, err := applyOne(ctx, pool, files, name)
		if err != nil {
			return applied, fmt.Errorf("store: migration %s: %w", name, err)
		}
		if ok {
			applied = append(applied, name)
		}
	}
	return applied, nil
}

// applyOne runs one file unless its name is already recorded. Recording the name comes first: the
// insert takes the row's lock, so a second run at the same moment waits and then skips the file.
func applyOne(ctx context.Context, pool *pgxpool.Pool, files fs.FS, name string) (bool, error) {
	sql, err := fs.ReadFile(files, name)
	if err != nil {
		return false, err
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck // a no-op once the transaction is committed

	tag, err := tx.Exec(ctx, `INSERT INTO schema_migrations (name) VALUES ($1) ON CONFLICT DO NOTHING`, name)
	if err != nil {
		return false, err
	}
	if tag.RowsAffected() == 0 {
		return false, nil
	}
	if _, err := tx.Exec(ctx, string(sql)); err != nil {
		return false, err
	}
	return true, tx.Commit(ctx)
}
