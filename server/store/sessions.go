package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
)

// Starting and planning sessions, and taking a maintainer's role away (FR-SESS-1, FR-SESS-2,
// FR-SESS-5, FR-SESS-7). Ending a session and last call are actions on its queue: they go through
// Apply (queue.go).

// StartSession starts a live session (FR-SESS-1) and moves the project's carried hands into it
// as waiting hands, in their old order (FR-SESS-7). ErrSessionLive when one is live already
// (FR-SESS-2: the unique index decides, so two starts at the same moment cannot both win), and
// ErrUnknownUser when the host has no user row. The session starts at version 1.
func (s *Store) StartSession(ctx context.Context, n NewSession) (Session, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Session{}, fmt.Errorf("store: start session: %w", err)
	}
	defer tx.Rollback(ctx) // does nothing after Commit; before it, the error already being returned is the one to report

	// The session row comes first. A second start of the same project waits here for this
	// transaction and is then refused by the index, so only one start ever moves the hands below.
	// The time is the server's own, in whole microseconds, like every time of the queue.
	var id int64
	err = tx.QueryRow(ctx, `
		INSERT INTO sessions (project_id, title, planned_minutes, started_at, started_by, queue_version)
		VALUES ($1, $2, NULLIF($3, 0), $4, $5, 1)
		RETURNING id`,
		n.ProjectID, n.Title, n.PlannedMinutes, time.Now().UTC().Truncate(time.Microsecond), n.HostID).Scan(&id)
	if err != nil {
		return Session{}, sessionStartRefusal(err)
	}
	// What was carried waits again. Its place in the line (sort_at) and the time it was raised
	// stay, so these hands stand before every hand of the new session, in the order they had.
	// ponytail: no index covers carried hands, so this scans the hands table once per start.
	// Add a partial index on hands (session_id) WHERE status = 'carried' when the table holds
	// hands by the hundred thousand.
	if _, err := tx.Exec(ctx, `
		UPDATE hands h
		SET session_id = $1, status = 'waiting', closed_at = NULL, taken_at = NULL, taken_by = NULL,
		    acked_at = NULL, away_count = 0
		FROM sessions earlier
		WHERE earlier.id = h.session_id AND earlier.project_id = $2 AND h.status = 'carried'`,
		id, n.ProjectID); err != nil {
		return Session{}, fmt.Errorf("store: start session: bring the carried hands in: %w", err)
	}
	started, err := scanSession(tx.QueryRow(ctx, sessionSelect+` WHERE s.id = $1`, id))
	if err != nil {
		return Session{}, fmt.Errorf("store: start session: read it back: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Session{}, fmt.Errorf("store: start session: %w", err)
	}
	return started, nil
}

// sessionStartRefusal names the two refusals of a start that the database decides, and wraps
// every other error with what was being done.
func sessionStartRefusal(err error) error {
	const (
		uniqueViolation     = "23505"
		foreignKeyViolation = "23503"
	)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch {
		case pgErr.Code == uniqueViolation && pgErr.ConstraintName == "sessions_one_live_per_project":
			return ErrSessionLive
		case pgErr.Code == foreignKeyViolation && pgErr.ConstraintName == "sessions_started_by_fkey":
			return ErrUnknownUser
		}
	}
	return fmt.Errorf("store: start session: %w", err)
}

// SetNextSession records when the project's next session is planned (FR-SESS-5). A zero time
// clears it.
func (s *Store) SetNextSession(ctx context.Context, projectID string, at time.Time) error {
	var planned *time.Time // nil is NULL: no session is planned
	if !at.IsZero() {
		planned = &at
	}
	tag, err := s.pool.Exec(ctx, `UPDATE projects SET next_session_at = $2 WHERE id = $1`, projectID, planned)
	if err != nil {
		return fmt.Errorf("store: set next session: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return errors.New("store: set next session: no such project")
	}
	return nil
}

// RemoveMaintainer takes a maintainer's role away, when GitHub says they no longer have the
// rights that gave it to them. They stay a member of the room. Someone who is no maintainer
// stays what they are.
func (s *Store) RemoveMaintainer(ctx context.Context, projectID, userID string) error {
	if _, err := s.pool.Exec(ctx, `DELETE FROM maintainers WHERE project_id = $1 AND user_id = $2`, projectID, userID); err != nil {
		return fmt.Errorf("store: remove maintainer: %w", err)
	}
	return nil
}
