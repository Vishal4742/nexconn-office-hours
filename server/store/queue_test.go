package store

import (
	"context"
	"errors"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"officehours/queue"
)

// The moment the queue tests happen at, in whole microseconds like every time the server writes.
var queueAt = time.Date(2026, 11, 14, 11, 30, 0, 0, time.UTC)

// queueDB is a database with three people (gh1 mia, gh2 diego, gh3 ana), the project "aaaa" and
// one live session of it, hosted by mia.
func queueDB(t *testing.T) (*Store, *pgxpool.Pool, int64) {
	t.Helper()
	pool := testDB(t)
	people(t, pool)
	mustExec(t, pool, `INSERT INTO projects (id, github_id, owner, repo, created_by) VALUES ('aaaa', 11, 'PaperPlane', 'Stash', 'gh1')`)
	return New(pool), pool, queueSession(t, pool, "aaaa", "live")
}

func queueSession(t *testing.T, pool *pgxpool.Pool, projectID, status string) int64 {
	t.Helper()
	var id int64
	err := pool.QueryRow(context.Background(), `
		INSERT INTO sessions (project_id, title, status, started_by, started_at, ended_at)
		VALUES ($1, 'Thursday office hours', $2, 'gh1', $3, CASE WHEN $2 = 'ended' THEN $3::timestamptz + interval '1 hour' END)
		RETURNING id`, projectID, status, queueAt).Scan(&id)
	if err != nil {
		t.Fatalf("insert session: %v", err)
	}
	return id
}

// queueHand puts a hand into a session the way a raise would have, raised offset after queueAt.
func queueHand(t *testing.T, pool *pgxpool.Pool, sessionID int64, authorID, status string, offset time.Duration) int64 {
	t.Helper()
	var id int64
	err := pool.QueryRow(context.Background(), `
		INSERT INTO hands (session_id, author_id, title, details, status, raised_at, sort_at)
		VALUES ($1, $2, 'How do I retry only on 5xx?', 'The docs show retry: 3.', $3, $4, $4) RETURNING id`,
		sessionID, authorID, status, queueAt.Add(offset)).Scan(&id)
	if err != nil {
		t.Fatalf("insert hand: %v", err)
	}
	return id
}

// raising is the rules of a raise and nothing more: the actor's hand joins the queue.
func raising(s queue.State, a queue.Action) (queue.State, error) {
	hands := append(slices.Clone(s.Hands), queue.Hand{AuthorID: a.ActorID, Title: a.Title, Details: a.Details, Status: queue.Waiting, RaisedAt: a.At, SortAt: a.At})
	return queue.State{Live: s.Live, HandsClosed: s.HandsClosed, Hands: hands}, nil
}

// changing gives rules that change one hand of the state and keep the rest.
func changing(handID int64, change func(h *queue.Hand)) stepFunc {
	return func(s queue.State, _ queue.Action) (queue.State, error) {
		hands := slices.Clone(s.Hands)
		for i := range hands {
			if hands[i].ID == handID {
				change(&hands[i])
			}
		}
		return queue.State{Live: s.Live, HandsClosed: s.HandsClosed, Hands: hands}, nil
	}
}

func TestApplyWritesWhatTheRulesDecided(t *testing.T) {
	ctx := context.Background()

	t.Run("FR-QUEUE-1: a raised hand gets its id, and the queue's version goes up by one", func(t *testing.T) {
		s, pool, session := queueDB(t)
		ap, err := s.applyWith(ctx, raising, session, queue.Action{Kind: queue.Raise, ActorID: "gh2", At: queueAt, Title: "Can the cache be shared?", Details: "Between tabs."}, nil)
		if err != nil {
			t.Fatalf("applyWith: %v", err)
		}
		if ap.ProjectID != "aaaa" || ap.SessionID != session || ap.Version != 1 || !ap.Before.Live || len(ap.Before.Hands) != 0 || len(ap.After.Hands) != 1 {
			t.Fatalf("applied = %+v", ap)
		}
		h := ap.After.Hands[0]
		if h.ID == 0 || h.AuthorID != "gh2" || h.Status != queue.Waiting {
			t.Fatalf("the raised hand = %+v, want one with an id", h)
		}
		var title, details, status string
		var raised, sorted time.Time
		if err := pool.QueryRow(ctx, `SELECT title, details, status, raised_at, sort_at FROM hands WHERE id = $1`, h.ID).Scan(&title, &details, &status, &raised, &sorted); err != nil {
			t.Fatalf("read the hand back: %v", err)
		}
		if title != "Can the cache be shared?" || details != "Between tabs." || status != "waiting" || !raised.Equal(queueAt) || !sorted.Equal(queueAt) {
			t.Errorf("row = %q %q %q %v %v", title, details, status, raised, sorted)
		}
		// A second action sees the first hand and gets the next version.
		ap, err = s.applyWith(ctx, raising, session, queue.Action{Kind: queue.Raise, ActorID: "gh3", At: queueAt.Add(time.Minute), Title: "Is there a way to revalidate?"}, nil)
		if err != nil || ap.Version != 2 || len(ap.Before.Hands) != 1 || ap.Before.Hands[0].ID != h.ID {
			t.Fatalf("second action: %+v, %v", ap, err)
		}
	})

	t.Run("the rules see the session, its open hands with their +1s, and nothing that is closed", func(t *testing.T) {
		s, pool, session := queueDB(t)
		answering := queueHand(t, pool, session, "gh2", "answering", 2*time.Minute)
		waiting := queueHand(t, pool, session, "gh3", "waiting", time.Minute)
		queueHand(t, pool, session, "gh1", "answered", 0)
		mustExec(t, pool, `UPDATE hands SET taken_at = $2, taken_by = 'gh1', acked_at = $3, away_count = 1 WHERE id = $1`, answering, queueAt.Add(3*time.Minute), queueAt.Add(4*time.Minute))
		mustExec(t, pool, `INSERT INTO hand_plus (hand_id, user_id) VALUES ($1, 'gh2'), ($1, 'gh1')`, waiting)
		mustExec(t, pool, `UPDATE sessions SET hands_closed = true WHERE id = $1`, session)

		var got queue.State
		_, err := s.applyWith(ctx, func(state queue.State, _ queue.Action) (queue.State, error) {
			got = state
			return state, nil
		}, session, queue.Action{Kind: queue.Here, ActorID: "gh2", At: queueAt}, nil)
		if err != nil {
			t.Fatalf("applyWith: %v", err)
		}
		if !got.Live || !got.HandsClosed || len(got.Hands) != 2 {
			t.Fatalf("state = %+v, want a live session with hands closed and two open hands", got)
		}
		first, second := got.Hands[0], got.Hands[1] // in the queue's order: by sort_at
		if first.ID != waiting || first.Status != queue.Waiting || !slices.Equal(first.Plus, []string{"gh1", "gh2"}) || !first.TakenAt.IsZero() || first.TakenBy != "" {
			t.Errorf("the waiting hand = %+v, want its +1s sorted by user id and no taker", first)
		}
		if second.ID != answering || second.Status != queue.Answering || second.TakenBy != "gh1" || !second.TakenAt.Equal(queueAt.Add(3*time.Minute)) ||
			!second.AckedAt.Equal(queueAt.Add(4*time.Minute)) || second.AwayCount != 1 || second.Title == "" || !second.RaisedAt.Equal(queueAt.Add(2*time.Minute)) {
			t.Errorf("the hand being answered = %+v", second)
		}
	})

	t.Run("FR-HAND-6: a raise is told when its author last raised a hand in the project, in any session", func(t *testing.T) {
		s, pool, session := queueDB(t)
		earlier := queueSession(t, pool, "aaaa", "ended")
		queueHand(t, pool, earlier, "gh2", "withdrawn", -50*time.Second)
		queueHand(t, pool, earlier, "gh2", "answered", -10*time.Minute)
		mustExec(t, pool, `INSERT INTO projects (id, github_id, owner, repo, created_by) VALUES ('bbbb', 12, 'Other', 'Repo', 'gh1')`)
		queueHand(t, pool, queueSession(t, pool, "bbbb", "live"), "gh2", "waiting", -5*time.Second)

		lastSeen := func(kind queue.Kind, actor string) time.Time {
			var last time.Time
			_, err := s.applyWith(ctx, func(state queue.State, a queue.Action) (queue.State, error) {
				last = a.LastRaisedAt
				return state, nil
			}, session, queue.Action{Kind: kind, ActorID: actor, At: queueAt}, nil)
			if err != nil {
				t.Fatalf("applyWith: %v", err)
			}
			return last
		}
		if got := lastSeen(queue.Raise, "gh2"); !got.Equal(queueAt.Add(-50 * time.Second)) {
			t.Errorf("LastRaisedAt = %v, want the withdrawn hand of 50 s ago (another project's hand does not count)", got)
		}
		if got := lastSeen(queue.Raise, "gh3"); !got.IsZero() {
			t.Errorf("LastRaisedAt for someone who never raised = %v, want zero", got)
		}
		if got := lastSeen(queue.Withdraw, "gh2"); !got.IsZero() {
			t.Errorf("LastRaisedAt on a withdraw = %v, want zero: only a raise asks", got)
		}
	})

	t.Run("FR-QUEUE-2: what the rules change about a hand is written, and what they clear becomes NULL", func(t *testing.T) {
		s, pool, session := queueDB(t)
		hand := queueHand(t, pool, session, "gh2", "waiting", 0)
		other := queueHand(t, pool, session, "gh3", "waiting", time.Minute)
		taken := queueAt.Add(5 * time.Minute)
		step := changing(hand, func(h *queue.Hand) {
			h.Status, h.TakenAt, h.TakenBy, h.AckedAt, h.AwayCount = queue.Answering, taken, "gh1", taken.Add(time.Second), 2
		})
		if _, err := s.applyWith(ctx, step, session, queue.Action{Kind: queue.Next, ActorID: "gh1", At: taken}, nil); err != nil {
			t.Fatalf("take: %v", err)
		}
		var status, takenBy string
		var takenAt, ackedAt time.Time
		var away int
		if err := pool.QueryRow(ctx, `SELECT status, taken_at, taken_by, acked_at, away_count FROM hands WHERE id = $1`, hand).Scan(&status, &takenAt, &takenBy, &ackedAt, &away); err != nil {
			t.Fatalf("read back: %v", err)
		}
		if status != "answering" || !takenAt.Equal(taken) || takenBy != "gh1" || !ackedAt.Equal(taken.Add(time.Second)) || away != 2 {
			t.Errorf("after the take: %q %v %q %v %d", status, takenAt, takenBy, ackedAt, away)
		}

		// Back to waiting behind the other hand (Away), then merged into it.
		step = changing(hand, func(h *queue.Hand) {
			h.Status, h.TakenAt, h.TakenBy, h.AckedAt, h.SortAt = queue.Waiting, time.Time{}, "", time.Time{}, queueAt.Add(2*time.Minute)
		})
		if _, err := s.applyWith(ctx, step, session, queue.Action{Kind: queue.Away, At: taken}, nil); err != nil {
			t.Fatalf("away: %v", err)
		}
		if n := count(t, pool, `SELECT count(*) FROM hands WHERE id = $1 AND status = 'waiting' AND taken_at IS NULL AND taken_by IS NULL AND acked_at IS NULL AND sort_at = $2`, hand, queueAt.Add(2*time.Minute)); n != 1 {
			t.Error("after Away the hand must wait again with its taker and times cleared and its new place")
		}
		step = changing(hand, func(h *queue.Hand) { h.Status, h.MergedInto, h.ClosedAt = queue.Merged, other, taken })
		if _, err := s.applyWith(ctx, step, session, queue.Action{Kind: queue.Merge, At: taken}, nil); err != nil {
			t.Fatalf("merge: %v", err)
		}
		if n := count(t, pool, `SELECT count(*) FROM hands WHERE id = $1 AND status = 'merged' AND merged_into = $2 AND closed_at = $3`, hand, other, taken); n != 1 {
			t.Error("after the merge the hand must be closed and point at the hand it joined")
		}
		if n := count(t, pool, `SELECT count(*) FROM hands WHERE id = $1 AND status = 'waiting' AND closed_at IS NULL`, other); n != 1 {
			t.Error("the hand the rules did not change must be as it was")
		}
	})

	t.Run("FR-HAND-7: +1s are added and taken back, and never touch the order", func(t *testing.T) {
		s, pool, session := queueDB(t)
		hand := queueHand(t, pool, session, "gh2", "waiting", 0)
		plus := func(ids ...string) {
			t.Helper()
			if _, err := s.applyWith(ctx, changing(hand, func(h *queue.Hand) { h.Plus = ids }), session, queue.Action{Kind: queue.Plus, At: queueAt}, nil); err != nil {
				t.Fatalf("applyWith: %v", err)
			}
		}
		rows := func() int { return count(t, pool, `SELECT count(*) FROM hand_plus WHERE hand_id = $1`, hand) }
		plus("gh3", "gh1")
		if rows() != 2 {
			t.Fatalf("%d +1 rows, want 2", rows())
		}
		plus("gh1")
		if rows() != 1 || count(t, pool, `SELECT count(*) FROM hand_plus WHERE hand_id = $1 AND user_id = 'gh1'`, hand) != 1 {
			t.Fatalf("after one was taken back: %d rows, want only gh1's", rows())
		}
		plus()
		if rows() != 0 {
			t.Fatalf("%d +1 rows, want none", rows())
		}
		if n := count(t, pool, `SELECT count(*) FROM hands WHERE id = $1 AND sort_at = raised_at`, hand); n != 1 {
			t.Error("a +1 moved the hand in the queue")
		}
	})

	t.Run("a refusal of the rules comes back as it is, and nothing is written", func(t *testing.T) {
		s, pool, session := queueDB(t)
		hand := queueHand(t, pool, session, "gh2", "waiting", 0)
		refusal := &queue.TooSoonError{Wait: 40 * time.Second}
		_, err := s.applyWith(ctx, func(queue.State, queue.Action) (queue.State, error) { return queue.State{}, refusal }, session, queue.Action{Kind: queue.Raise, ActorID: "gh2", At: queueAt}, nil)
		if err != refusal {
			t.Fatalf("err = %v, want the rules' own error value", err)
		}
		if n := count(t, pool, `SELECT queue_version FROM sessions WHERE id = $1`, session); n != 0 {
			t.Errorf("the version is %d after a refusal, want 0", n)
		}
		if n := count(t, pool, `SELECT count(*) FROM hands WHERE id = $1 AND status = 'waiting'`, hand); n != 1 {
			t.Error("a refusal changed a hand")
		}
	})

	t.Run("FR-SESS-4, FR-SESS-6: last call and the end of the session are written with the action", func(t *testing.T) {
		s, pool, session := queueDB(t)
		hand := queueHand(t, pool, session, "gh2", "waiting", 0)
		closeHands := func(state queue.State, a queue.Action) (queue.State, error) {
			state.HandsClosed = a.Closed
			return state, nil
		}
		if _, err := s.applyWith(ctx, closeHands, session, queue.Action{Kind: queue.LastCall, Closed: true, At: queueAt}, nil); err != nil {
			t.Fatalf("last call: %v", err)
		}
		if n := count(t, pool, `SELECT count(*) FROM sessions WHERE id = $1 AND hands_closed AND status = 'live' AND ended_at IS NULL`, session); n != 1 {
			t.Error("last call must close hands and leave the session live")
		}
		ended := queueAt.Add(45 * time.Minute)
		end := func(state queue.State, a queue.Action) (queue.State, error) {
			out, _ := changing(hand, func(h *queue.Hand) { h.Status, h.ClosedAt = queue.Carried, a.At })(state, a)
			out.Live = false
			return out, nil
		}
		ap, err := s.applyWith(ctx, end, session, queue.Action{Kind: queue.End, Carry: true, At: ended}, nil)
		if err != nil || ap.Version != 2 || ap.After.Live {
			t.Fatalf("end: %+v, %v", ap, err)
		}
		if n := count(t, pool, `SELECT count(*) FROM sessions WHERE id = $1 AND status = 'ended' AND ended_at = $2`, session, ended); n != 1 {
			t.Error("the session must be ended at the action's time")
		}
		if n := count(t, pool, `SELECT count(*) FROM hands WHERE id = $1 AND status = 'carried'`, hand); n != 1 {
			t.Error("the waiting hand must be carried")
		}
		// FR-SESS-2: with the session ended, the project can have a live one again.
		queueSession(t, pool, "aaaa", "live")
	})

	t.Run("two actions at the same moment are applied one after the other", func(t *testing.T) {
		s, pool, session := queueDB(t)
		slowRaise := func(state queue.State, a queue.Action) (queue.State, error) {
			time.Sleep(50 * time.Millisecond) // long enough for the other request to arrive at the lock
			return raising(state, a)
		}
		var wg sync.WaitGroup
		sawBefore := make([]int, 2)
		for i, actor := range []string{"gh2", "gh3"} {
			wg.Go(func() {
				ap, err := s.applyWith(ctx, slowRaise, session, queue.Action{Kind: queue.Raise, ActorID: actor, At: queueAt, Title: "A question of ten letters"}, nil)
				if err != nil {
					t.Errorf("applyWith as %s: %v", actor, err)
					return
				}
				sawBefore[i] = len(ap.Before.Hands) + ap.Version*10
			})
		}
		wg.Wait()
		slices.Sort(sawBefore)
		// One saw an empty queue and made version 1; the other saw that hand and made version 2.
		if !slices.Equal(sawBefore, []int{10, 21}) {
			t.Errorf("hands seen + 10 × version = %v, want [10 21]", sawBefore)
		}
		if n := count(t, pool, `SELECT count(*) FROM hands WHERE session_id = $1`, session); n != 2 {
			t.Errorf("%d hands, want 2", n)
		}
	})

	t.Run("ARCHITECTURE §3: the database refuses what the rules must never allow, in the rules' own words", func(t *testing.T) {
		s, pool, session := queueDB(t)
		first := queueHand(t, pool, session, "gh2", "answering", 0)
		second := queueHand(t, pool, session, "gh3", "waiting", time.Minute)

		_, err := s.applyWith(ctx, raising, session, queue.Action{Kind: queue.Raise, ActorID: "gh2", At: queueAt, Title: "A second open hand"}, nil)
		if !errors.Is(err, queue.ErrAlreadyOpen) {
			t.Errorf("FR-HAND-2: a second open hand of one author: %v, want queue.ErrAlreadyOpen", err)
		}
		_, err = s.applyWith(ctx, changing(second, func(h *queue.Hand) { h.Status = queue.Answering }), session, queue.Action{Kind: queue.Next, At: queueAt}, nil)
		if !errors.Is(err, queue.ErrAnswering) {
			t.Errorf("FR-QUEUE-3: a second hand being answered: %v, want queue.ErrAnswering", err)
		}
		_, err = s.applyWith(ctx, raising, session, queue.Action{Kind: queue.Raise, ActorID: "gh404", At: queueAt, Title: "From nobody we know"}, nil)
		if !errors.Is(err, ErrUnknownUser) {
			t.Errorf("a hand of a user without a row: %v, want ErrUnknownUser", err)
		}
		if n := count(t, pool, `SELECT queue_version FROM sessions WHERE id = $1`, session); n != 0 {
			t.Errorf("the version is %d after three refused writes, want 0", n)
		}
		if n := count(t, pool, `SELECT count(*) FROM hands WHERE session_id = $1`, session); n != 2 || first == 0 {
			t.Errorf("%d hands, want the two that were there", n)
		}
	})

	t.Run("rules that lose or invent a hand are a bug, and nothing is written", func(t *testing.T) {
		s, pool, session := queueDB(t)
		queueHand(t, pool, session, "gh2", "waiting", 0)
		lose := func(queue.State, queue.Action) (queue.State, error) { return queue.State{Live: true}, nil }
		if _, err := s.applyWith(ctx, lose, session, queue.Action{Kind: queue.Skip, At: queueAt}, nil); err == nil || !strings.Contains(err.Error(), "0 of the 1 hands") {
			t.Errorf("rules that lost a hand: %v, want an error that says so", err)
		}
		invent := func(state queue.State, _ queue.Action) (queue.State, error) {
			state.Hands = append(slices.Clone(state.Hands), queue.Hand{ID: 99999, AuthorID: "gh3", Status: queue.Waiting})
			return state, nil
		}
		if _, err := s.applyWith(ctx, invent, session, queue.Action{Kind: queue.Skip, At: queueAt}, nil); err == nil || !strings.Contains(err.Error(), "were not given") {
			t.Errorf("rules that invented a hand: %v, want an error that says so", err)
		}
		if n := count(t, pool, `SELECT queue_version FROM sessions WHERE id = $1`, session); n != 0 {
			t.Errorf("the version is %d, want 0", n)
		}
	})

	t.Run("a session that does not exist is ErrNoSession", func(t *testing.T) {
		s, _, _ := queueDB(t)
		if _, err := s.applyWith(ctx, raising, 99999, queue.Action{Kind: queue.Raise, ActorID: "gh2", At: queueAt}, nil); !errors.Is(err, ErrNoSession) {
			t.Errorf("err = %v, want ErrNoSession", err)
		}
	})
}

func TestQueueSnapshot(t *testing.T) {
	ctx := context.Background()

	t.Run("a project that never had a session: nothing but the planned time", func(t *testing.T) {
		pool := testDB(t)
		people(t, pool)
		planned := queueAt.Add(48 * time.Hour)
		mustExec(t, pool, `INSERT INTO projects (id, github_id, owner, repo, created_by, next_session_at) VALUES ('aaaa', 11, 'PaperPlane', 'Stash', 'gh1', $1)`, planned)
		snap, err := New(pool).Queue(ctx, "aaaa")
		if err != nil || snap.Session != nil || len(snap.Open) != 0 || len(snap.Closed) != 0 || !snap.NextSessionAt.Equal(planned) || snap.MedianAnswer != 0 {
			t.Fatalf("snapshot = %+v, %v", snap, err)
		}
		if snap, err := New(pool).Queue(ctx, "nope"); err != nil || snap.Session != nil || !snap.NextSessionAt.IsZero() {
			t.Fatalf("an unknown project: %+v, %v; want an empty snapshot", snap, err)
		}
	})

	t.Run("FR-QUEUE-1, FR-QUEUE-4: the live session with its hands in the queue's order", func(t *testing.T) {
		s, pool, session := queueDB(t)
		mustExec(t, pool, `UPDATE users SET name = 'Mia Kowalski', avatar_url = 'https://avatars.example/1' WHERE id = 'gh1'`)
		mustExec(t, pool, `UPDATE sessions SET planned_minutes = 45, queue_version = 7 WHERE id = $1`, session)
		late := queueHand(t, pool, session, "gh3", "waiting", 3*time.Minute)
		early := queueHand(t, pool, session, "gh1", "waiting", time.Minute)
		answering := queueHand(t, pool, session, "gh2", "answering", 2*time.Minute)
		mustExec(t, pool, `UPDATE hands SET taken_at = $2, taken_by = 'gh1' WHERE id = $1`, answering, queueAt.Add(5*time.Minute))
		mustExec(t, pool, `INSERT INTO hand_plus (hand_id, user_id) VALUES ($1, 'gh2')`, late)
		// Away moved this one behind: the order follows sort_at, not when it was raised.
		mustExec(t, pool, `UPDATE hands SET sort_at = $2 WHERE id = $1`, early, queueAt.Add(10*time.Minute))

		snap, err := s.Queue(ctx, "aaaa")
		if err != nil {
			t.Fatalf("Queue: %v", err)
		}
		sess := snap.Session
		if sess == nil || sess.ID != session || sess.ProjectID != "aaaa" || sess.Number != 1 || !sess.Live || sess.PlannedMinutes != 45 || sess.Version != 7 ||
			!sess.StartedAt.Equal(queueAt) || !sess.EndedAt.IsZero() || sess.Title != "Thursday office hours" {
			t.Fatalf("session = %+v", sess)
		}
		if sess.Host != (Person{UserID: "gh1", Login: "mia", Name: "Mia Kowalski", AvatarURL: "https://avatars.example/1"}) {
			t.Errorf("host = %+v", sess.Host)
		}
		var order []int64
		for _, h := range snap.Open {
			order = append(order, h.ID)
		}
		if !slices.Equal(order, []int64{answering, late, early}) {
			t.Fatalf("open hands in order %v, want the one being answered, then by sort_at: %v", order, []int64{answering, late, early})
		}
		now := snap.Open[0]
		if now.Author != (Person{UserID: "gh2", Login: "diego", Name: "diego"}) || now.Taker.UserID != "gh1" || now.Taker.Name != "Mia Kowalski" || !now.TakenAt.Equal(queueAt.Add(5*time.Minute)) {
			t.Errorf("the hand being answered = %+v (a name that is empty falls back to the login)", now)
		}
		if next := snap.Open[1]; !slices.Equal(next.Plus, []string{"gh2"}) || next.Taker != (Person{}) || next.FAQID != 0 {
			t.Errorf("the first waiting hand = %+v", next)
		}
	})

	t.Run("FR-FAQ-1, FR-QUEUE-10: closed hands, their FAQ entries and the median answer time", func(t *testing.T) {
		s, pool, session := queueDB(t)
		answered := func(author string, tookMinutes int, closedAfter time.Duration) int64 {
			id := queueHand(t, pool, session, author, "answered", 0)
			closed := queueAt.Add(closedAfter)
			mustExec(t, pool, `UPDATE hands SET taken_at = $2, taken_by = 'gh1', closed_at = $3 WHERE id = $1`, id, closed.Add(-time.Duration(tookMinutes)*time.Minute), closed)
			return id
		}
		first := answered("gh2", 4, 10*time.Minute)
		if snap, _ := s.Queue(ctx, "aaaa"); snap.MedianAnswer != 0 {
			t.Errorf("median after one answer = %v, want 0 until there are two", snap.MedianAnswer)
		}
		second := answered("gh3", 10, 30*time.Minute)
		if snap, _ := s.Queue(ctx, "aaaa"); snap.MedianAnswer != 7*time.Minute {
			t.Errorf("median of 4 and 10 minutes = %v, want 7m", snap.MedianAnswer)
		}
		third := answered("gh1", 6, 40*time.Minute)
		skipped := queueHand(t, pool, session, "gh2", "skipped", 0)
		mustExec(t, pool, `UPDATE hands SET closed_at = $2 WHERE id = $1`, skipped, queueAt.Add(20*time.Minute))
		mustExec(t, pool, `INSERT INTO faq_entries (project_id, hand_id, slug, title, answer, author_id) VALUES ('aaaa', $1, 'kept', 'Kept', 'An answer that stays.', 'gh1')`, first)
		mustExec(t, pool, `INSERT INTO faq_entries (project_id, hand_id, slug, title, answer, author_id, deleted_at) VALUES ('aaaa', $1, 'gone', 'Gone', 'An answer that was deleted.', 'gh1', now())`, second)

		snap, err := s.Queue(ctx, "aaaa")
		if err != nil {
			t.Fatalf("Queue: %v", err)
		}
		var order []int64
		for _, h := range snap.Closed {
			order = append(order, h.ID)
		}
		if !slices.Equal(order, []int64{third, second, skipped, first}) || len(snap.Open) != 0 {
			t.Fatalf("closed hands in order %v, want the latest closed first: %v", order, []int64{third, second, skipped, first})
		}
		if snap.MedianAnswer != 6*time.Minute {
			t.Errorf("median of 4, 6 and 10 minutes = %v, want 6m (a skipped hand does not count)", snap.MedianAnswer)
		}
		if kept := snap.Closed[3]; kept.FAQID == 0 || kept.FAQSlug != "kept" {
			t.Errorf("the answered hand = %+v, want its FAQ entry", kept)
		}
		if gone := snap.Closed[1]; gone.FAQID != 0 || gone.FAQSlug != "" {
			t.Errorf("FR-FAQ-6: the hand whose entry was deleted still names it: %+v", gone)
		}
	})

	t.Run("no live session: the last one that ended, and the count of sessions per project", func(t *testing.T) {
		pool := testDB(t)
		people(t, pool)
		mustExec(t, pool, `INSERT INTO projects (id, github_id, owner, repo, created_by) VALUES ('aaaa', 11, 'PaperPlane', 'Stash', 'gh1'), ('bbbb', 12, 'Other', 'Repo', 'gh1')`)
		queueSession(t, pool, "aaaa", "ended")
		queueSession(t, pool, "bbbb", "ended")
		last := queueSession(t, pool, "aaaa", "ended")
		carried := queueHand(t, pool, last, "gh2", "carried", 0)
		s := New(pool)
		snap, err := s.Queue(ctx, "aaaa")
		if err != nil || snap.Session == nil || snap.Session.ID != last || snap.Session.Live || snap.Session.Number != 2 || snap.Session.EndedAt.IsZero() {
			t.Fatalf("snapshot = %+v, %v; want the project's second session, ended", snap.Session, err)
		}
		if len(snap.Closed) != 1 || snap.Closed[0].ID != carried || snap.Closed[0].Status != queue.Carried {
			t.Errorf("closed = %+v, want the carried hand (FR-SESS-7: its author sees where it went)", snap.Closed)
		}
		live := queueSession(t, pool, "aaaa", "live")
		if snap, err := s.Queue(ctx, "aaaa"); err != nil || snap.Session.ID != live || snap.Session.Number != 3 || !snap.Session.Live {
			t.Fatalf("with a live session: %+v, %v", snap.Session, err)
		}
	})
}

func TestDone(t *testing.T) {
	ctx := context.Background()
	finishing := func(handID int64) stepFunc {
		return changing(handID, func(h *queue.Hand) { h.Status, h.ClosedAt = queue.Answered, queueAt.Add(9*time.Minute) })
	}
	draft := FAQDraft{Slug: "how-do-i-retry-only-on-5xx", Title: "How do I retry only on 5xx?", Question: "The docs show retry: 3.",
		Answer: "Pass a function: retry: (err) => err.status >= 500.", Links: []string{"https://example.org/docs/retry"}, AuthorID: "gh1"}

	t.Run("FR-FAQ-1, FR-HAND-8: the hand is answered and its entry exists, or neither", func(t *testing.T) {
		s, pool, session := queueDB(t)
		hand := queueHand(t, pool, session, "gh2", "answering", 0)
		mustExec(t, pool, `INSERT INTO hand_plus (hand_id, user_id) VALUES ($1, 'gh3'), ($1, 'gh1')`, hand)
		at := queueAt.Add(9 * time.Minute)
		ap, entry, err := s.doneWith(ctx, finishing(hand), session, queue.Action{Kind: queue.Done, ActorID: "gh1", HandID: hand, At: at}, draft)
		if err != nil {
			t.Fatalf("doneWith: %v", err)
		}
		if ap.Version != 1 || len(ap.Before.Hands) != 1 || len(ap.Before.Hands[0].Plus) != 2 {
			t.Errorf("applied = %+v, want the hand as it was before, with its two +1s", ap)
		}
		want := FAQEntry{ID: entry.ID, ProjectID: "aaaa", HandID: hand, Slug: draft.Slug, Title: draft.Title, Question: draft.Question, Answer: draft.Answer,
			Links: draft.Links, Author: Person{UserID: "gh1", Login: "mia", Name: "mia"}, AskedBy: 3, CreatedAt: at}
		if entry.ID == 0 || !entry.CreatedAt.Equal(at) || entry.Slug != want.Slug || entry.AskedBy != 3 || entry.Author != want.Author ||
			entry.HandID != hand || entry.Title != want.Title || entry.Question != want.Question || entry.Answer != want.Answer || !slices.Equal(entry.Links, want.Links) || entry.Deleted {
			t.Errorf("entry = %+v\nwant    %+v", entry, want)
		}
		if n := count(t, pool, `SELECT count(*) FROM faq_entries WHERE id = $1 AND project_id = 'aaaa' AND hand_id = $2 AND asked_by = 3 AND author_id = 'gh1'
			AND links = '["https://example.org/docs/retry"]'::jsonb AND created_at = $3 AND deleted_at IS NULL`, entry.ID, hand, at); n != 1 {
			t.Error("the entry's row is not what was returned")
		}
		if n := count(t, pool, `SELECT count(*) FROM hands WHERE id = $1 AND status = 'answered'`, hand); n != 1 {
			t.Error("the hand is not answered")
		}
		snap, err := s.Queue(ctx, "aaaa")
		if err != nil || len(snap.Closed) != 1 || snap.Closed[0].FAQID != entry.ID || snap.Closed[0].FAQSlug != draft.Slug {
			t.Errorf("the queue does not name the entry of the answered hand: %+v, %v", snap.Closed, err)
		}
	})

	t.Run("FR-FAQ-1: a slug that is taken gets -2, then -3, and stays within 80 characters", func(t *testing.T) {
		s, pool, session := queueDB(t)
		slugs := func(d FAQDraft, n int) []string {
			var got []string
			for range n {
				hand := queueHand(t, pool, session, "gh2", "answering", 0)
				_, entry, err := s.doneWith(ctx, finishing(hand), session, queue.Action{Kind: queue.Done, ActorID: "gh1", HandID: hand, At: queueAt}, d)
				if err != nil {
					t.Fatalf("doneWith: %v", err)
				}
				got = append(got, entry.Slug)
			}
			return got
		}
		if got := slugs(draft, 3); !slices.Equal(got, []string{draft.Slug, draft.Slug + "-2", draft.Slug + "-3"}) {
			t.Errorf("slugs = %v", got)
		}
		// A deleted entry keeps its address: the next answer with that title does not take it over.
		mustExec(t, pool, `UPDATE faq_entries SET deleted_at = now() WHERE slug = $1`, draft.Slug+"-3")
		if got := slugs(draft, 1); got[0] != draft.Slug+"-4" {
			t.Errorf("after a delete the next slug is %q, want %q", got[0], draft.Slug+"-4")
		}
		long := draft
		long.Slug = strings.Repeat("a", 79) + "b" // 80 characters: no room for a suffix
		got := slugs(long, 3)
		if got[0] != long.Slug || got[1] != strings.Repeat("a", 78)+"-2" || got[2] != strings.Repeat("a", 78)+"-3" {
			t.Errorf("long slugs = %v", got)
		}
		for _, slug := range got {
			if len(slug) > 80 {
				t.Errorf("slug %q is %d characters, more than the 80 a FAQ address takes", slug, len(slug))
			}
		}
	})

	t.Run("nothing happens by halves", func(t *testing.T) {
		s, pool, session := queueDB(t)
		hand := queueHand(t, pool, session, "gh2", "answering", 0)
		untouched := func(when string) {
			t.Helper()
			if n := count(t, pool, `SELECT count(*) FROM faq_entries`); n != 0 {
				t.Errorf("%s: %d FAQ entries, want none", when, n)
			}
			if n := count(t, pool, `SELECT count(*) FROM hands WHERE id = $1 AND status = 'answering'`, hand); n != 1 {
				t.Errorf("%s: the hand is no longer being answered", when)
			}
			if n := count(t, pool, `SELECT queue_version FROM sessions WHERE id = $1`, session); n != 0 {
				t.Errorf("%s: the version is %d, want 0", when, n)
			}
		}
		done := queue.Action{Kind: queue.Done, ActorID: "gh1", HandID: hand, At: queueAt}

		refuse := func(queue.State, queue.Action) (queue.State, error) { return queue.State{}, queue.ErrNotAnswering }
		if _, _, err := s.doneWith(ctx, refuse, session, done, draft); !errors.Is(err, queue.ErrNotAnswering) {
			t.Errorf("a refusal of the rules: %v, want it back as it is", err)
		}
		untouched("after a refusal")

		stranger := draft
		stranger.AuthorID = "gh404"
		if _, _, err := s.doneWith(ctx, finishing(hand), session, done, stranger); !errors.Is(err, ErrUnknownUser) {
			t.Errorf("an author without a row: %v, want ErrUnknownUser", err)
		}
		untouched("after an entry that could not be written")

		bad := draft
		bad.Slug = "Not A Slug"
		if _, _, err := s.doneWith(ctx, finishing(hand), session, done, bad); err == nil {
			t.Error("a slug that no address takes was accepted")
		}
		if _, _, err := s.doneWith(ctx, finishing(hand), session, queue.Action{Kind: queue.Skip, HandID: hand, At: queueAt}, draft); err == nil {
			t.Error("Done accepted an action that is not a Done")
		}
		untouched("after bad input")
	})

	t.Run("a draft without links is stored as an empty list", func(t *testing.T) {
		s, pool, session := queueDB(t)
		hand := queueHand(t, pool, session, "gh2", "answering", 0)
		plain := draft
		plain.Links = nil
		_, entry, err := s.doneWith(ctx, finishing(hand), session, queue.Action{Kind: queue.Done, ActorID: "gh1", HandID: hand, At: queueAt}, plain)
		if err != nil || entry.Links == nil || len(entry.Links) != 0 || entry.AskedBy != 1 {
			t.Fatalf("entry = %+v, %v; want no links, as an empty list, asked by 1", entry, err)
		}
		if n := count(t, pool, `SELECT count(*) FROM faq_entries WHERE id = $1 AND links = '[]'::jsonb`, entry.ID); n != 1 {
			t.Error("links must be stored as []")
		}
	})
}

func TestQueueLookups(t *testing.T) {
	ctx := context.Background()
	s, pool, session := queueDB(t)
	hand := queueHand(t, pool, session, "gh2", "waiting", 0)

	if project, found, err := s.ProjectOfSession(ctx, session); err != nil || !found || project != "aaaa" {
		t.Errorf("ProjectOfSession = %q, %v, %v", project, found, err)
	}
	if _, found, err := s.ProjectOfSession(ctx, 99999); err != nil || found {
		t.Errorf("ProjectOfSession of an unknown id: found %v, err %v", found, err)
	}
	if project, sess, found, err := s.ProjectOfHand(ctx, hand); err != nil || !found || project != "aaaa" || sess != session {
		t.Errorf("ProjectOfHand = %q, %d, %v, %v", project, sess, found, err)
	}
	if _, _, found, err := s.ProjectOfHand(ctx, 99999); err != nil || found {
		t.Errorf("ProjectOfHand of an unknown id: found %v, err %v", found, err)
	}

	mustExec(t, pool, `UPDATE users SET name = 'Ana Ferreira' WHERE id = 'gh3'`)
	got, err := s.People(ctx, []string{"gh3", "gh404", "gh1", "gh3"})
	want := []Person{{UserID: "gh3", Login: "ana", Name: "Ana Ferreira"}, {UserID: "gh1", Login: "mia", Name: "mia"}}
	if err != nil || !slices.Equal(got, want) {
		t.Errorf("People = %+v, %v; want %+v (in the order asked, unknown ids left out, nobody twice)", got, err, want)
	}
	if got, err := s.People(ctx, nil); err != nil || len(got) != 0 {
		t.Errorf("People(nil) = %+v, %v", got, err)
	}
}
