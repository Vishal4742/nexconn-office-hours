package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
)

// Grant is one agent a person connected (FR-AGENT-2, FR-AGENT-3). The row holds no token.
type Grant struct {
	ID           string
	UserID       string
	ClientName   string    // what the app called itself
	RedirectHost string    // where the consent page sent the person back
	CreatedAt    time.Time // set by the database
	LastUsedAt   time.Time // zero until the agent's first request
}

// ErrGrantExists means a grant with that id was made already: its authorization code came back
// a second time.
var ErrGrantExists = errors.New("store: the grant exists already")

// grantListMax bounds a person's list. ponytail: no paging; nobody connects a hundred agents.
const grantListMax = 100

// CreateGrant records a person's approval of an agent.
func (s *Store) CreateGrant(ctx context.Context, g Grant) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO agent_grants (id, user_id, client_name, redirect_host) VALUES ($1, $2, $3, $4)`,
		g.ID, g.UserID, g.ClientName, g.RedirectHost)
	const uniqueViolation = "23505"
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == uniqueViolation && pgErr.ConstraintName == "agent_grants_pkey" {
		return ErrGrantExists
	}
	if err != nil {
		return unknownUserOr("create grant", err)
	}
	return nil
}

// UseGrant reports whether a grant still lets its agent in: not disconnected, and its person's
// account not deleted (FR-AUTH-5). It notes the use, at most once a minute. The update in the
// WITH runs beside the SELECT, which sees the row as it was: the update never changes the answer.
func (s *Store) UseGrant(ctx context.Context, id string) (bool, error) {
	var active bool
	err := s.pool.QueryRow(ctx, `
		WITH noted AS (
			UPDATE agent_grants g SET last_used_at = now()
			FROM users u
			WHERE g.id = $1 AND u.id = g.user_id AND g.revoked_at IS NULL AND u.deleted_at IS NULL
			  AND (g.last_used_at IS NULL OR g.last_used_at < now() - interval '1 minute')
		)
		SELECT EXISTS (
			SELECT 1 FROM agent_grants g JOIN users u ON u.id = g.user_id
			WHERE g.id = $1 AND g.revoked_at IS NULL AND u.deleted_at IS NULL)`, id).Scan(&active)
	if err != nil {
		return false, fmt.Errorf("store: use grant: %w", err)
	}
	return active, nil
}

// RevokeGrantByID ends a grant whoever made it: its code came back a second time, so it may have
// been stolen (RFC 6749 §4.1.2).
func (s *Store) RevokeGrantByID(ctx context.Context, id string) error {
	if _, err := s.pool.Exec(ctx, `UPDATE agent_grants SET revoked_at = now() WHERE id = $1 AND revoked_at IS NULL`, id); err != nil {
		return fmt.Errorf("store: revoke grant: %w", err)
	}
	return nil
}

// RevokeGrant disconnects one of a person's agents (FR-AGENT-3). It reports whether there was one
// to end: the id of someone else's grant ends nothing.
func (s *Store) RevokeGrant(ctx context.Context, userID, id string) (bool, error) {
	tag, err := s.pool.Exec(ctx, `
		UPDATE agent_grants SET revoked_at = now() WHERE id = $1 AND user_id = $2 AND revoked_at IS NULL`, id, userID)
	if err != nil {
		return false, fmt.Errorf("store: disconnect grant: %w", err)
	}
	return tag.RowsAffected() == 1, nil
}

// ListGrants lists a person's connected agents, the newest first.
func (s *Store) ListGrants(ctx context.Context, userID string) ([]Grant, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, user_id, client_name, redirect_host, created_at, last_used_at FROM agent_grants
		WHERE user_id = $1 AND revoked_at IS NULL ORDER BY created_at DESC LIMIT $2`, userID, grantListMax)
	if err != nil {
		return nil, fmt.Errorf("store: list grants: %w", err)
	}
	defer rows.Close()
	var grants []Grant
	for rows.Next() {
		var g Grant
		var lastUsed *time.Time
		if err := rows.Scan(&g.ID, &g.UserID, &g.ClientName, &g.RedirectHost, &g.CreatedAt, &lastUsed); err != nil {
			return nil, fmt.Errorf("store: list grants: %w", err)
		}
		if lastUsed != nil {
			g.LastUsedAt = *lastUsed
		}
		grants = append(grants, g)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: list grants: %w", err)
	}
	return grants, nil
}
