package store

import (
	"context"
	"time"
)

// Owned by the sessions worker of Phase 2 (notes/phase-2-plan.md, worker 2). The signatures are
// a contract; the bodies are theirs.

// StartSession starts a live session (FR-SESS-1) and moves the project's carried hands into it
// as waiting hands, in their old order (FR-SESS-7). ErrSessionLive when one is live already
// (FR-SESS-2: the unique index decides, so two starts at the same moment cannot both win).
func (s *Store) StartSession(ctx context.Context, n NewSession) (Session, error) {
	return Session{}, ErrNotBuilt
}

// SetNextSession records when the project's next session is planned (FR-SESS-5). A zero time
// clears it.
func (s *Store) SetNextSession(ctx context.Context, projectID string, at time.Time) error {
	return ErrNotBuilt
}

// RemoveMaintainer takes a maintainer's role away, when GitHub says they no longer have the
// rights that gave it to them. They stay a member of the room.
func (s *Store) RemoveMaintainer(ctx context.Context, projectID, userID string) error {
	return ErrNotBuilt
}
