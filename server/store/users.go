package store

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Store is every query the server runs. Handlers use it through small interfaces of their own.
type Store struct {
	pool *pgxpool.Pool
}

// New wraps a pool from Open.
func New(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

// Ping reports whether the database answers.
func (s *Store) Ping(ctx context.Context) error {
	return s.pool.Ping(ctx)
}

// User is one person, as GitHub describes them. No email address (NFR-PRIV-1).
type User struct {
	ID        string // "gh" + GitHubID; also their Nexconn user id
	GitHubID  int64
	Login     string
	Name      string // display name, never empty
	AvatarURL string
}

// UpsertUser saves who just signed in (FR-AUTH-1). A rename on GitHub updates the same row, and
// an account that was deleted here comes back.
func (s *Store) UpsertUser(ctx context.Context, u User) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO users (id, github_id, login, name, avatar_url)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (github_id) DO UPDATE
		SET login = EXCLUDED.login, name = EXCLUDED.name, avatar_url = EXCLUDED.avatar_url, deleted_at = NULL`,
		u.ID, u.GitHubID, u.Login, u.Name, u.AvatarURL)
	if err != nil {
		return fmt.Errorf("store: save user: %w", err)
	}
	return nil
}
