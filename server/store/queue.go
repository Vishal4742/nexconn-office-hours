package store

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"officehours/queue"
)

// The queue's persistence. The signatures are a contract (notes/phase-2-plan.md); the bodies
// arrive with the store core of Phase 2. The two reading helpers below are shared by every file
// of this package that reads a person or a session, so that none writes its own.

// personCols is the SELECT list of a Person from a users row with the given alias. The name falls
// back to the login, and a row that a LEFT JOIN did not find reads as the zero Person.
func personCols(alias string) string {
	return fmt.Sprintf("COALESCE(%[1]s.id, ''), COALESCE(%[1]s.login, ''), COALESCE(NULLIF(%[1]s.name, ''), %[1]s.login, ''), COALESCE(%[1]s.avatar_url, '')", alias)
}

// sessionSelect reads sessions as scanSession wants them. Add a WHERE on s.
var sessionSelect = `
	SELECT s.id, s.project_id,
	       (SELECT count(*) FROM sessions n WHERE n.project_id = s.project_id AND n.id <= s.id),
	       s.title, s.status = 'live', COALESCE(s.planned_minutes, 0), s.started_at, s.ended_at,
	       s.hands_closed, s.queue_version, ` + personCols("host") + `
	FROM sessions s LEFT JOIN users host ON host.id = s.started_by`

// scanSession reads one row of sessionSelect. pgx.ErrNoRows passes through.
func scanSession(row pgx.Row) (Session, error) {
	var (
		s     Session
		ended *time.Time
	)
	err := row.Scan(&s.ID, &s.ProjectID, &s.Number, &s.Title, &s.Live, &s.PlannedMinutes, &s.StartedAt, &ended,
		&s.HandsClosed, &s.Version, &s.Host.UserID, &s.Host.Login, &s.Host.Name, &s.Host.AvatarURL)
	if err != nil {
		return Session{}, err
	}
	if ended != nil {
		s.EndedAt = *ended
	}
	return s, nil
}

// Queue reads everything the room shows about a project's queue. Until the store core is built it
// answers "no session yet", which is true while no session can be started.
func (s *Store) Queue(ctx context.Context, projectID string) (Snapshot, error) {
	return Snapshot{}, nil
}

// Done finishes the hand being answered and creates its FAQ entry in the same transaction
// (FR-FAQ-1): both happen or neither. The action must be a queue.Done. The draft's slug is made
// unique in the project by adding "-2", "-3" and so on; AskedBy counts the author and the +1s.
func (s *Store) Done(ctx context.Context, sessionID int64, a queue.Action, draft FAQDraft) (Applied, FAQEntry, error) {
	return Applied{}, FAQEntry{}, ErrNotBuilt
}

// Apply does one action on a session's queue in one transaction: it locks the session, loads
// its open hands, asks queue.Step, writes what changed and raises the queue's version by one
// (FR-QUEUE-1). A refusal of Step comes back as it is and nothing is written. For a Raise it
// fills in Action.LastRaisedAt first.
func (s *Store) Apply(ctx context.Context, sessionID int64, a queue.Action) (Applied, error) {
	return Applied{}, ErrNotBuilt
}

// ProjectOfSession says which project a session belongs to.
func (s *Store) ProjectOfSession(ctx context.Context, sessionID int64) (projectID string, found bool, err error) {
	return "", false, ErrNotBuilt
}

// ProjectOfHand says which project and session a hand belongs to.
func (s *Store) ProjectOfHand(ctx context.Context, handID int64) (projectID string, sessionID int64, found bool, err error) {
	return "", 0, false, ErrNotBuilt
}

// People looks users up by id, in the order asked; unknown ids are left out.
func (s *Store) People(ctx context.Context, userIDs []string) ([]Person, error) {
	return nil, ErrNotBuilt
}
