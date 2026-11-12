package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"officehours/guard"
	"officehours/nexconn"
	"officehours/queue"
	"officehours/store"
)

// The people and the one project every queue test starts with (newQueueApp).
const (
	queueProject = "prj1" // the project's id; its room is groupID(queueProject)
	hostID       = "gh1"  // a maintainer of it
	askerID      = "gh2"  // a member
	otherID      = "gh3"  // another member
	strangerID   = "gh9"  // signed in, but not in the room
	liveSession  = int64(12)
)

// fakeRoles answers the guard from fakeProjects' tables, like the store does from the real ones.
type fakeRoles struct{ projects *fakeProjects }

func (f fakeRoles) IsMember(_ context.Context, projectID, userID string) (bool, error) {
	return f.projects.members[projectID+" "+userID], f.projects.err
}

func (f fakeRoles) IsMaintainer(_ context.Context, projectID, userID string) (bool, error) {
	return f.projects.maintainers[projectID+" "+userID], f.projects.err
}

// fakeQueue stands in for the store behind the queue routes. It records every action it was asked
// to apply and answers with what the test put in; it knows no rules (those are queue.Step's).
type fakeQueue struct {
	sessions map[int64]string // session id → project id
	hands    map[int64]int64  // hand id → session id
	people   map[string]store.Person

	asked    []queue.Action   // every action Apply and Done got, in order
	drafts   []store.FAQDraft // every draft Done got
	refuse   error            // Apply and Done answer with it
	applied  store.Applied    // what Apply and Done answer; its project and session are filled in
	entry    store.FAQEntry   // what Done answers
	snapshot store.Snapshot   // what Queue answers
	err      error            // every read fails with it
}

func (f *fakeQueue) Queue(context.Context, string) (store.Snapshot, error) {
	return f.snapshot, f.err
}

func (f *fakeQueue) answer(sessionID int64, a queue.Action) (store.Applied, error) {
	f.asked = append(f.asked, a)
	if f.refuse != nil {
		return store.Applied{}, f.refuse
	}
	ap := f.applied
	ap.ProjectID, ap.SessionID = f.sessions[sessionID], sessionID
	return ap, nil
}

func (f *fakeQueue) Apply(_ context.Context, sessionID int64, a queue.Action) (store.Applied, error) {
	return f.answer(sessionID, a)
}

func (f *fakeQueue) Done(_ context.Context, sessionID int64, a queue.Action, draft store.FAQDraft) (store.Applied, store.FAQEntry, error) {
	f.drafts = append(f.drafts, draft)
	ap, err := f.answer(sessionID, a)
	if err != nil {
		return store.Applied{}, store.FAQEntry{}, err
	}
	return ap, f.entry, nil
}

func (f *fakeQueue) ProjectOfSession(_ context.Context, sessionID int64) (string, bool, error) {
	projectID, found := f.sessions[sessionID]
	return projectID, found, f.err
}

func (f *fakeQueue) ProjectOfHand(_ context.Context, handID int64) (string, int64, bool, error) {
	sessionID, found := f.hands[handID]
	return f.sessions[sessionID], sessionID, found, f.err
}

func (f *fakeQueue) People(_ context.Context, userIDs []string) ([]store.Person, error) {
	var out []store.Person
	for _, id := range userIDs {
		if p, ok := f.people[id]; ok {
			out = append(out, p)
		}
	}
	return out, f.err
}

// fakeFAQ stands in for the store behind the FAQ routes and the public FAQ pages. A test puts its
// entries in; lists come back newest first, and a "search" keeps the entries whose title holds
// the query, which is enough to tell that the query arrived.
type fakeFAQ struct {
	entries []store.FAQEntry // every entry, deleted ones too
	asked   []string         // what was asked, in order: "list prj1 q=retry limit=5", "delete 7", …
	err     error            // every call fails with it
}

func (f *fakeFAQ) matching(projectID, query string, limit int) []store.FAQEntry {
	var out []store.FAQEntry
	for _, e := range f.entries {
		if e.ProjectID == projectID && !e.Deleted && strings.Contains(strings.ToLower(e.Title), strings.ToLower(query)) {
			out = append(out, e)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	if len(out) > limit {
		out = out[:limit]
	}
	return out
}

func (f *fakeFAQ) FAQList(_ context.Context, projectID, query string, limit int) ([]store.FAQEntry, error) {
	f.asked = append(f.asked, fmt.Sprintf("list %s q=%s limit=%d", projectID, query, limit))
	return f.matching(projectID, query, limit), f.err
}

func (f *fakeFAQ) FAQSuggest(_ context.Context, projectID, text string, limit int) ([]store.FAQEntry, error) {
	f.asked = append(f.asked, fmt.Sprintf("suggest %s text=%s limit=%d", projectID, text, limit))
	return f.matching(projectID, "", limit), f.err
}

func (f *fakeFAQ) FAQBySlug(_ context.Context, projectID, slug string) (store.FAQEntry, bool, error) {
	for _, e := range f.entries {
		if e.ProjectID == projectID && e.Slug == slug {
			return e, true, f.err
		}
	}
	return store.FAQEntry{}, false, f.err
}

func (f *fakeFAQ) ProjectOfFAQ(_ context.Context, id int64) (string, bool, error) {
	for _, e := range f.entries {
		if e.ID == id {
			return e.ProjectID, true, f.err
		}
	}
	return "", false, f.err
}

func (f *fakeFAQ) DeleteFAQ(_ context.Context, id int64) error {
	f.asked = append(f.asked, fmt.Sprintf("delete %d", id))
	for i := range f.entries {
		if f.entries[i].ID == id {
			f.entries[i].Deleted = true
		}
	}
	return f.err
}

// fakeChat records what the server said in which room. tell sends at the same time, so it locks.
type fakeChat struct {
	mu    sync.Mutex
	sent  []nexconn.Message
	muted []string // "channel user minutes"
	err   error    // every call fails with it
}

func (f *fakeChat) Send(_ context.Context, m nexconn.Message) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sent = append(f.sent, m)
	return "msg", f.err
}

func (f *fakeChat) Mute(_ context.Context, channelID, userID string, minutes int32) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.muted = append(f.muted, fmt.Sprintf("%s %s %d", channelID, userID, minutes))
	return f.err
}

// types lists the types of the messages sent so far, sorted: they go out at the same time.
func (f *fakeChat) types() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var types []string
	for _, m := range f.sent {
		types = append(types, m.Type)
	}
	sort.Strings(types)
	return strings.Join(types, " ")
}

// one returns the only message of a type sent so far, and fails the test if there is not one.
func (f *fakeChat) one(t *testing.T, msgType string) nexconn.Message {
	t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	var found []nexconn.Message
	for _, m := range f.sent {
		if m.Type == msgType {
			found = append(found, m)
		}
	}
	if len(found) != 1 {
		t.Fatalf("%d %s messages were sent, want 1 (all: %+v)", len(found), msgType, f.sent)
	}
	return found[0]
}

// queueApp is the server with one registered project, PaperPlane/Stash, and fakes behind every
// queue and FAQ route: the tables of api_projects_test.go (which the guard reads its roles from),
// a queue store that records what it was asked, a FAQ store, a chat that records what was said,
// and the fake GitHub. hostID maintains the project; askerID and otherID are members; strangerID
// is neither. The project has one session, liveSession, no hands and no FAQ entries: a test adds
// what it needs. A test that needs a fake of its own puts it into a.deps and calls a.remux().
type queueApp struct {
	*projectsApp
	queue *fakeQueue
	faq   *fakeFAQ
	chat  *fakeChat
}

func newQueueApp(t *testing.T) *queueApp {
	t.Helper()
	base := newProjectsApp(t)
	roles := fakeRoles{base.projects}
	g, err := guard.New(testSite, base.codec, roles)
	if err != nil {
		t.Fatalf("guard.New: %v", err)
	}
	p := store.Project{ID: queueProject, GitHubID: int64(githubID("PaperPlane", "Stash")), Owner: "PaperPlane", Repo: "Stash", Description: "A tiny HTTP cache."}
	base.projects.byRepo[repoKey(p.Owner, p.Repo)] = p
	base.projects.maintainers[p.ID+" "+hostID] = true
	for _, id := range []string{hostID, askerID, otherID} {
		base.projects.join(p.ID, id)
	}
	a := &queueApp{
		projectsApp: base,
		chat:        &fakeChat{},
		faq:         &fakeFAQ{},
		queue: &fakeQueue{
			sessions: map[int64]string{liveSession: queueProject},
			hands:    map[int64]int64{},
			people:   map[string]store.Person{},
		},
	}
	for _, id := range []string{hostID, askerID, otherID, strangerID} {
		a.queue.people[id] = person(id)
	}
	a.deps.guard, a.deps.queue, a.deps.faq, a.deps.chat = g, a.queue, a.faq, a.chat
	a.remux()
	return a
}

// remux builds the routes again, after a test has put a fake of its own into a.deps.
func (a *queueApp) remux() { a.mux = newMux(config{AppURL: testSite}, a.deps) }

// person is a user as the queue shows them; as(t, id) signs the same person in.
func person(id string) store.Person {
	return store.Person{UserID: id, Login: "login-" + id, Name: "Name " + id, AvatarURL: "https://avatars.example/" + id}
}

func TestQueueRefusal(t *testing.T) {
	for _, c := range []struct {
		err    error
		status int
		says   string
	}{
		{&queue.TooSoonError{Wait: 39400 * time.Millisecond}, http.StatusTooManyRequests, "Wait 40 s"}, // FR-HAND-6
		{&queue.TooSoonError{Wait: 200 * time.Millisecond}, http.StatusTooManyRequests, "Wait 1 s"},
		{queue.ErrNotLive, http.StatusConflict, "no live session"},
		{queue.ErrHandsClosed, http.StatusConflict, "Hands are closed"}, // FR-SESS-6, AT-18
		{queue.ErrAlreadyOpen, http.StatusConflict, "already have an open hand"},
		{queue.ErrNoSuchHand, http.StatusConflict, "no longer in the queue"},
		{queue.ErrNotYours, http.StatusForbidden, "Only the person who raised"},
		{queue.ErrOwnHand, http.StatusConflict, "your own hand"},
		{queue.ErrNothingWaiting, http.StatusConflict, "Nobody is waiting"},
		{queue.ErrAnswering, http.StatusConflict, "Finish it first"},
		{queue.ErrNotAnswering, http.StatusConflict, "not being answered"},
		{queue.ErrIsHere, http.StatusConflict, "said they are here"}, // FR-QUEUE-9, AT-17
		{queue.ErrBadStatus, http.StatusConflict, "any more"},
		{store.ErrSessionLive, http.StatusConflict, "already live"}, // FR-SESS-2
		{queue.ErrNotImplemented, http.StatusNotImplemented, "isn't built yet"},
		{store.ErrNotBuilt, http.StatusNotImplemented, "isn't built yet"},
		{fmt.Errorf("store: apply: %w", queue.ErrNotYours), http.StatusForbidden, "Only the person who raised"},
	} {
		var refusal *guard.Error
		if got := queueRefusal(c.err); !errors.As(got, &refusal) || refusal.Status != c.status || !strings.Contains(refusal.Msg, c.says) {
			t.Errorf("queueRefusal(%v) = %v, want status %d and a sentence with %q", c.err, got, c.status, c.says)
		}
	}
	// Anything the rules did not refuse is our fault, and stays an error for writeError's 500.
	broken := errors.New("connection reset")
	if got := queueRefusal(broken); got != broken {
		t.Errorf("queueRefusal(%v) = %v, want the same error back", broken, got)
	}
}

func TestDescribeQueue(t *testing.T) {
	at := time.Date(2026, 11, 14, 11, 30, 0, 0, time.UTC)
	snap := store.Snapshot{
		Session: &store.Session{ID: liveSession, ProjectID: queueProject, Number: 3, Title: "Thursday office hours", Live: true, PlannedMinutes: 45, StartedAt: at, Version: 9, Host: person(hostID)},
		Open: []store.QueueHand{
			{Hand: queue.Hand{ID: 101, AuthorID: askerID, Title: "How do I retry only on 5xx?", Status: queue.Answering, RaisedAt: at, TakenAt: at.Add(time.Minute), TakenBy: hostID}, Author: person(askerID), Taker: person(hostID)},
			{Hand: queue.Hand{ID: 102, AuthorID: otherID, Title: "Can the cache be shared?", Status: queue.Waiting, RaisedAt: at, Plus: []string{askerID}}, Author: person(otherID)},
		},
		Closed: []store.QueueHand{
			{Hand: queue.Hand{ID: 100, AuthorID: otherID, Title: "Where are the docs?", Status: queue.Answered, ClosedAt: at}, Author: person(otherID), FAQID: 7, FAQSlug: "where-are-the-docs"},
			{Hand: queue.Hand{ID: 99, AuthorID: otherID, Title: "Buy cheap watches", Details: "spam.example", Status: queue.Removed, ClosedAt: at}, Author: person(otherID)},
		},
		MedianAnswer: 150 * time.Second,
	}

	t.Run("FR-QUEUE-4: a member sees every hand, which one is theirs and which they +1'd", func(t *testing.T) {
		q := describeQueue(snap, askerID, at)
		if q.Session == nil || q.Session.ID != "12" || q.Session.Version != 9 || !q.Session.Live || q.Session.Host == nil || q.Session.Host.Login != "login-gh1" || q.Session.EndedAt != nil {
			t.Fatalf("session = %+v", q.Session)
		}
		if len(q.Hands) != 2 || len(q.Done) != 2 {
			t.Fatalf("%d open and %d done hands, want 2 and 2", len(q.Hands), len(q.Done))
		}
		mine, theirs := q.Hands[0], q.Hands[1]
		if mine.ID != "101" || !mine.IsMine || mine.PlusByMe || mine.Status != "answering" || mine.TakenAt == nil || mine.TakenBy == nil || mine.AckedAt != nil {
			t.Errorf("my hand = %+v", mine)
		}
		if theirs.IsMine || !theirs.PlusByMe || theirs.PlusCount != 1 || theirs.TakenAt != nil || theirs.TakenBy != nil {
			t.Errorf("their hand = %+v", theirs)
		}
		if q.Done[0].FAQID != "7" || q.Done[0].FAQSlug != "where-are-the-docs" || q.MedianAnswerSeconds != 150 || !q.ServerTime.Equal(at) {
			t.Errorf("done = %+v, median %d, server time %v", q.Done[0], q.MedianAnswerSeconds, q.ServerTime)
		}
		if mine.FAQID != "" || mine.FAQSlug != "" {
			t.Errorf("an open hand names a FAQ entry: %+v", mine)
		}
	})

	t.Run("FR-MOD-1, AT-18: a removed hand keeps its status and loses its words, for everyone", func(t *testing.T) {
		for _, viewer := range []string{askerID, otherID, hostID} {
			gone := describeQueue(snap, viewer, at).Done[1]
			if gone.ID != "99" || gone.Status != "removed" || gone.Title != "" || gone.Details != "" {
				t.Errorf("as %s the removed hand is %+v, want its id and status and no text", viewer, gone)
			}
		}
	})

	t.Run("a visitor nobody knows owns no hand and +1'd none", func(t *testing.T) {
		for _, h := range describeQueue(snap, "", at).Hands {
			if h.IsMine || h.PlusByMe {
				t.Errorf("hand %s is marked as the visitor's: %+v", h.ID, h)
			}
		}
	})

	t.Run("no session yet: null and empty lists, never a missing field", func(t *testing.T) {
		text, err := json.Marshal(describeQueue(store.Snapshot{}, askerID, at))
		if err != nil {
			t.Fatal(err)
		}
		for _, want := range []string{`"session":null`, `"hands":[]`, `"done":[]`, `"nextSessionAt":null`, `"medianAnswerSeconds":0`} {
			if !strings.Contains(string(text), want) {
				t.Errorf("the JSON has no %s: %s", want, text)
			}
		}
	})
}

func TestQueuePage(t *testing.T) {
	const path = "/api/p/paperplane/stash/queue"
	seeded := func(t *testing.T) *queueApp {
		a := newQueueApp(t)
		a.queue.snapshot = store.Snapshot{
			Session: &store.Session{ID: liveSession, Title: "Thursday office hours", Live: true, Version: 2},
			Open:    []store.QueueHand{{Hand: queue.Hand{ID: 101, AuthorID: askerID, Title: "How do I retry only on 5xx?", Status: queue.Waiting}, Author: person(askerID)}},
		}
		return a
	}

	t.Run("FR-QUEUE-4: a member gets the session and its hands", func(t *testing.T) {
		a := seeded(t)
		res, body := call(a.mux, "GET", path, "", a.as(t, askerID))
		var q queueJSON
		decode(t, body, &q)
		if res.StatusCode != http.StatusOK || q.Session == nil || len(q.Hands) != 1 || !q.Hands[0].IsMine {
			t.Fatalf("status %d, queue %+v", res.StatusCode, q)
		}
	})

	t.Run("FR-PROJ-3: someone outside the room learns that a session is live, and no question or name", func(t *testing.T) {
		a := seeded(t)
		for name, cookie := range map[string]*http.Cookie{"signed out": nil, "not a member": a.as(t, strangerID)} {
			res, body := call(a.mux, "GET", path, "", cookie)
			var q queueJSON
			decode(t, body, &q)
			if res.StatusCode != http.StatusOK || q.Session == nil || !q.Session.Live || len(q.Hands) != 0 || strings.Contains(body, "retry only") || strings.Contains(body, "login-gh2") {
				t.Errorf("%s: status %d body %s", name, res.StatusCode, body)
			}
		}
	})

	t.Run("an unknown project is a 404, a store that fails a 500, a part that is not built a 501", func(t *testing.T) {
		a := seeded(t)
		res, body := call(a.mux, "GET", "/api/p/nobody/nothing/queue", "", nil)
		refused(t, "unknown project", res, body, http.StatusNotFound, "Nobody hosts office hours")
		a.queue.err = errors.New("connection reset")
		res, body = call(a.mux, "GET", path, "", a.as(t, askerID))
		refused(t, "store down", res, body, http.StatusInternalServerError, "Something went wrong")
		a.queue.err = store.ErrNotBuilt
		res, body = call(a.mux, "GET", path, "", a.as(t, askerID))
		refused(t, "not built", res, body, http.StatusNotImplemented, "isn't built yet")
	})
}

// A route of each kind, built from the shared helpers the way the real routes are.
func helperRoutes(a *queueApp) *http.ServeMux {
	d := a.deps
	mux := http.NewServeMux()
	act := func(start func(http.ResponseWriter, *http.Request, any, role) (target, bool), need role) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			t, ok := start(w, r, nil, need)
			if !ok {
				return
			}
			ap, err := d.queue.Apply(r.Context(), t.SessionID, queue.Action{Kind: queue.Next, ActorID: t.User.ID, HandID: t.HandID, At: now()})
			if err != nil {
				writeError(w, queueRefusal(err))
				return
			}
			d.finish(w, r, t, ap, botSays(`a "quoted" </script> line`))
		}
	}
	mux.HandleFunc("POST /t/sessions/{id}/member", act(d.onSession, asMember))
	mux.HandleFunc("POST /t/sessions/{id}/maintainer", act(d.onSession, asMaintainer))
	mux.HandleFunc("POST /t/hands/{id}/member", act(d.onHand, asMember))
	mux.HandleFunc("POST /t/hands/{id}/maintainer", act(d.onHand, asMaintainer))
	project := func(need role) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if u, p, ok := d.onProject(w, r, nil, need); ok {
				writeJSON(w, http.StatusOK, envelope{OK: true, Data: u.ID + " " + p.ID})
			}
		}
	}
	mux.HandleFunc("POST /t/p/{owner}/{repo}/member", project(asMember))
	mux.HandleFunc("POST /t/p/{owner}/{repo}/maintainer", project(asMaintainer))
	return mux
}

func TestQueueMutationChecks(t *testing.T) {
	t.Run("NFR-SEC-2, NFR-SEC-5: who is refused, and that nothing is applied or said for them", func(t *testing.T) {
		a := newQueueApp(t)
		a.queue.hands[101] = liveSession
		mux := helperRoutes(a)
		otherSite := func(r *http.Request) { r.Header.Set("Origin", "https://evil.example") }
		form := func(r *http.Request) { r.Header.Set("Content-Type", "application/x-www-form-urlencoded") }
		for _, c := range []struct {
			name, path string
			cookie     *http.Cookie
			mutate     func(*http.Request)
			status     int
			says       string
		}{
			{"another site", "/t/sessions/12/maintainer", a.as(t, hostID), otherSite, http.StatusForbidden, "did not come from Office Hours"},
			{"signed out", "/t/sessions/12/member", nil, nil, http.StatusUnauthorized, "Sign in first"},
			{"not JSON", "/t/sessions/12/member", a.as(t, askerID), form, http.StatusUnsupportedMediaType, "JSON"},
			{"an id that is no number", "/t/sessions/abc/member", a.as(t, askerID), nil, http.StatusNotFound, "no such session"},
			{"an unknown session", "/t/sessions/999/member", a.as(t, askerID), nil, http.StatusNotFound, "no such session"},
			{"an unknown hand", "/t/hands/999/member", a.as(t, askerID), nil, http.StatusNotFound, "no such hand"},
			{"AT-10: a member on a maintainer's session route", "/t/sessions/12/maintainer", a.as(t, askerID), nil, http.StatusForbidden, "Only a maintainer"},
			{"AT-10: a member on a maintainer's hand route", "/t/hands/101/maintainer", a.as(t, askerID), nil, http.StatusForbidden, "Only a maintainer"},
			{"someone outside the room on a session", "/t/sessions/12/member", a.as(t, strangerID), nil, http.StatusForbidden, "Join this room first"},
			{"someone outside the room on a hand", "/t/hands/101/member", a.as(t, strangerID), nil, http.StatusForbidden, "Join this room first"},
			{"a member on a maintainer's project route", "/t/p/paperplane/stash/maintainer", a.as(t, askerID), nil, http.StatusForbidden, "Only a maintainer"},
			{"someone outside the room on a project route", "/t/p/paperplane/stash/member", a.as(t, strangerID), nil, http.StatusForbidden, "Join this room first"},
			{"a project nobody registered", "/t/p/nobody/nothing/member", a.as(t, askerID), nil, http.StatusNotFound, "Nobody hosts office hours"},
		} {
			var mutate []func(*http.Request)
			if c.mutate != nil {
				mutate = append(mutate, c.mutate)
			}
			res, body := call(mux, "POST", c.path, "{}", c.cookie, mutate...)
			refused(t, c.name, res, body, c.status, c.says)
		}
		if len(a.queue.asked) != 0 || a.chat.types() != "" {
			t.Errorf("a refused caller reached the store (%+v) or the room (%s)", a.queue.asked, a.chat.types())
		}
	})

	t.Run("FR-QUEUE-4: an action that is applied is told to the room, and answers with the queue", func(t *testing.T) {
		a := newQueueApp(t)
		a.queue.hands[101] = liveSession
		a.queue.applied = store.Applied{Version: 7}
		a.queue.snapshot = store.Snapshot{Session: &store.Session{ID: liveSession, Live: true, Version: 7}}
		res, body := call(helperRoutes(a), "POST", "/t/hands/101/maintainer", "{}", a.as(t, hostID))
		var q queueJSON
		decode(t, body, &q)
		if res.StatusCode != http.StatusOK || q.Session == nil || q.Session.Version != 7 {
			t.Fatalf("status %d body %s", res.StatusCode, body)
		}
		if got := a.queue.asked; len(got) != 1 || got[0].ActorID != hostID || got[0].HandID != 101 || got[0].At.IsZero() || got[0].At.Nanosecond()%1000 != 0 {
			t.Fatalf("applied %+v, want one action by the host on hand 101 at a time in whole microseconds", got)
		}
		if a.chat.types() != "OH:Queue RC:TxtMsg" {
			t.Fatalf("the room got %q, want the ping and the line", a.chat.types())
		}
		ping, line := a.chat.one(t, typeQueue), a.chat.one(t, typeText)
		for _, m := range []nexconn.Message{ping, line} {
			if m.From != botID || m.ChannelID != groupID(queueProject) {
				t.Errorf("FR-QUEUE-7: %s was sent by %q into %q, want the bot into the project's room", m.Type, m.From, m.ChannelID)
			}
		}
		var sent struct {
			SessionID string `json:"sessionId"`
			Version   int    `json:"version"`
			SentAt    int64  `json:"sentAt"`
		}
		if err := json.Unmarshal([]byte(ping.Content), &sent); err != nil || sent.SessionID != "12" || sent.Version != 7 || sent.SentAt == 0 || !ping.Transient {
			t.Errorf("ping = %+v (%v), want session \"12\", version 7, a send time, not stored", ping, err)
		}
		var said struct {
			Content string `json:"content"`
		}
		if err := json.Unmarshal([]byte(line.Content), &said); err != nil || said.Content != `a "quoted" </script> line` {
			t.Errorf("NFR-SEC-6: the line arrived as %q (%v), want its text unchanged inside one JSON field", line.Content, err)
		}
	})

	t.Run("a refusal of the rules is a sentence, and the room hears nothing", func(t *testing.T) {
		a := newQueueApp(t)
		a.queue.refuse = queue.ErrAnswering
		res, body := call(helperRoutes(a), "POST", "/t/sessions/12/maintainer", "{}", a.as(t, hostID))
		refused(t, "refused by the rules", res, body, http.StatusConflict, "Finish it first")
		if a.chat.types() != "" {
			t.Errorf("the room got %q after a refusal", a.chat.types())
		}
	})

	t.Run("ARCHITECTURE §6: a room that cannot be told does not fail an action that is written", func(t *testing.T) {
		a := newQueueApp(t)
		a.chat.err = errors.New("nexconn is down")
		res, body := call(helperRoutes(a), "POST", "/t/sessions/12/maintainer", "{}", a.as(t, hostID))
		if res.StatusCode != http.StatusOK {
			t.Fatalf("status %d body %s, want 200", res.StatusCode, body)
		}
	})

	t.Run("a server without a chat or a queue store says so", func(t *testing.T) {
		a := newQueueApp(t)
		a.deps.chat = nil
		if res, body := call(helperRoutes(a), "POST", "/t/sessions/12/maintainer", "{}", a.as(t, hostID)); res.StatusCode != http.StatusOK {
			t.Errorf("without a chat: status %d body %s, want 200 (the queue still works)", res.StatusCode, body)
		}
		a.deps.queue = nil
		res, body := call(helperRoutes(a), "POST", "/t/sessions/12/maintainer", "{}", a.as(t, hostID))
		refused(t, "without a queue store", res, body, http.StatusServiceUnavailable, "isn't set up")
	})
}

func TestRaisedHandAndHandIn(t *testing.T) {
	old := queue.Hand{ID: 101, AuthorID: otherID, Status: queue.Waiting}
	fresh := queue.Hand{ID: 102, AuthorID: askerID, Status: queue.Waiting}
	ap := store.Applied{Before: queue.State{Hands: []queue.Hand{old}}, After: queue.State{Hands: []queue.Hand{old, fresh}}}
	if h, ok := raisedHand(ap); !ok || h.ID != 102 {
		t.Errorf("raisedHand = %+v, %v; want hand 102", h, ok)
	}
	if _, ok := raisedHand(store.Applied{Before: ap.Before, After: ap.Before}); ok {
		t.Error("raisedHand found a new hand where none was raised")
	}
	if h, ok := handIn(ap.After, 101); !ok || h.AuthorID != otherID {
		t.Errorf("handIn(101) = %+v, %v", h, ok)
	}
	if _, ok := handIn(ap.Before, 102); ok {
		t.Error("handIn found a hand that is not in the state")
	}
}

func TestCards(t *testing.T) {
	read := func(t *testing.T, m nexconn.Message) map[string]any {
		t.Helper()
		var out map[string]any
		if err := json.Unmarshal([]byte(m.Content), &out); err != nil {
			t.Fatalf("%s content is not JSON: %v in %s", m.Type, err, m.Content)
		}
		return out
	}
	const nasty = `Why does "retry" fail? </script><b>x</b>`

	t.Run("FR-SESS-3: the start card mentions everyone and carries a push line", func(t *testing.T) {
		m := sessionStarted(store.Session{ID: 12, Title: nasty})
		c := read(t, m)
		mention, _ := c["mentionedInfo"].(map[string]any)
		if m.Type != typeSession || !m.Mention || m.Push == "" || m.Transient || c["sessionId"] != "12" || c["title"] != nasty || c["state"] != "live" || mention["type"] != float64(1) {
			t.Errorf("start card = %+v", m)
		}
	})

	t.Run("FR-SESS-4: the end card counts what became of the hands, and mentions nobody", func(t *testing.T) {
		m := sessionEnded(12, "Thursday office hours", 9, 2, 1)
		c := read(t, m)
		if m.Mention || c["state"] != "ended" || c["answered"] != float64(9) || c["carried"] != float64(2) || c["notReached"] != float64(1) || c["mentionedInfo"] != nil {
			t.Errorf("end card = %+v", m)
		}
	})

	t.Run("FR-HAND-5: the hand card names the hand, its session and its author", func(t *testing.T) {
		m := handRaised(12, queue.Hand{ID: 101, Title: nasty}, "mia-k")
		c := read(t, m)
		if m.Type != typeHand || c["handId"] != "101" || c["sessionId"] != "12" || c["title"] != nasty || c["authorLogin"] != "mia-k" || !strings.Contains(m.Push, "@mia-k") {
			t.Errorf("hand card = %+v", m)
		}
	})

	t.Run("FR-FAQ-2, NFR-SEC-6: the FAQ card carries a slug and no address", func(t *testing.T) {
		m := faqAdded(store.FAQEntry{ID: 7, Slug: "why-does-retry-fail", Title: nasty}, []string{"mia-k", "diego-ruiz"})
		c := read(t, m)
		asked, _ := c["askedBy"].([]any)
		if m.Type != typeFAQ || c["faqId"] != "7" || c["slug"] != "why-does-retry-fail" || c["title"] != nasty || len(asked) != 2 || strings.Contains(m.Content, "http") {
			t.Errorf("FAQ card = %+v", m)
		}
		if !strings.Contains(faqAdded(store.FAQEntry{ID: 7}, nil).Content, `"askedBy":[]`) {
			t.Error("a FAQ card with no askers must carry an empty list, not null")
		}
	})
}
