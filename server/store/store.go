// Package store owns the Postgres connection pool. Queries join it in P1.
package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Open prepares a pool sized for serverless: at most 2 connections per instance, and idle ones
// closed after 10 s so a suspended instance holds none. Point it at Neon's pooled (-pooler) host.
// It connects on first use, not here.
func Open(ctx context.Context, databaseURL string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		// The parse error is not wrapped: it can quote the URL, and the URL holds the password.
		return nil, errors.New("store: DATABASE_URL is not a valid Postgres URL")
	}
	cfg.MaxConns = 2
	cfg.MaxConnIdleTime = 10 * time.Second
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("store: open pool: %w", err)
	}
	return pool, nil
}
