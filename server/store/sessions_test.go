package store

import (
	"context"
	"errors"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// sessionSeed adds mia, diego and ana, the project aaaa that mia maintains, and the project bbbb
// that ana maintains.
func sessionSeed(t *testing.T, pool *pgxpool.Pool) *Store {
	t.Helper()
	people(t, pool)
	s := New(pool)
	for _, p := range []struct {
		project Project
		by      string
	}{
		{Project{ID: "aaaa", GitHubID: 1001, Owner: "PaperPlane", Repo: "Stash"}, "gh1"},
		{Project{ID: "bbbb", GitHubID: 1002, Owner: "ana", Repo: "atlas"}, "gh3"},
	} {
		if _, _, err := s.CreateProject(context.Background(), p.project, p.by); err != nil {
			t.Fatalf("CreateProject(%s): %v", p.project.ID, err)
		}
	}
	return s
}

// sessionEnd ends a project's live session the way the queue's End does, as far as a start cares.
func sessionEnd(t *testing.T, pool *pgxpool.Pool, projectID string) {
	t.Helper()
	mustExec(t, pool, `UPDATE sessions SET status = 'ended', ended_at = now() WHERE project_id = $1 AND status = 'live'`, projectID)
}

func TestSessionStart(t *testing.T) {
	t.Run("FR-SESS-1: a started session is live at version 1, with its number and its host", func(t *testing.T) {
		pool := testDB(t)
		s, ctx := sessionSeed(t, pool), context.Background()
		mustExec(t, pool, `UPDATE users SET name = 'Mia K', avatar_url = 'https://avatars.example/u/1' WHERE id = 'gh1'`)

		before := time.Now().Add(-time.Second)
		got, err := s.StartSession(ctx, NewSession{ProjectID: "aaaa", HostID: "gh1", Title: "Thursday office hours", PlannedMinutes: 45})
		if err != nil {
			t.Fatalf("StartSession: %v", err)
		}
		if got.StartedAt.Before(before) || got.StartedAt.After(time.Now().Add(time.Second)) || got.StartedAt.Nanosecond()%1000 != 0 {
			t.Errorf("StartedAt = %v, want now, in whole microseconds", got.StartedAt)
		}
		want := Session{
			ID: got.ID, ProjectID: "aaaa", Number: 1, Title: "Thursday office hours", Live: true, PlannedMinutes: 45,
			StartedAt: got.StartedAt, Version: 1,
			Host: Person{UserID: "gh1", Login: "mia", Name: "Mia K", AvatarURL: "https://avatars.example/u/1"},
		}
		if got.ID == 0 || got != want {
			t.Fatalf("StartSession = %+v, want %+v", got, want)
		}
		// What was returned is what was written: the same row, read back.
		var (
			status, startedBy string
			version, planned  int
			closed            bool
			startedAt         time.Time
		)
		if err := pool.QueryRow(ctx, `SELECT status, started_by, queue_version, planned_minutes, hands_closed, started_at FROM sessions WHERE id = $1`, got.ID).
			Scan(&status, &startedBy, &version, &planned, &closed, &startedAt); err != nil {
			t.Fatalf("read the session: %v", err)
		}
		if status != "live" || startedBy != "gh1" || version != 1 || planned != 45 || closed || !startedAt.Equal(got.StartedAt) {
			t.Fatalf("the row: status %q, started_by %q, version %d, planned %d, hands closed %v, started at %v (returned %v)",
				status, startedBy, version, planned, closed, startedAt, got.StartedAt)
		}
	})

	t.Run("FR-SESS-1: a session without a planned length, hosted by someone without a display name", func(t *testing.T) {
		pool := testDB(t)
		s, ctx := sessionSeed(t, pool), context.Background()
		got, err := s.StartSession(ctx, NewSession{ProjectID: "bbbb", HostID: "gh3", Title: "Atlas hour"})
		if err != nil {
			t.Fatalf("StartSession: %v", err)
		}
		// The login stands in for a missing name, as everywhere a person is shown.
		if got.PlannedMinutes != 0 || got.Host != (Person{UserID: "gh3", Login: "ana", Name: "ana"}) {
			t.Fatalf("StartSession = %+v, want no planned length and ana as the host", got)
		}
		if n := count(t, pool, `SELECT count(*) FROM sessions WHERE id = $1 AND planned_minutes IS NULL`, got.ID); n != 1 {
			t.Fatal("a length that was not planned must be stored as none, not as 0 minutes")
		}
	})

	t.Run("FR-SESS-2: a second live session in one project is refused and writes nothing", func(t *testing.T) {
		pool := testDB(t)
		s, ctx := sessionSeed(t, pool), context.Background()
		first, err := s.StartSession(ctx, NewSession{ProjectID: "aaaa", HostID: "gh1", Title: "Thursday office hours"})
		if err != nil {
			t.Fatalf("StartSession: %v", err)
		}
		if _, err := s.StartSession(ctx, NewSession{ProjectID: "aaaa", HostID: "gh1", Title: "A second one"}); !errors.Is(err, ErrSessionLive) {
			t.Fatalf("second StartSession: err = %v, want ErrSessionLive", err)
		}
		if n := count(t, pool, `SELECT count(*) FROM sessions`); n != 1 {
			t.Fatalf("%d sessions after the refused start, want 1", n)
		}
		// Another project is not in the way, and has its own count.
		other, err := s.StartSession(ctx, NewSession{ProjectID: "bbbb", HostID: "gh3", Title: "Atlas hour"})
		if err != nil || other.Number != 1 {
			t.Fatalf("StartSession in another project = %+v, %v; want its first session", other, err)
		}
		// Once the live one has ended, the next start works and counts on.
		sessionEnd(t, pool, "aaaa")
		next, err := s.StartSession(ctx, NewSession{ProjectID: "aaaa", HostID: "gh1", Title: "The week after"})
		if err != nil || next.Number != 2 || next.ID <= first.ID || next.Version != 1 {
			t.Fatalf("StartSession after the end = %+v, %v; want session number 2 at version 1", next, err)
		}
	})

	t.Run("a host whose user row is gone is ErrUnknownUser, and no session is left behind", func(t *testing.T) {
		pool := testDB(t)
		s, ctx := sessionSeed(t, pool), context.Background()
		if _, err := s.StartSession(ctx, NewSession{ProjectID: "aaaa", HostID: "gh404", Title: "Thursday office hours"}); !errors.Is(err, ErrUnknownUser) {
			t.Fatalf("StartSession for a user with no row: err = %v, want ErrUnknownUser", err)
		}
		if n := count(t, pool, `SELECT count(*) FROM sessions`); n != 0 {
			t.Fatalf("a refused start left %d session rows behind", n)
		}
	})

	t.Run("a project that does not exist is an error, not a session", func(t *testing.T) {
		pool := testDB(t)
		s, ctx := sessionSeed(t, pool), context.Background()
		_, err := s.StartSession(ctx, NewSession{ProjectID: "zzzz", HostID: "gh1", Title: "Thursday office hours"})
		if err == nil || errors.Is(err, ErrUnknownUser) || errors.Is(err, ErrSessionLive) {
			t.Fatalf("StartSession for no project: err = %v, want an error that is neither refusal", err)
		}
	})
}

func TestSessionNumberCountsPerProject(t *testing.T) {
	pool := testDB(t)
	s, ctx := sessionSeed(t, pool), context.Background()
	// Session ids count across the whole site; a session's number counts inside its project. The
	// starts happen one after the other, each in the database the one before left.
	for _, tt := range []struct {
		name            string
		project, host   string
		wantID          int64
		wantNumber      int
		endItAfterwards bool
	}{
		{"FR-SESS-1: the first session of a project is number 1", "aaaa", "gh1", 1, 1, true},
		{"FR-SESS-1: another project starts at 1 too", "bbbb", "gh3", 2, 1, true},
		{"FR-SESS-1: the second session of the first project is number 2", "aaaa", "gh1", 3, 2, true},
		{"FR-SESS-1: its third is number 3, whatever happened elsewhere", "aaaa", "gh1", 4, 3, false},
		{"FR-SESS-1: the other project counts its own", "bbbb", "gh3", 5, 2, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := s.StartSession(ctx, NewSession{ProjectID: tt.project, HostID: tt.host, Title: "Office hours"})
			if err != nil || got.ID != tt.wantID || got.Number != tt.wantNumber {
				t.Fatalf("id %d, number %d, err %v; want id %d and number %d", got.ID, got.Number, err, tt.wantID, tt.wantNumber)
			}
			if tt.endItAfterwards {
				sessionEnd(t, pool, tt.project)
			}
		})
	}
}

func TestSessionStartAtTheSameMoment(t *testing.T) {
	pool := testDB(t)
	s, ctx := sessionSeed(t, pool), context.Background()
	// Two hands wait from last week, so the starts race for them as well.
	mustExec(t, pool, `INSERT INTO sessions (project_id, title, status, ended_at) VALUES ('aaaa', 'Last week', 'ended', now())`)
	mustExec(t, pool, `INSERT INTO hands (session_id, author_id, title, status, closed_at) VALUES
		(1, 'gh2', 'How do I retry only on 5xx errors?', 'carried', now()),
		(1, 'gh3', 'Can the cache be shared between workers?', 'carried', now())`)

	// FR-SESS-2: eight starts in one project at once. The unique index decides.
	const racers = 8
	type result struct {
		id  int64
		err error
	}
	results := make([]result, racers)
	var wg sync.WaitGroup
	for i := range racers {
		wg.Go(func() {
			started, err := s.StartSession(ctx, NewSession{ProjectID: "aaaa", HostID: "gh1", Title: "Thursday office hours"})
			results[i] = result{started.ID, err}
		})
	}
	wg.Wait()

	var winner int64
	for i, r := range results {
		switch {
		case r.err == nil && winner != 0:
			t.Fatalf("FR-SESS-2: racer %d started session %d, but session %d was started too", i, r.id, winner)
		case r.err == nil:
			winner = r.id
		case !errors.Is(r.err, ErrSessionLive):
			t.Fatalf("FR-SESS-2: racer %d: err = %v, want ErrSessionLive", i, r.err)
		}
	}
	if winner == 0 {
		t.Fatal("FR-SESS-2: no start won")
	}
	if live, all := count(t, pool, `SELECT count(*) FROM sessions WHERE status = 'live'`), count(t, pool, `SELECT count(*) FROM sessions`); live != 1 || all != 2 {
		t.Fatalf("%d live sessions of %d, want 1 of 2 (the old one and the winner)", live, all)
	}
	// FR-SESS-7: the hands that waited are in the winner's session, once.
	if n := count(t, pool, `SELECT count(*) FROM hands WHERE session_id = $1 AND status = 'waiting'`, winner); n != 2 {
		t.Fatalf("FR-SESS-7: %d hands wait in the session that won, want the 2 that were carried", n)
	}
	if n := count(t, pool, `SELECT count(*) FROM hands`); n != 2 {
		t.Fatalf("%d hands after the race, want 2", n)
	}
}

// sessionHandRow is a hand as the carry-over test reads it back.
type sessionHandRow struct {
	ID        int64
	SessionID int64
	AuthorID  string
	Title     string
	Details   string
	Status    string
	RaisedAt  time.Time
	SortAt    time.Time
	Cleared   bool // closed_at, taken_at, taken_by and acked_at are all empty
	AwayCount int
}

// sessionHands reads the hands a WHERE clause picks, in the queue's order.
func sessionHands(t *testing.T, pool *pgxpool.Pool, where string, args ...any) []sessionHandRow {
	t.Helper()
	rows, err := pool.Query(context.Background(), `
		SELECT id, session_id, author_id, title, details, status, raised_at, sort_at,
		       closed_at IS NULL AND taken_at IS NULL AND taken_by IS NULL AND acked_at IS NULL, away_count
		FROM hands WHERE `+where+` ORDER BY sort_at, id`, args...)
	if err != nil {
		t.Fatalf("read hands: %v", err)
	}
	hands, err := pgx.CollectRows(rows, pgx.RowToStructByPos[sessionHandRow])
	if err != nil {
		t.Fatalf("read hands: %v", err)
	}
	return hands
}

func TestSessionStartBringsCarriedHandsIn(t *testing.T) {
	pool := testDB(t)
	s, ctx := sessionSeed(t, pool), context.Background()
	mustExec(t, pool, `INSERT INTO users (id, github_id, login) VALUES ('gh4', 4, 'lee')`)
	// Last week's session, by the minute. Whole seconds, so Postgres keeps every time as it is.
	lastWeek := time.Now().UTC().Add(-7 * 24 * time.Hour).Truncate(time.Second)
	at := func(minute int) time.Time { return lastWeek.Add(time.Duration(minute) * time.Minute) }
	// Session 1: two weeks ago, ended without carry-over. Session 2: another project, ended with
	// a carried hand. Session 3: last week, ended with three hands carried over.
	mustExec(t, pool, `INSERT INTO sessions (project_id, title, status, ended_at) VALUES
		('aaaa', 'Two weeks ago', 'ended', now()), ('bbbb', 'Atlas hour', 'ended', now()), ('aaaa', 'Last week', 'ended', now())`)
	mustExec(t, pool, `INSERT INTO hands (session_id, author_id, title, status, closed_at) VALUES
		(1, 'gh2', 'Never reached two weeks ago', 'not_reached', now()),
		(1, 'gh3', 'Answered two weeks ago', 'answered', now()),
		(2, 'gh2', 'Carried in another project', 'carried', now())`)
	// Hand 4 was raised first, missed a turn and was sent back, so it is last in line. It holds
	// every field a start must clear.
	mustExec(t, pool, `INSERT INTO hands (session_id, author_id, title, details, status, raised_at, sort_at, taken_at, taken_by, closed_at, acked_at, away_count) VALUES
		(3, 'gh2', 'Sent back after a missed turn', 'Kept as written.', 'carried', $1, $2, $3, 'gh1', $6, $3, 2),
		(3, 'gh3', 'First in line', '', 'carried', $4, $4, NULL, NULL, $6, NULL, 0),
		(3, 'gh1', 'Second in line', '', 'carried', $5, $5, NULL, NULL, $6, NULL, 1)`,
		at(10), at(30), at(20), at(12), at(14), at(45))
	mustExec(t, pool, `INSERT INTO hands (session_id, author_id, title, status, closed_at) VALUES
		(3, 'gh2', 'Answered last week', 'answered', now()),
		(3, 'gh3', 'Withdrawn last week', 'withdrawn', now())`)
	mustExec(t, pool, `INSERT INTO hand_plus (hand_id, user_id) VALUES (5, 'gh1'), (5, 'gh2')`)

	started, err := s.StartSession(ctx, NewSession{ProjectID: "aaaa", HostID: "gh1", Title: "Thursday office hours"})
	if err != nil {
		t.Fatalf("StartSession: %v", err)
	}
	if started.Number != 3 || started.Version != 1 {
		t.Fatalf("StartSession = %+v, want the project's third session at version 1", started)
	}

	t.Run("FR-SESS-7: the carried hands wait in the new session, in their old order, as they were raised", func(t *testing.T) {
		want := []sessionHandRow{
			{ID: 5, SessionID: started.ID, AuthorID: "gh3", Title: "First in line", Status: "waiting", RaisedAt: at(12), SortAt: at(12), Cleared: true},
			{ID: 6, SessionID: started.ID, AuthorID: "gh1", Title: "Second in line", Status: "waiting", RaisedAt: at(14), SortAt: at(14), Cleared: true},
			{ID: 4, SessionID: started.ID, AuthorID: "gh2", Title: "Sent back after a missed turn", Details: "Kept as written.", Status: "waiting", RaisedAt: at(10), SortAt: at(30), Cleared: true},
		}
		got := sessionHands(t, pool, `session_id = $1`, started.ID)
		if !slices.EqualFunc(got, want, func(a, b sessionHandRow) bool {
			a.RaisedAt, a.SortAt, b.RaisedAt, b.SortAt = a.RaisedAt.UTC(), a.SortAt.UTC(), b.RaisedAt.UTC(), b.SortAt.UTC()
			return a == b
		}) {
			t.Fatalf("the new session's hands:\n got %+v\nwant %+v", got, want)
		}
		// A +1 stays with its hand.
		if n := count(t, pool, `SELECT count(*) FROM hand_plus WHERE hand_id = 5`); n != 2 {
			t.Errorf("FR-HAND-7: hand 5 has %d +1s after the carry-over, want its 2", n)
		}
	})

	t.Run("FR-SESS-7: a hand raised in the new session stands behind the carried ones", func(t *testing.T) {
		mustExec(t, pool, `INSERT INTO hands (session_id, author_id, title) VALUES ($1, 'gh4', 'Raised today')`, started.ID)
		var order []int64
		for _, h := range sessionHands(t, pool, `session_id = $1`, started.ID) {
			order = append(order, h.ID)
		}
		if want := []int64{5, 6, 4, 9}; !slices.Equal(order, want) {
			t.Fatalf("the queue's order = %v, want %v", order, want)
		}
	})

	t.Run("FR-QUEUE-2: only carried hands of this project move", func(t *testing.T) {
		for _, tt := range []struct {
			id      int64
			session int64
			status  string
		}{
			{1, 1, "not_reached"}, {2, 1, "answered"}, {3, 2, "carried"}, {7, 3, "answered"}, {8, 3, "withdrawn"},
		} {
			got := sessionHands(t, pool, `id = $1`, tt.id)
			if len(got) != 1 || got[0].SessionID != tt.session || got[0].Status != tt.status || got[0].Cleared {
				t.Errorf("hand %d = %+v, want it left in session %d as %s", tt.id, got, tt.session, tt.status)
			}
		}
	})
}

func TestSessionSetNext(t *testing.T) {
	pool := testDB(t)
	s, ctx := sessionSeed(t, pool), context.Background()
	read := func(projectID string) *time.Time {
		t.Helper()
		var at *time.Time
		if err := pool.QueryRow(ctx, `SELECT next_session_at FROM projects WHERE id = $1`, projectID).Scan(&at); err != nil {
			t.Fatalf("read next_session_at: %v", err)
		}
		return at
	}
	thursday := time.Date(2026, 11, 19, 11, 30, 0, 123456000, time.UTC)

	// FR-SESS-5: the time is kept as the moment it names, whatever zone it was written in.
	if err := s.SetNextSession(ctx, "aaaa", thursday.In(time.FixedZone("IST", 5*3600+1800))); err != nil {
		t.Fatalf("SetNextSession: %v", err)
	}
	if got := read("aaaa"); got == nil || !got.Equal(thursday) {
		t.Fatalf("FR-SESS-5: next_session_at = %v, want %v", got, thursday)
	}
	if got := read("bbbb"); got != nil {
		t.Fatalf("FR-SESS-5: another project got a next session too: %v", got)
	}
	// Setting it again replaces it; the zero time clears it.
	if err := s.SetNextSession(ctx, "aaaa", thursday.Add(7*24*time.Hour)); err != nil {
		t.Fatalf("second SetNextSession: %v", err)
	}
	if got := read("aaaa"); got == nil || !got.Equal(thursday.Add(7*24*time.Hour)) {
		t.Fatalf("FR-SESS-5: next_session_at = %v after setting it again", got)
	}
	if err := s.SetNextSession(ctx, "aaaa", time.Time{}); err != nil {
		t.Fatalf("clearing: %v", err)
	}
	if got := read("aaaa"); got != nil {
		t.Fatalf("FR-SESS-5: next_session_at = %v after clearing, want none", got)
	}
	if err := s.SetNextSession(ctx, "zzzz", thursday); err == nil {
		t.Fatal("SetNextSession for a project that does not exist = nil error, want one")
	}
}

func TestSessionRemoveMaintainer(t *testing.T) {
	pool := testDB(t)
	s, ctx := sessionSeed(t, pool), context.Background()
	if err := s.AddMaintainer(ctx, "aaaa", "gh2"); err != nil { // diego maintains aaaa too
		t.Fatalf("AddMaintainer: %v", err)
	}
	if err := s.AddMaintainer(ctx, "bbbb", "gh2"); err != nil { // and bbbb
		t.Fatalf("AddMaintainer: %v", err)
	}
	roles := func(projectID, userID string) (isMember, isMaintainer bool) {
		t.Helper()
		isMember, err := s.IsMember(ctx, projectID, userID)
		if err != nil {
			t.Fatalf("IsMember: %v", err)
		}
		isMaintainer, err = s.IsMaintainer(ctx, projectID, userID)
		if err != nil {
			t.Fatalf("IsMaintainer: %v", err)
		}
		return isMember, isMaintainer
	}

	// Taking the role away twice is as good as once.
	for range 2 {
		if err := s.RemoveMaintainer(ctx, "aaaa", "gh2"); err != nil {
			t.Fatalf("RemoveMaintainer: %v", err)
		}
	}
	for _, tt := range []struct {
		name                   string
		project, user          string
		isMember, isMaintainer bool
	}{
		{"NFR-SEC-2: the one who lost the role stays a member of the room", "aaaa", "gh2", true, false},
		{"NFR-SEC-2: the other maintainer of the project keeps the role", "aaaa", "gh1", true, true},
		{"NFR-SEC-2: their role in another project is not touched", "bbbb", "gh2", true, true},
	} {
		if isMember, isMaintainer := roles(tt.project, tt.user); isMember != tt.isMember || isMaintainer != tt.isMaintainer {
			t.Errorf("%s: member %v, maintainer %v; want %v and %v", tt.name, isMember, isMaintainer, tt.isMember, tt.isMaintainer)
		}
	}
	// Someone who never was a maintainer, or a project nobody registered: nothing to do, no error.
	if err := s.RemoveMaintainer(ctx, "aaaa", "gh3"); err != nil {
		t.Fatalf("RemoveMaintainer for a member: %v", err)
	}
	if err := s.RemoveMaintainer(ctx, "zzzz", "gh1"); err != nil {
		t.Fatalf("RemoveMaintainer in no project: %v", err)
	}
	if members, maintainers := count(t, pool, `SELECT count(*) FROM members WHERE project_id = 'aaaa'`), count(t, pool, `SELECT count(*) FROM maintainers WHERE project_id = 'aaaa'`); members != 2 || maintainers != 1 {
		t.Fatalf("project aaaa has %d members and %d maintainers, want 2 and 1", members, maintainers)
	}
}
