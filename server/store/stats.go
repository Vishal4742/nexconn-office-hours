package store

import "context"

// Owned by the FAQ-and-stats API part of the parallel build (notes/phase-2-plan.md, worker 8). The signature is a contract; the
// body is theirs.

// Stats counts the five numbers of FR-MET-1 for one project, or for the whole site when
// projectID is empty.
func (s *Store) Stats(ctx context.Context, projectID string) (Stats, error) {
	return Stats{}, ErrNotBuilt
}
