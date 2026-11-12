package store

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"officehours/queue"
)

// The queue's persistence (FR-QUEUE-1): one function reads what a room shows, and one writes a
// change, after asking the rules. The exported signatures are a contract
// (notes/phase-2-plan.md). The two reading helpers below are shared by every file of this
// package that reads a person or a session, so that none writes its own.

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
	s.StartedAt = s.StartedAt.UTC()
	if ended != nil {
		s.EndedAt = ended.UTC()
	}
	return s, nil
}

// ErrNoSession means a queue action named a session that does not exist.
var ErrNoSession = errors.New("store: no such session")

// handSelect reads hands as handRow wants them. A hand's +1s come sorted by user id: they are a
// set, and the table keeps no order for them.
const handSelect = `
	SELECT h.id, h.author_id, h.title, h.details, h.status, h.raised_at, h.sort_at, h.taken_at,
	       COALESCE(h.taken_by, ''), h.closed_at, h.acked_at, h.away_count, COALESCE(h.merged_into, 0),
	       COALESCE((SELECT array_agg(p.user_id ORDER BY p.user_id) FROM hand_plus p WHERE p.hand_id = h.id), '{}')`

// handRow is one row of handSelect on its way to a queue.Hand.
type handRow struct {
	hand                 queue.Hand
	status               string
	taken, closed, acked *time.Time
}

// dest lists the Scan targets for handSelect's columns, in their order.
func (r *handRow) dest() []any {
	return []any{&r.hand.ID, &r.hand.AuthorID, &r.hand.Title, &r.hand.Details, &r.status, &r.hand.RaisedAt, &r.hand.SortAt,
		&r.taken, &r.hand.TakenBy, &r.closed, &r.acked, &r.hand.AwayCount, &r.hand.MergedInto, &r.hand.Plus}
}

// done finishes the hand after the scan: a NULL time is the zero time, and every time is UTC.
func (r *handRow) done() queue.Hand {
	h := r.hand
	h.Status = queue.Status(r.status)
	h.RaisedAt, h.SortAt = h.RaisedAt.UTC(), h.SortAt.UTC()
	h.TakenAt, h.ClosedAt, h.AckedAt = timeOf(r.taken), timeOf(r.closed), timeOf(r.acked)
	return h
}

func timeOf(t *time.Time) time.Time {
	if t == nil {
		return time.Time{}
	}
	return t.UTC()
}

// The zero value of a field is NULL in its column.
func nullTime(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}

func nullText(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func nullID(id int64) *int64 {
	if id == 0 {
		return nil
	}
	return &id
}

// Queue reads everything the room shows about a project's queue, in one consistent view: the
// live session, or else the last one that ended, with all of its hands.
func (s *Store) Queue(ctx context.Context, projectID string) (Snapshot, error) {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return Snapshot{}, fmt.Errorf("store: queue: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // nothing was written; the connection goes back either way

	var (
		snap Snapshot
		next *time.Time
	)
	// A planned time that has passed is no plan any more (FR-SESS-5): that session began or was
	// missed, and nobody should be told to wait for it.
	err = tx.QueryRow(ctx, `SELECT next_session_at FROM projects WHERE id = $1 AND next_session_at > now()`, projectID).Scan(&next)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return Snapshot{}, fmt.Errorf("store: queue: %w", err)
	}
	snap.NextSessionAt = timeOf(next)

	session, err := scanSession(tx.QueryRow(ctx, sessionSelect+`
		WHERE s.project_id = $1 ORDER BY (s.status = 'live') DESC, s.id DESC LIMIT 1`, projectID))
	if errors.Is(err, pgx.ErrNoRows) {
		return snap, nil // the project never had a session
	}
	if err != nil {
		return Snapshot{}, fmt.Errorf("store: queue: session: %w", err)
	}
	snap.Session = &session

	rows, err := tx.Query(ctx, handSelect+`, `+personCols("author")+`, `+personCols("taker")+`, COALESCE(f.id, 0), COALESCE(f.slug, '')
		FROM hands h
		JOIN users author ON author.id = h.author_id
		LEFT JOIN users taker ON taker.id = h.taken_by
		LEFT JOIN faq_entries f ON f.hand_id = h.id AND f.deleted_at IS NULL
		WHERE h.session_id = $1`, session.ID)
	if err != nil {
		return Snapshot{}, fmt.Errorf("store: queue: hands: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var (
			row handRow
			h   QueueHand
		)
		dest := append(row.dest(),
			&h.Author.UserID, &h.Author.Login, &h.Author.Name, &h.Author.AvatarURL,
			&h.Taker.UserID, &h.Taker.Login, &h.Taker.Name, &h.Taker.AvatarURL, &h.FAQID, &h.FAQSlug)
		if err := rows.Scan(dest...); err != nil {
			return Snapshot{}, fmt.Errorf("store: queue: hands: %w", err)
		}
		h.Hand = row.done()
		if h.Status.IsOpen() {
			snap.Open = append(snap.Open, h)
		} else {
			snap.Closed = append(snap.Closed, h)
		}
	}
	if err := rows.Err(); err != nil {
		return Snapshot{}, fmt.Errorf("store: queue: hands: %w", err)
	}

	// The hand being answered first, then the waiting ones in the queue's order (FR-QUEUE-1).
	slices.SortFunc(snap.Open, func(a, b QueueHand) int {
		if a.Status != b.Status {
			if a.Status == queue.Answering {
				return -1
			}
			if b.Status == queue.Answering {
				return 1
			}
		}
		return cmp.Or(a.SortAt.Compare(b.SortAt), cmp.Compare(a.ID, b.ID))
	})
	// The rest: what was closed last comes first.
	slices.SortFunc(snap.Closed, func(a, b QueueHand) int {
		return cmp.Or(b.ClosedAt.Compare(a.ClosedAt), cmp.Compare(b.ID, a.ID))
	})
	snap.MedianAnswer = medianAnswer(snap.Closed)
	return snap, nil
}

// medianAnswer is the median time from Next to Done over a session's answered hands, or zero
// until two were answered (FR-QUEUE-10).
func medianAnswer(closed []QueueHand) time.Duration {
	var took []time.Duration
	for _, h := range closed {
		if h.Status == queue.Answered && !h.TakenAt.IsZero() && !h.ClosedAt.Before(h.TakenAt) {
			took = append(took, h.ClosedAt.Sub(h.TakenAt))
		}
	}
	if len(took) < 2 {
		return 0
	}
	slices.Sort(took)
	mid := len(took) / 2
	if len(took)%2 == 1 {
		return took[mid]
	}
	return (took[mid-1] + took[mid]) / 2
}

// stepFunc is queue.Step, or what a test puts in its place.
type stepFunc func(queue.State, queue.Action) (queue.State, error)

// Apply does one action on a session's queue in one transaction: it locks the session, loads
// its open hands, asks queue.Step, writes what changed and raises the queue's version by one
// (FR-QUEUE-1). A refusal of Step comes back as it is and nothing is written. For a Raise it
// fills in Action.LastRaisedAt first.
func (s *Store) Apply(ctx context.Context, sessionID int64, a queue.Action) (Applied, error) {
	return s.applyWith(ctx, queue.Step, sessionID, a, nil)
}

// applyWith is Apply with the rules given by the caller, and with an extra write that happens in
// the same transaction once the queue is written (Done's FAQ entry).
func (s *Store) applyWith(ctx context.Context, step stepFunc, sessionID int64, a queue.Action,
	extra func(ctx context.Context, tx pgx.Tx, ap Applied) error) (Applied, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Applied{}, fmt.Errorf("store: apply: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // a no-op after Commit; before it, the rollback is the point

	// The lock makes two actions on one session wait for each other, so each sees what the one
	// before it wrote.
	ap := Applied{SessionID: sessionID}
	err = tx.QueryRow(ctx, `SELECT project_id, status = 'live', hands_closed FROM sessions WHERE id = $1 FOR UPDATE`, sessionID).
		Scan(&ap.ProjectID, &ap.Before.Live, &ap.Before.HandsClosed)
	if errors.Is(err, pgx.ErrNoRows) {
		return Applied{}, ErrNoSession
	}
	if err != nil {
		return Applied{}, fmt.Errorf("store: apply: session: %w", err)
	}
	if ap.Before.Hands, err = openHands(ctx, tx, sessionID); err != nil {
		return Applied{}, err
	}
	if a.Kind == queue.Raise {
		// FR-HAND-6: the author's latest hand in this project, in any session.
		var last *time.Time
		err := tx.QueryRow(ctx, `
			SELECT max(h.raised_at) FROM hands h JOIN sessions s ON s.id = h.session_id
			WHERE s.project_id = $1 AND h.author_id = $2`, ap.ProjectID, a.ActorID).Scan(&last)
		if err != nil {
			return Applied{}, fmt.Errorf("store: apply: last raise: %w", err)
		}
		a.LastRaisedAt = timeOf(last)
	}

	after, err := step(ap.Before, a)
	if err != nil {
		return Applied{}, err // the rules' own refusal, as it is
	}
	if err := writeHands(ctx, tx, sessionID, ap.Before.Hands, after.Hands); err != nil {
		return Applied{}, err
	}
	err = tx.QueryRow(ctx, `
		UPDATE sessions
		SET queue_version = queue_version + 1, hands_closed = $2,
		    status   = CASE WHEN $3 THEN status ELSE 'ended' END,
		    ended_at = CASE WHEN $3 THEN ended_at ELSE $4 END
		WHERE id = $1 RETURNING queue_version`, sessionID, after.HandsClosed, after.Live, a.At).Scan(&ap.Version)
	if err != nil {
		return Applied{}, fmt.Errorf("store: apply: session: %w", err)
	}
	ap.After = after
	if extra != nil {
		if err := extra(ctx, tx, ap); err != nil {
			return Applied{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return Applied{}, fmt.Errorf("store: apply: commit: %w", err)
	}
	return ap, nil
}

// openHands loads the hands of a session that are still in the queue.
func openHands(ctx context.Context, tx pgx.Tx, sessionID int64) ([]queue.Hand, error) {
	rows, err := tx.Query(ctx, handSelect+` FROM hands h WHERE h.session_id = $1 AND h.status IN ('waiting', 'answering') ORDER BY h.sort_at, h.id`, sessionID)
	if err != nil {
		return nil, fmt.Errorf("store: apply: hands: %w", err)
	}
	defer rows.Close()
	var hands []queue.Hand
	for rows.Next() {
		var row handRow
		if err := rows.Scan(row.dest()...); err != nil {
			return nil, fmt.Errorf("store: apply: hands: %w", err)
		}
		hands = append(hands, row.done())
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: apply: hands: %w", err)
	}
	return hands, nil
}

// writeHands writes the difference between the hands the rules got and the hands they returned.
// A hand with id 0 is new and gets its id here, in after. The rules return every hand they got:
// anything else is a bug in them, and nothing is written.
func writeHands(ctx context.Context, tx pgx.Tx, sessionID int64, before, after []queue.Hand) error {
	was := make(map[int64]queue.Hand, len(before))
	for _, h := range before {
		was[h.ID] = h
	}
	seen := 0
	for i, h := range after {
		if h.ID == 0 {
			err := tx.QueryRow(ctx, `
				INSERT INTO hands (session_id, author_id, title, details, status, raised_at, sort_at)
				VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING id`,
				sessionID, h.AuthorID, h.Title, h.Details, string(h.Status), h.RaisedAt, h.SortAt).Scan(&after[i].ID)
			if err != nil {
				return writeRefusal("raise", err)
			}
			if err := writePlus(ctx, tx, after[i].ID, nil, h.Plus); err != nil {
				return err
			}
			continue
		}
		old, known := was[h.ID]
		if !known {
			return fmt.Errorf("store: apply: the rules returned hand %d, which they were not given", h.ID)
		}
		seen++
		if handChanged(old, h) {
			_, err := tx.Exec(ctx, `
				UPDATE hands SET status = $2, sort_at = $3, taken_at = $4, taken_by = $5, closed_at = $6,
				                 acked_at = $7, away_count = $8, merged_into = $9
				WHERE id = $1`,
				h.ID, string(h.Status), h.SortAt, nullTime(h.TakenAt), nullText(h.TakenBy), nullTime(h.ClosedAt),
				nullTime(h.AckedAt), h.AwayCount, nullID(h.MergedInto))
			if err != nil {
				return writeRefusal("write hand", err)
			}
		}
		if err := writePlus(ctx, tx, h.ID, old.Plus, h.Plus); err != nil {
			return err
		}
	}
	if seen != len(before) {
		return fmt.Errorf("store: apply: the rules returned %d of the %d hands they were given", seen, len(before))
	}
	return nil
}

// handChanged reports whether anything the rules may change about a hand differs. Its author,
// words and raise time never change.
func handChanged(a, b queue.Hand) bool {
	return a.Status != b.Status || !a.SortAt.Equal(b.SortAt) || !a.TakenAt.Equal(b.TakenAt) || a.TakenBy != b.TakenBy ||
		!a.ClosedAt.Equal(b.ClosedAt) || !a.AckedAt.Equal(b.AckedAt) || a.AwayCount != b.AwayCount || a.MergedInto != b.MergedInto
}

// writePlus makes a hand's +1 rows match the set the rules returned (FR-HAND-7, FR-HAND-8).
func writePlus(ctx context.Context, tx pgx.Tx, handID int64, before, after []string) error {
	for _, userID := range after {
		if slices.Contains(before, userID) {
			continue
		}
		if _, err := tx.Exec(ctx, `INSERT INTO hand_plus (hand_id, user_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`, handID, userID); err != nil {
			return writeRefusal("add +1", err)
		}
	}
	for _, userID := range before {
		if slices.Contains(after, userID) {
			continue
		}
		if _, err := tx.Exec(ctx, `DELETE FROM hand_plus WHERE hand_id = $1 AND user_id = $2`, handID, userID); err != nil {
			return fmt.Errorf("store: apply: take +1 back: %w", err)
		}
	}
	return nil
}

// writeRefusal names what the database refused. The unique indexes are the last guard behind the
// rules (ARCHITECTURE §3), so their refusals read like the rules' own; a user without a row is
// the store's usual ErrUnknownUser.
func writeRefusal(op string, err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch {
		case pgErr.ConstraintName == "hands_one_open_per_author":
			return queue.ErrAlreadyOpen
		case pgErr.ConstraintName == "hands_one_answering_per_session":
			return queue.ErrAnswering
		case pgErr.Code == "23503" && slices.Contains([]string{"hands_author_id_fkey", "hands_taken_by_fkey", "hand_plus_user_id_fkey", "faq_entries_author_id_fkey"}, pgErr.ConstraintName):
			return ErrUnknownUser
		}
	}
	return fmt.Errorf("store: apply: %s: %w", op, err)
}

// maxSlug is the longest slug a FAQ page's address takes (slugRe in the server's validate.go).
const maxSlug = 80

var slugShape = regexp.MustCompile(`^[a-z0-9-]{1,80}$`)

// Done finishes the hand being answered and creates its FAQ entry in the same transaction
// (FR-FAQ-1): both happen or neither. The action must be a queue.Done. The draft's slug is made
// unique in the project by adding "-2", "-3" and so on; AskedBy counts the author and the +1s.
func (s *Store) Done(ctx context.Context, sessionID int64, a queue.Action, draft FAQDraft) (Applied, FAQEntry, error) {
	return s.doneWith(ctx, queue.Step, sessionID, a, draft)
}

func (s *Store) doneWith(ctx context.Context, step stepFunc, sessionID int64, a queue.Action, draft FAQDraft) (Applied, FAQEntry, error) {
	if a.Kind != queue.Done || a.HandID == 0 {
		return Applied{}, FAQEntry{}, errors.New("store: done: the action must be a queue.Done on one hand")
	}
	if !slugShape.MatchString(draft.Slug) {
		return Applied{}, FAQEntry{}, fmt.Errorf("store: done: %q cannot be a slug", draft.Slug)
	}
	links := draft.Links
	if links == nil {
		links = []string{}
	}
	linksJSON, err := json.Marshal(links)
	if err != nil {
		return Applied{}, FAQEntry{}, fmt.Errorf("store: done: links: %w", err)
	}

	var entry FAQEntry
	ap, err := s.applyWith(ctx, step, sessionID, a, func(ctx context.Context, tx pgx.Tx, ap Applied) error {
		askedBy := 1 // the author
		for _, h := range ap.Before.Hands {
			if h.ID == a.HandID {
				askedBy += len(h.Plus) // FR-HAND-8: the +1s, and with them everyone who was merged in
			}
		}
		slug, err := freeSlug(ctx, tx, ap.ProjectID, draft.Slug)
		if err != nil {
			return err
		}
		entry = FAQEntry{ProjectID: ap.ProjectID, HandID: a.HandID, Slug: slug, Title: draft.Title, Question: draft.Question,
			Answer: draft.Answer, Links: links, AskedBy: askedBy, CreatedAt: a.At.UTC()}
		err = tx.QueryRow(ctx, `
			INSERT INTO faq_entries (project_id, hand_id, slug, title, question, answer, links, asked_by, author_id, created_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7::jsonb, $8, $9, $10) RETURNING id`,
			entry.ProjectID, entry.HandID, entry.Slug, entry.Title, entry.Question, entry.Answer, string(linksJSON),
			entry.AskedBy, draft.AuthorID, entry.CreatedAt).Scan(&entry.ID)
		if err != nil {
			return writeRefusal("faq entry", err)
		}
		err = tx.QueryRow(ctx, `SELECT `+personCols("u")+` FROM users u WHERE u.id = $1`, draft.AuthorID).
			Scan(&entry.Author.UserID, &entry.Author.Login, &entry.Author.Name, &entry.Author.AvatarURL)
		if err != nil {
			return fmt.Errorf("store: done: author: %w", err)
		}
		return nil
	})
	if err != nil {
		return Applied{}, FAQEntry{}, err
	}
	return ap, entry, nil
}

// freeSlug returns base, or base with the first of "-2", "-3", … that no entry of the project
// has, deleted ones included: an address that answered once is never given to another answer.
// The whole stays within maxSlug. Dones of one project come one after the other (they hold the
// lock of its one live session), so looking first is enough; the unique index is the guard.
func freeSlug(ctx context.Context, tx pgx.Tx, projectID, base string) (string, error) {
	const tries = 200
	for n := 1; n <= tries; n++ {
		slug := base
		if n > 1 {
			suffix := "-" + strconv.Itoa(n)
			if len(slug)+len(suffix) > maxSlug {
				slug = strings.TrimRight(slug[:maxSlug-len(suffix)], "-")
			}
			slug += suffix
		}
		var taken bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM faq_entries WHERE project_id = $1 AND slug = $2)`, projectID, slug).Scan(&taken); err != nil {
			return "", fmt.Errorf("store: done: slug: %w", err)
		}
		if !taken {
			return slug, nil
		}
	}
	return "", fmt.Errorf("store: done: no free slug after %d tries for %q", tries, base)
}

// ProjectOfSession says which project a session belongs to.
func (s *Store) ProjectOfSession(ctx context.Context, sessionID int64) (projectID string, found bool, err error) {
	err = s.pool.QueryRow(ctx, `SELECT project_id FROM sessions WHERE id = $1`, sessionID).Scan(&projectID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("store: project of session: %w", err)
	}
	return projectID, true, nil
}

// ProjectOfHand says which project and session a hand belongs to.
func (s *Store) ProjectOfHand(ctx context.Context, handID int64) (projectID string, sessionID int64, found bool, err error) {
	err = s.pool.QueryRow(ctx, `
		SELECT s.project_id, h.session_id FROM hands h JOIN sessions s ON s.id = h.session_id WHERE h.id = $1`, handID).
		Scan(&projectID, &sessionID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", 0, false, nil
	}
	if err != nil {
		return "", 0, false, fmt.Errorf("store: project of hand: %w", err)
	}
	return projectID, sessionID, true, nil
}

// People looks users up by id, in the order asked; unknown ids are left out, and an id asked
// twice comes back once.
func (s *Store) People(ctx context.Context, userIDs []string) ([]Person, error) {
	if len(userIDs) == 0 {
		return nil, nil
	}
	rows, err := s.pool.Query(ctx, `SELECT `+personCols("u")+` FROM users u WHERE u.id = ANY($1)`, userIDs)
	if err != nil {
		return nil, fmt.Errorf("store: people: %w", err)
	}
	defer rows.Close()
	known := make(map[string]Person, len(userIDs))
	for rows.Next() {
		var p Person
		if err := rows.Scan(&p.UserID, &p.Login, &p.Name, &p.AvatarURL); err != nil {
			return nil, fmt.Errorf("store: people: %w", err)
		}
		known[p.UserID] = p
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: people: %w", err)
	}
	out := make([]Person, 0, len(known))
	for _, id := range userIDs {
		if p, ok := known[id]; ok {
			out = append(out, p)
			delete(known, id)
		}
	}
	return out, nil
}
