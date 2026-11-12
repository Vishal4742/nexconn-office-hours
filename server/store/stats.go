package store

import (
	"context"
	"fmt"
)

// Stats counts the five numbers of FR-MET-1 for one project, or for the whole site when
// projectID is empty: the sessions held (the live one too), every hand that was raised whatever
// became of it, the hands that were answered, the FAQ entries that are not deleted, and the
// people who joined a room. Someone in two rooms is one participant of the site.
//
// It is one statement, so the five numbers are of the same moment: a hand that is answered while
// it runs is in both hand counts or in neither.
//
// ponytail: it counts the tables on every request, which is nothing for thousands of rows. Cache
// the answer for a minute, or keep counters, when a stats page gets slow.
func (s *Store) Stats(ctx context.Context, projectID string) (Stats, error) {
	var n Stats
	err := s.pool.QueryRow(ctx, `
		SELECT (SELECT count(*) FROM sessions s WHERE $1 = '' OR s.project_id = $1),
		       h.raised, h.answered,
		       (SELECT count(*) FROM faq_entries f WHERE f.deleted_at IS NULL AND ($1 = '' OR f.project_id = $1)),
		       (SELECT count(DISTINCT m.user_id) FROM members m WHERE $1 = '' OR m.project_id = $1)
		FROM (
			SELECT count(*) AS raised, count(*) FILTER (WHERE h.status = 'answered') AS answered
			FROM hands h JOIN sessions s ON s.id = h.session_id
			WHERE $1 = '' OR s.project_id = $1
		) h`, projectID).Scan(&n.Sessions, &n.HandsRaised, &n.HandsAnswered, &n.FAQEntries, &n.Participants)
	if err != nil {
		return Stats{}, fmt.Errorf("store: count stats: %w", err)
	}
	return n, nil
}
