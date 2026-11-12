package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"officehours/nexconn"
	"officehours/queue"
	"officehours/store"
)

const (
	sessionStartPath    = "/api/p/paperplane/stash/sessions"
	sessionNextPath     = "/api/p/paperplane/stash/next-session"
	sessionEndPath      = "/api/sessions/12/end"
	sessionLastCallPath = "/api/sessions/12/lastcall"

	sessionStartBody = `{"title":"Thursday office hours","plannedMinutes":45}`
	sessionStartedID = int64(13) // the id sessionFakeStarts gives a session it starts
	sessionHostRole  = queueProject + " " + hostID
)

// sessionRoutes are the four routes of this file, each with a body it accepts.
var sessionRoutes = []struct{ path, body string }{
	{sessionStartPath, sessionStartBody},
	{sessionNextPath, `{"at":""}`},
	{sessionEndPath, `{"carry":false}`},
	{sessionLastCallPath, `{"closed":true}`},
}

// sessionFakeStarts stands in for the store behind the session routes. It records what it was
// asked and, like the real store, shows the outcome where the routes read it back: a started
// session and a planned time in the queue, a role that was taken away in the tables the guard
// reads.
type sessionFakeStarts struct {
	queue *fakeQueue
	roles *fakeProjects

	asked    []store.NewSession // every StartSession, in order
	planned  []time.Time        // every SetNextSession, in order; the zero time clears
	removed  []string           // every RemoveMaintainer: "project user"
	startErr error              // StartSession refuses with it
	err      error              // SetNextSession and RemoveMaintainer fail with it
}

func (f *sessionFakeStarts) StartSession(_ context.Context, n store.NewSession) (store.Session, error) {
	f.asked = append(f.asked, n)
	if f.startErr != nil {
		return store.Session{}, f.startErr
	}
	started := store.Session{
		ID: sessionStartedID, ProjectID: n.ProjectID, Number: 4, Title: n.Title, Live: true,
		PlannedMinutes: n.PlannedMinutes, StartedAt: now(), Version: 1, Host: person(n.HostID),
	}
	f.queue.snapshot.Session = &started
	return started, nil
}

func (f *sessionFakeStarts) SetNextSession(_ context.Context, _ string, at time.Time) error {
	if f.err != nil {
		return f.err
	}
	f.planned = append(f.planned, at)
	f.queue.snapshot.NextSessionAt = at
	return nil
}

func (f *sessionFakeStarts) RemoveMaintainer(_ context.Context, projectID, userID string) error {
	if f.err != nil {
		return f.err
	}
	f.removed = append(f.removed, projectID+" "+userID)
	delete(f.roles.maintainers, projectID+" "+userID)
	return nil
}

// sessionFakeBlindQueue is the queue store on a day when an action is written and the queue
// cannot be read afterwards.
type sessionFakeBlindQueue struct{ *fakeQueue }

func (sessionFakeBlindQueue) Queue(context.Context, string) (store.Snapshot, error) {
	return store.Snapshot{}, errors.New("connection reset")
}

// sessionFakeLeavingQueue is the queue store while the caller goes away the moment their action
// is written: from then on, whatever still uses the request's context fails.
type sessionFakeLeavingQueue struct {
	*fakeQueue
	leave context.CancelFunc
}

func (f sessionFakeLeavingQueue) Apply(ctx context.Context, sessionID int64, a queue.Action) (store.Applied, error) {
	defer f.leave()
	return f.fakeQueue.Apply(ctx, sessionID, a)
}

func (f sessionFakeLeavingQueue) Queue(ctx context.Context, projectID string) (store.Snapshot, error) {
	if err := ctx.Err(); err != nil {
		return store.Snapshot{}, err
	}
	return f.fakeQueue.Queue(ctx, projectID)
}

// sessionApp is newQueueApp with a fake session store behind the session routes.
func sessionApp(t *testing.T) (*queueApp, *sessionFakeStarts) {
	t.Helper()
	a := newQueueApp(t)
	starts := &sessionFakeStarts{queue: a.queue, roles: a.projects}
	a.deps.starts = starts
	a.remux()
	return a, starts
}

// sessionUntouched fails the test if a refused request reached a store or the room.
func sessionUntouched(t *testing.T, name string, a *queueApp, starts *sessionFakeStarts) {
	t.Helper()
	if len(starts.asked) != 0 || len(starts.planned) != 0 || len(starts.removed) != 0 || len(a.queue.asked) != 0 {
		t.Errorf("%s: a refused request reached the store: started %+v, planned %v, removed %v, applied %+v",
			name, starts.asked, starts.planned, starts.removed, a.queue.asked)
	}
	if said := a.chat.types(); said != "" {
		t.Errorf("%s: a refused request was told to the room: %s", name, said)
	}
}

// sessionAction returns the one action the queue store was asked to apply, and fails the test if
// it is not by the host, of the wanted kind and at a time from now(): UTC, in whole microseconds.
// (Some clocks tick in microseconds anyway, so the zone is what tells now() from time.Now().)
func sessionAction(t *testing.T, a *queueApp, kind queue.Kind) queue.Action {
	t.Helper()
	if len(a.queue.asked) != 1 {
		t.Fatalf("%d actions were applied, want 1: %+v", len(a.queue.asked), a.queue.asked)
	}
	got := a.queue.asked[0]
	if got.Kind != kind || got.ActorID != hostID || got.HandID != 0 || got.At.IsZero() || got.At.Location() != time.UTC || got.At.Nanosecond()%1000 != 0 {
		t.Fatalf("applied %+v, want one %s by the host at a time from now()", got, kind)
	}
	return got
}

// sessionCardOf reads an OH:Session card, and fails the test unless the bot sent it into the
// project's room (FR-QUEUE-7).
func sessionCardOf(t *testing.T, m nexconn.Message) sessionCardContent {
	t.Helper()
	if m.From != botID || m.ChannelID != groupID(queueProject) {
		t.Errorf("FR-QUEUE-7: the card was sent by %q into %q, want the bot into the project's room", m.From, m.ChannelID)
	}
	var card sessionCardContent
	if err := json.Unmarshal([]byte(m.Content), &card); err != nil {
		t.Fatalf("the card's content is not JSON: %v in %s", err, m.Content)
	}
	return card
}

// sessionPingOf reads the session id and the version of the one ping the room got.
func sessionPingOf(t *testing.T, a *queueApp) (sessionID string, version int) {
	t.Helper()
	var ping struct {
		SessionID string `json:"sessionId"`
		Version   int    `json:"version"`
	}
	if err := json.Unmarshal([]byte(a.chat.one(t, typeQueue).Content), &ping); err != nil {
		t.Fatalf("the ping's content is not JSON: %v", err)
	}
	return ping.SessionID, ping.Version
}

func TestSessionStart(t *testing.T) {
	t.Run("FR-SESS-1, FR-SESS-3: a maintainer starts a session; the room gets the card with @all and the ping", func(t *testing.T) {
		a, starts := sessionApp(t)
		res, body := call(a.mux, "POST", sessionStartPath, `{"title":"  Thursday   office\n hours ","plannedMinutes":45}`, a.as(t, hostID))
		var q queueJSON
		decode(t, body, &q)
		if res.StatusCode != http.StatusOK || q.Session == nil {
			t.Fatalf("status %d body %s, want 200 and the queue with the new session", res.StatusCode, body)
		}
		if s := q.Session; s.ID != "13" || s.Number != 4 || s.Title != "Thursday office hours" || !s.Live || s.PlannedMinutes != 45 || s.Version != 1 || s.Host == nil || s.Host.UserID != hostID {
			t.Errorf("the answer's session = %+v, host %+v", s, s.Host)
		}
		// The title is stored as one clean line, and the host is who the session says is signed in.
		if want := []store.NewSession{{ProjectID: queueProject, HostID: hostID, Title: "Thursday office hours", PlannedMinutes: 45}}; !slices.Equal(starts.asked, want) {
			t.Errorf("the store was asked to start %+v, want %+v", starts.asked, want)
		}
		// Plan decision 7: GitHub was asked about the project's own repository, with the caller's token.
		if want := []string{"Bearer " + testUserToken + " /repos/PaperPlane/Stash"}; !slices.Equal(a.gh.asked, want) {
			t.Errorf("GitHub was asked %q, want %q", a.gh.asked, want)
		}
		if len(starts.removed) != 0 || !a.projects.maintainers[sessionHostRole] {
			t.Errorf("a maintainer with rights lost the role: removed %v", starts.removed)
		}

		if said := a.chat.types(); said != "OH:Queue OH:Session" {
			t.Fatalf("the room got %q, want the card and the ping", said)
		}
		m := a.chat.one(t, typeSession)
		card := sessionCardOf(t, m)
		if !m.Mention || m.Transient || !strings.Contains(m.Push, "Thursday office hours") || card.SessionID != "13" || card.Title != "Thursday office hours" ||
			card.State != "live" || card.MentionedInfo == nil || card.MentionedInfo.Type != 1 {
			t.Errorf("FR-SESS-3: the start card = %+v with content %+v, want a mention of everyone", m, card)
		}
		if id, version := sessionPingOf(t, a); id != "13" || version != 1 {
			t.Errorf("FR-QUEUE-4: the ping names session %q at version %d, want 13 at 1", id, version)
		}
		if strings.Contains(body, testUserToken) {
			t.Errorf("NFR-SEC-1: the reply contains the user's GitHub token: %s", body)
		}
	})

	t.Run("FR-SESS-1: the title is 5 to 80 characters, the planned length 15 to 180 minutes or none", func(t *testing.T) {
		for _, tt := range []struct {
			name    string
			body    string
			status  int
			says    string // for a refusal
			title   string // for a start: what the store gets
			minutes int
		}{
			{"FR-SESS-1: 5 characters are enough, and a length is optional", `{"title":"Hello"}`, 200, "", "Hello", 0},
			{"FR-SESS-1: 80 characters fit", `{"title":"` + strings.Repeat("a", 80) + `"}`, 200, "", strings.Repeat("a", 80), 0},
			{"FR-SESS-1: characters are counted, not bytes", `{"title":"` + strings.Repeat("é", 80) + `"}`, 200, "", strings.Repeat("é", 80), 0},
			{"FR-SESS-1: five characters of another script", `{"title":"オフィス時"}`, 200, "", "オフィス時", 0},
			{"FR-SESS-1: spaces at the ends and runs of spaces count once", `{"title":"  a   b \t c  "}`, 200, "", "a b c", 0},
			{"FR-SESS-1: 15 minutes", `{"title":"Thursday office hours","plannedMinutes":15}`, 200, "", "Thursday office hours", 15},
			{"FR-SESS-1: 180 minutes", `{"title":"Thursday office hours","plannedMinutes":180}`, 200, "", "Thursday office hours", 180},
			{"FR-SESS-1: no length planned", `{"title":"Thursday office hours","plannedMinutes":0}`, 200, "", "Thursday office hours", 0},

			{"FR-SESS-1: 4 characters are too few", `{"title":" abcd "}`, 400, "5 to 80 characters", "", 0},
			{"FR-SESS-1: spaces alone are no title", `{"title":"          "}`, 400, "5 to 80 characters", "", 0},
			{"FR-SESS-1: no title at all", `{"plannedMinutes":45}`, 400, "5 to 80 characters", "", 0},
			{"FR-SESS-1: 81 characters are too many", `{"title":"` + strings.Repeat("a", 81) + `"}`, 400, "5 to 80 characters", "", 0},
			{"FR-SESS-1: 14 minutes are too short", `{"title":"Thursday office hours","plannedMinutes":14}`, 400, "15 to 180 minutes", "", 0},
			{"FR-SESS-1: 181 minutes are too long", `{"title":"Thursday office hours","plannedMinutes":181}`, 400, "15 to 180 minutes", "", 0},
			{"FR-SESS-1: a negative length", `{"title":"Thursday office hours","plannedMinutes":-45}`, 400, "15 to 180 minutes", "", 0},
			{"NFR-SEC-3: minutes as text", `{"title":"Thursday office hours","plannedMinutes":"45"}`, 400, "not the JSON", "", 0},
			{"NFR-SEC-3: half a minute", `{"title":"Thursday office hours","plannedMinutes":45.5}`, 400, "not the JSON", "", 0},
			{"NFR-SEC-3: a title that is a number", `{"title":12345}`, 400, "not the JSON", "", 0},
			{"NFR-SEC-3: a field we don't have", `{"title":"Thursday office hours","live":true}`, 400, "not the JSON", "", 0},
			{"NFR-SEC-3: no JSON object", `"Thursday office hours"`, 400, "not the JSON", "", 0},
		} {
			t.Run(tt.name, func(t *testing.T) {
				a, starts := sessionApp(t)
				res, body := call(a.mux, "POST", sessionStartPath, tt.body, a.as(t, hostID))
				if tt.status != http.StatusOK {
					refused(t, tt.name, res, body, tt.status, tt.says)
					// Nothing is asked of GitHub for a request that could never start a session.
					if len(a.gh.asked) != 0 {
						t.Errorf("GitHub was asked %q for a request that was refused for its input", a.gh.asked)
					}
					sessionUntouched(t, tt.name, a, starts)
					return
				}
				want := []store.NewSession{{ProjectID: queueProject, HostID: hostID, Title: tt.title, PlannedMinutes: tt.minutes}}
				if res.StatusCode != http.StatusOK || !slices.Equal(starts.asked, want) {
					t.Fatalf("status %d body %s; the store was asked to start %+v, want %+v", res.StatusCode, body, starts.asked, want)
				}
			})
		}
	})

	t.Run("FR-SESS-2: a session that is live already is in the way, and the room hears nothing", func(t *testing.T) {
		a, starts := sessionApp(t)
		starts.startErr = store.ErrSessionLive
		res, body := call(a.mux, "POST", sessionStartPath, sessionStartBody, a.as(t, hostID))
		refused(t, "a live session", res, body, http.StatusConflict, "already live")
		// The store decides, with its unique index: the route does not look first.
		if len(starts.asked) != 1 || a.chat.types() != "" {
			t.Errorf("the store was asked %d times and the room got %q; want one try and silence", len(starts.asked), a.chat.types())
		}
	})

	t.Run("a session whose user row is gone signs in again; a database that fails is a 500 without its text", func(t *testing.T) {
		a, starts := sessionApp(t)
		starts.startErr = store.ErrUnknownUser
		res, body := call(a.mux, "POST", sessionStartPath, sessionStartBody, a.as(t, hostID))
		refused(t, "unknown user", res, body, http.StatusUnauthorized, "Sign in again")

		starts.startErr = errors.New("connection refused to 10.0.0.5:5432")
		res, body = call(a.mux, "POST", sessionStartPath, sessionStartBody, a.as(t, hostID))
		refused(t, "database down", res, body, http.StatusInternalServerError, "Try again")
		if strings.Contains(body, "10.0.0.5") || a.chat.types() != "" {
			t.Errorf("NFR-OPS-2: body %s, the room got %q; nothing may leak or be announced", body, a.chat.types())
		}
	})

	t.Run("ARCHITECTURE §6: a room that cannot be told does not undo a session that started", func(t *testing.T) {
		a, starts := sessionApp(t)
		a.chat.err = errors.New("nexconn is down")
		res, body := call(a.mux, "POST", sessionStartPath, sessionStartBody, a.as(t, hostID))
		if res.StatusCode != http.StatusOK || len(starts.asked) != 1 {
			t.Fatalf("status %d body %s, want 200", res.StatusCode, body)
		}
	})

	t.Run("a server without a session store, a queue store or a GitHub app says it is not set up", func(t *testing.T) {
		for name, strip := range map[string]func(*deps){
			"no session store": func(d *deps) { d.starts = nil },
			"no queue store":   func(d *deps) { d.queue = nil },
			"no GitHub app":    func(d *deps) { d.github = nil },
		} {
			a, starts := sessionApp(t)
			strip(&a.deps)
			a.remux()
			res, body := call(a.mux, "POST", sessionStartPath, sessionStartBody, a.as(t, hostID))
			refused(t, name, res, body, http.StatusServiceUnavailable, "set up")
			if len(a.gh.asked) != 0 {
				t.Errorf("%s: GitHub was asked %q", name, a.gh.asked)
			}
			sessionUntouched(t, name, a, starts)
		}
	})
}

func TestSessionStartAsksGitHubAgain(t *testing.T) {
	const (
		theRepo   = "PaperPlane/Stash" // the name the room is registered under
		elsewhere = 2002               // the id of another repository
	)
	private := `{"id":1001,"name":"Stash","private":true,"owner":{"login":"PaperPlane","type":"Organization"},"description":null,"permissions":` + adminRights + `}`
	rateLimit := `{"message":"You have exceeded a rate limit."}`

	type outcome int
	const (
		yes     outcome = iota // the session starts
		clearNo                // refused, and the role is gone
		hiccup                 // refused, and nothing is removed
	)
	for _, tt := range []struct {
		name   string
		status int // GitHub's
		reply  string
		header map[string]string
		hang   bool
		want   outcome
		code   int
		says   string
		never  string // words that would mislead here
	}{
		// Yes: GitHub answered for the project's own repository and reports one of the two rights.
		{"NFR-SEC-2: maintain rights are a yes", 200, githubRepo("PaperPlane", "Organization", "Stash", maintainRights), nil, false, yes, 200, "", ""},
		{"NFR-SEC-2: admin rights are a yes", 200, githubRepo("PaperPlane", "Organization", "Stash", adminRights), nil, false, yes, 200, "", ""},
		{"NFR-SEC-2: the same repository under a new name is a yes", 200, withID(githubRepo("PaperPlane", "Organization", "Cache", maintainRights), 1001), nil, false, yes, 200, "", ""},

		// A clear no: the same repository, and neither right.
		{"NFR-SEC-2: push rights on an organization's repository are a clear no", 200, githubRepo("PaperPlane", "Organization", "Stash", pushRights), nil, false, clearNo, 403, "ask an owner of PaperPlane to approve Office Hours", ""},
		{"NFR-SEC-2: push rights on a person's repository are a clear no", 200, githubRepo("PaperPlane", "User", "Stash", pushRights), nil, false, clearNo, 403, "no longer a maintainer of this room", "approve"},
		{"NFR-SEC-2: no rights at all are a clear no", 200, githubRepo("PaperPlane", "User", "Stash", `{"admin":false,"maintain":false,"push":false,"triage":false,"pull":true}`), nil, false, clearNo, 403, "GitHub no longer shows admin or maintain rights for you on " + theRepo, ""},

		// Anything else: the start is refused and the role stays.
		{"NFR-SEC-2: GitHub is down", 500, `{"message":"Server Error"}`, nil, false, hiccup, 502, "GitHub didn't answer", "no longer"},
		{"NFR-SEC-2: GitHub does not answer in time", 200, "", nil, true, hiccup, 502, "GitHub didn't answer", "no longer"},
		{"NFR-SEC-2: GitHub sends a page instead of JSON", 200, `<html>`, nil, false, hiccup, 502, "GitHub didn't answer", "no longer"},
		{"NFR-SEC-2: a secondary rate limit", 403, rateLimit, map[string]string{"Retry-After": "30"}, false, hiccup, 502, "GitHub didn't answer", "approve"},
		{"NFR-SEC-2: the hourly rate limit", 403, rateLimit, map[string]string{"X-RateLimit-Remaining": "0"}, false, hiccup, 502, "GitHub didn't answer", "approve"},
		{"NFR-SEC-2: the sign-in is out of date", 401, `{"message":"Bad credentials"}`, nil, false, hiccup, 401, "Sign in again", "no longer a maintainer"},
		{"NFR-SEC-2: GitHub refuses to show the repository", 403, `{"message":"OAuth App access restrictions"}`, nil, false, hiccup, 403, "approve Office Hours", "no longer a maintainer"},
		{"NFR-SEC-2: the repository is hidden or gone", 404, `{"message":"Not Found"}`, nil, false, hiccup, 409, "made private or deleted", "no longer a maintainer"},
		{"NFR-SEC-2: the name now belongs to another repository, which the caller owns", 200, withID(githubRepo("PaperPlane", "Organization", "Stash", adminRights), elsewhere), nil, false, hiccup, 409, "different repository", ""},
		{"NFR-SEC-2: the name now belongs to another repository, where the caller has no rights", 200, withID(githubRepo("PaperPlane", "Organization", "Stash", pushRights), elsewhere), nil, false, hiccup, 409, "different repository", "no longer a maintainer"},
		{"NFR-SEC-2, A-7: a reply without permissions proves nothing", 200, githubRepo("PaperPlane", "Organization", "Stash", ""), nil, false, hiccup, 502, "didn't say what rights", "no longer a maintainer"},
		{"NFR-PRIV-1: the repository is private now", 200, private, nil, false, hiccup, 403, "private now", "no longer a maintainer"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			a, starts := sessionApp(t)
			a.gh.status, a.gh.reply, a.gh.header = tt.status, tt.reply, tt.header
			if tt.hang {
				a.gh.hang = make(chan struct{})
				defer close(a.gh.hang)
			}
			res, body := call(a.mux, "POST", sessionStartPath, sessionStartBody, a.as(t, hostID))
			if len(a.gh.asked) != 1 {
				t.Fatalf("GitHub was asked %d times, want once: %q", len(a.gh.asked), a.gh.asked)
			}

			if tt.want == yes {
				if res.StatusCode != http.StatusOK || len(starts.asked) != 1 || len(starts.removed) != 0 || a.chat.types() != "OH:Queue OH:Session" {
					t.Fatalf("status %d body %s, %d starts, removed %v, the room got %q; want a started session", res.StatusCode, body, len(starts.asked), starts.removed, a.chat.types())
				}
				return
			}

			refused(t, tt.name, res, body, tt.code, tt.says)
			if tt.never != "" && strings.Contains(body, tt.never) {
				t.Errorf("the sentence must not say %q: %s", tt.never, body)
			}
			// A refused start created no session and told the room nothing.
			if len(starts.asked) != 0 || len(a.queue.asked) != 0 || a.chat.types() != "" {
				t.Errorf("a refused start reached the store (%+v) or the room (%q)", starts.asked, a.chat.types())
			}
			isMaintainer, isMember := a.projects.maintainers[sessionHostRole], a.projects.members[sessionHostRole]
			if tt.want == clearNo {
				if want := []string{sessionHostRole}; !slices.Equal(starts.removed, want) || isMaintainer || !isMember {
					t.Errorf("after a clear no: removed %v, maintainer %v, member %v; want the role gone and the membership kept", starts.removed, isMaintainer, isMember)
				}
				return
			}
			if len(starts.removed) != 0 || !isMaintainer {
				t.Errorf("a hiccup cost a maintainer the role: removed %v, maintainer %v", starts.removed, isMaintainer)
			}
		})
	}

	t.Run("NFR-SEC-2: after a clear no the caller is a member like any other", func(t *testing.T) {
		a, starts := sessionApp(t)
		a.gh.reply = githubRepo("PaperPlane", "Organization", "Stash", pushRights)
		call(a.mux, "POST", sessionStartPath, sessionStartBody, a.as(t, hostID))

		// GitHub would say yes again, but nobody asks it for someone who is no maintainer here.
		a.gh.reply = githubRepo("PaperPlane", "Organization", "Stash", adminRights)
		for _, route := range sessionRoutes {
			res, body := call(a.mux, "POST", route.path, route.body, a.as(t, hostID))
			refused(t, route.path, res, body, http.StatusForbidden, "Only a maintainer")
		}
		if len(a.gh.asked) != 1 || len(starts.asked) != 0 || len(starts.planned) != 0 || len(a.queue.asked) != 0 || a.chat.types() != "" {
			t.Errorf("GitHub was asked %d times, %d sessions started, %d times planned, %d actions applied, the room got %q; want 1 and nothing else",
				len(a.gh.asked), len(starts.asked), len(starts.planned), len(a.queue.asked), a.chat.types())
		}
	})

	t.Run("NFR-SEC-2: a role that cannot be taken away is a 500, and still no session", func(t *testing.T) {
		a, starts := sessionApp(t)
		a.gh.reply = githubRepo("PaperPlane", "Organization", "Stash", pushRights)
		starts.err = errors.New("connection refused to 10.0.0.5:5432")
		res, body := call(a.mux, "POST", sessionStartPath, sessionStartBody, a.as(t, hostID))
		refused(t, "the role stays for now", res, body, http.StatusInternalServerError, "Try again")
		if len(starts.asked) != 0 || a.chat.types() != "" || strings.Contains(body, "10.0.0.5") {
			t.Errorf("body %s, %d starts, the room got %q; want no session, silence and no leak", body, len(starts.asked), a.chat.types())
		}
	})
}

func TestSessionEnd(t *testing.T) {
	ended := func(a *queueApp, after []queue.Hand, closed ...queue.Status) {
		a.queue.applied = store.Applied{Version: 9, After: queue.State{Hands: after}}
		over := store.Session{ID: liveSession, ProjectID: queueProject, Number: 3, Title: "Thursday office hours", Version: 9, StartedAt: now().Add(-time.Hour), EndedAt: now()}
		a.queue.snapshot = store.Snapshot{Session: &over}
		for i, status := range closed {
			a.queue.snapshot.Closed = append(a.queue.snapshot.Closed, store.QueueHand{Hand: queue.Hand{ID: int64(200 + i), AuthorID: askerID, Status: status}, Author: person(askerID)})
		}
	}

	t.Run("FR-SESS-4, FR-SESS-7: waiting hands are carried over unless the maintainer says no", func(t *testing.T) {
		for _, tt := range []struct {
			name  string
			body  string
			carry bool
		}{
			{"FR-SESS-7: carry is ticked", `{"carry":true}`, true},
			{"FR-SESS-4: carry is unticked", `{"carry":false}`, false},
			{"FR-SESS-7: nothing is said about it", `{}`, true},
			{"FR-SESS-7: carry is null", `{"carry":null}`, true},
		} {
			t.Run(tt.name, func(t *testing.T) {
				a, _ := sessionApp(t)
				ended(a, nil)
				res, body := call(a.mux, "POST", sessionEndPath, tt.body, a.as(t, hostID))
				var q queueJSON
				decode(t, body, &q)
				if res.StatusCode != http.StatusOK || q.Session == nil || q.Session.Live || q.Session.EndedAt == nil {
					t.Fatalf("status %d body %s, want 200 and the queue of the ended session", res.StatusCode, body)
				}
				if got := sessionAction(t, a, queue.End); got.Carry != tt.carry {
					t.Errorf("the end was applied with carry %v, want %v", got.Carry, tt.carry)
				}
			})
		}
	})

	t.Run("FR-SESS-4, FR-SESS-7: the ended card counts what became of the hands", func(t *testing.T) {
		carried := []queue.Hand{{ID: 104, Status: queue.Carried}, {ID: 105, Status: queue.Carried}}
		notReached := []queue.Hand{{ID: 104, Status: queue.NotReached}, {ID: 105, Status: queue.NotReached}, {ID: 106, Status: queue.NotReached}}
		for _, tt := range []struct {
			name   string
			body   string
			after  []queue.Hand
			closed []queue.Status
			want   sessionCardContent
		}{
			{"FR-SESS-7: two hands carried, three answered", `{"carry":true}`, carried,
				[]queue.Status{queue.Carried, queue.Carried, queue.Answered, queue.Skipped, queue.Answered, queue.Withdrawn, queue.Answered, queue.Removed, queue.Merged},
				sessionCardContent{SessionID: "12", Title: "Thursday office hours", State: "ended", Answered: 3, Carried: 2}},
			{"FR-SESS-4: three hands not reached, one answered", `{"carry":false}`, notReached,
				[]queue.Status{queue.NotReached, queue.NotReached, queue.NotReached, queue.Answered},
				sessionCardContent{SessionID: "12", Title: "Thursday office hours", State: "ended", Answered: 1, NotReached: 3}},
			{"FR-SESS-4: a session nobody raised a hand in", `{}`, nil, nil,
				sessionCardContent{SessionID: "12", Title: "Thursday office hours", State: "ended"}},
		} {
			t.Run(tt.name, func(t *testing.T) {
				a, _ := sessionApp(t)
				ended(a, tt.after, tt.closed...)
				res, body := call(a.mux, "POST", sessionEndPath, tt.body, a.as(t, hostID))
				if res.StatusCode != http.StatusOK {
					t.Fatalf("status %d body %s, want 200", res.StatusCode, body)
				}
				if said := a.chat.types(); said != "OH:Queue OH:Session" {
					t.Fatalf("the room got %q, want the card and the ping", said)
				}
				m := a.chat.one(t, typeSession)
				// The end mentions nobody: only the start rings everyone (FR-SESS-3).
				if card := sessionCardOf(t, m); card != tt.want || m.Mention || !strings.Contains(m.Push, "Thursday office hours") {
					t.Errorf("the ended card = %+v (mention %v), want %+v", card, m.Mention, tt.want)
				}
				if id, version := sessionPingOf(t, a); id != "12" || version != 9 {
					t.Errorf("FR-QUEUE-4: the ping names session %q at version %d, want 12 at 9", id, version)
				}
			})
		}
	})

	t.Run("FR-QUEUE-2: the rules refuse an end, and the room hears nothing", func(t *testing.T) {
		for _, tt := range []struct {
			name   string
			refuse error
			says   string
		}{
			{"FR-QUEUE-2: a hand is being answered", queue.ErrAnswering, "Finish it first"},
			{"FR-SESS-4: the session is over already", queue.ErrNotLive, "no live session"},
		} {
			a, _ := sessionApp(t)
			a.queue.refuse = tt.refuse
			res, body := call(a.mux, "POST", sessionEndPath, `{"carry":true}`, a.as(t, hostID))
			refused(t, tt.name, res, body, http.StatusConflict, tt.says)
			if a.chat.types() != "" {
				t.Errorf("%s: the room got %q after a refusal", tt.name, a.chat.types())
			}
		}
	})

	t.Run("ARCHITECTURE §6: an end that is written is announced even when its card cannot be made", func(t *testing.T) {
		t.Run("the queue shows a session that started in the meantime", func(t *testing.T) {
			a, _ := sessionApp(t)
			ended(a, nil, queue.Answered)
			a.queue.snapshot.Session = &store.Session{ID: sessionStartedID, Title: "The next one", Live: true, Version: 1}
			res, body := call(a.mux, "POST", sessionEndPath, `{}`, a.as(t, hostID))
			if res.StatusCode != http.StatusOK || a.chat.types() != "OH:Queue" {
				t.Fatalf("status %d body %s, the room got %q; want 200 and the ping alone", res.StatusCode, body, a.chat.types())
			}
			if id, version := sessionPingOf(t, a); id != "12" || version != 9 {
				t.Errorf("the ping names session %q at version %d, want the ended one, 12 at 9", id, version)
			}
		})
		t.Run("the queue shows no session", func(t *testing.T) {
			a, _ := sessionApp(t)
			a.queue.applied = store.Applied{Version: 9}
			res, body := call(a.mux, "POST", sessionEndPath, `{}`, a.as(t, hostID))
			if res.StatusCode != http.StatusOK || a.chat.types() != "OH:Queue" {
				t.Fatalf("status %d body %s, the room got %q; want 200 and the ping alone", res.StatusCode, body, a.chat.types())
			}
		})
		t.Run("plan decision 6: the maintainer goes away once the end is written", func(t *testing.T) {
			a, _ := sessionApp(t)
			ended(a, nil, queue.Answered)
			ctx, leave := context.WithCancel(context.Background())
			a.deps.queue = sessionFakeLeavingQueue{a.queue, leave}
			a.remux()
			call(a.mux, "POST", sessionEndPath, `{}`, a.as(t, hostID), func(r *http.Request) { *r = *r.WithContext(ctx) })
			if said := a.chat.types(); said != "OH:Queue OH:Session" {
				t.Fatalf("the room got %q, want the ended card and the ping", said)
			}
		})
		t.Run("the queue cannot be read", func(t *testing.T) {
			a, _ := sessionApp(t)
			a.queue.applied = store.Applied{Version: 9}
			a.deps.queue = sessionFakeBlindQueue{a.queue}
			a.remux()
			res, body := call(a.mux, "POST", sessionEndPath, `{}`, a.as(t, hostID))
			// The caller learns that the answer could not be read; the room still learns of the end.
			refused(t, "blind", res, body, http.StatusInternalServerError, "Try again")
			if len(a.queue.asked) != 1 || a.chat.types() != "OH:Queue" {
				t.Fatalf("%d actions applied, the room got %q; want the end applied and the ping sent", len(a.queue.asked), a.chat.types())
			}
		})
	})

	t.Run("NFR-SEC-3: a body the route does not expect is refused before anything is applied", func(t *testing.T) {
		for name, body := range map[string]string{
			"carry as text":         `{"carry":"yes"}`,
			"carry as a number":     `{"carry":1}`,
			"a field we don't have": `{"carry":true,"notReached":true}`,
			"a list":                `[true]`,
		} {
			a, starts := sessionApp(t)
			res, text := call(a.mux, "POST", sessionEndPath, body, a.as(t, hostID))
			refused(t, name, res, text, http.StatusBadRequest, "not the JSON")
			sessionUntouched(t, name, a, starts)
		}
	})
}

func TestSessionLastCall(t *testing.T) {
	hands := func(waiting int) []queue.Hand {
		out := []queue.Hand{{ID: 101, AuthorID: askerID, Status: queue.Answering}}
		for i := range waiting {
			out = append(out, queue.Hand{ID: int64(102 + i), AuthorID: otherID, Status: queue.Waiting})
		}
		return out
	}
	for _, tt := range []struct {
		name      string
		body      string
		closed    bool // what the body asks for
		wasClosed bool
		waiting   int
		line      string // "" when the room is told nothing in words
	}{
		{"FR-SESS-6: closing hands tells the room how many are left", `{"closed":true}`, true, false, 3, "Last call: hands are closed. 3 questions are left in line."},
		{"FR-SESS-6: one is left", `{"closed":true}`, true, false, 1, "Last call: hands are closed. 1 question is left in line."},
		{"FR-SESS-6: nobody is left; the hand being answered is not in line", `{"closed":true}`, true, false, 0, "Last call: hands are closed. Nobody is left in line."},
		{"FR-SESS-6: opening hands again says so", `{"closed":false}`, false, true, 2, "Hands are open again: you can raise yours."},
		{"FR-SESS-6: closing hands that are closed says nothing twice", `{"closed":true}`, true, true, 3, ""},
		{"FR-SESS-6: opening hands that are open says nothing", `{"closed":false}`, false, false, 3, ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			a, _ := sessionApp(t)
			a.queue.applied = store.Applied{
				Version: 5,
				Before:  queue.State{Live: true, HandsClosed: tt.wasClosed, Hands: hands(tt.waiting)},
				After:   queue.State{Live: true, HandsClosed: tt.closed, Hands: hands(tt.waiting)},
			}
			a.queue.snapshot = store.Snapshot{Session: &store.Session{ID: liveSession, Live: true, HandsClosed: tt.closed, Version: 5}}
			res, body := call(a.mux, "POST", sessionLastCallPath, tt.body, a.as(t, hostID))
			var q queueJSON
			decode(t, body, &q)
			if res.StatusCode != http.StatusOK || q.Session == nil || q.Session.HandsClosed != tt.closed {
				t.Fatalf("status %d body %s, want 200 and the queue", res.StatusCode, body)
			}
			if got := sessionAction(t, a, queue.LastCall); got.Closed != tt.closed {
				t.Errorf("last call was applied with closed %v, want %v", got.Closed, tt.closed)
			}
			if id, version := sessionPingOf(t, a); id != "12" || version != 5 {
				t.Errorf("FR-QUEUE-4: the ping names session %q at version %d, want 12 at 5", id, version)
			}
			if tt.line == "" {
				if said := a.chat.types(); said != "OH:Queue" {
					t.Errorf("the room got %q, want the ping alone", said)
				}
				return
			}
			m := a.chat.one(t, typeText)
			var said struct {
				Content string `json:"content"`
			}
			if err := json.Unmarshal([]byte(m.Content), &said); err != nil || said.Content != tt.line || m.From != botID || m.ChannelID != groupID(queueProject) {
				t.Errorf("the room was told %q by %q in %q (%v), want %q by the bot in the project's room", said.Content, m.From, m.ChannelID, err, tt.line)
			}
		})
	}

	t.Run("NFR-SEC-3: the body must say whether hands are closed", func(t *testing.T) {
		for name, tt := range map[string]struct{ body, says string }{
			"nothing":               {`{}`, "closed or open"},
			"null":                  {`{"closed":null}`, "closed or open"},
			"closed as text":        {`{"closed":"yes"}`, "not the JSON"},
			"a field we don't have": {`{"closed":true,"minutes":5}`, "not the JSON"},
		} {
			a, starts := sessionApp(t)
			res, body := call(a.mux, "POST", sessionLastCallPath, tt.body, a.as(t, hostID))
			refused(t, name, res, body, http.StatusBadRequest, tt.says)
			sessionUntouched(t, name, a, starts)
		}
	})

	t.Run("FR-SESS-6: last call after the session ended is refused, and the room hears nothing", func(t *testing.T) {
		a, _ := sessionApp(t)
		a.queue.refuse = queue.ErrNotLive
		res, body := call(a.mux, "POST", sessionLastCallPath, `{"closed":true}`, a.as(t, hostID))
		refused(t, "not live", res, body, http.StatusConflict, "no live session")
		if a.chat.types() != "" {
			t.Errorf("the room got %q after a refusal", a.chat.types())
		}
	})
}

func TestSessionNextTime(t *testing.T) {
	inTwoDays := time.Now().UTC().Add(48 * time.Hour).Truncate(time.Second)
	text := func(at time.Time) string { return at.Format(time.RFC3339Nano) }
	body := func(at string) string { return `{"at":"` + at + `"}` }

	t.Run("FR-SESS-5: a maintainer plans the next session, in any zone, and gets the queue with it", func(t *testing.T) {
		for _, tt := range []struct {
			name string
			at   string
			want time.Time
		}{
			{"FR-SESS-5: as a page writes it, in UTC with milliseconds", inTwoDays.Add(250 * time.Millisecond).Format("2006-01-02T15:04:05.000Z"), inTwoDays.Add(250 * time.Millisecond)},
			{"FR-SESS-5: in the maintainer's own zone", text(inTwoDays.In(time.FixedZone("IST", 5*3600+1800))), inTwoDays},
			{"FR-SESS-5: finer than Postgres keeps", text(inTwoDays.Add(123456789 * time.Nanosecond)), inTwoDays.Add(123456 * time.Microsecond)},
			{"FR-SESS-5: eleven months away", text(inTwoDays.AddDate(0, 11, 0)), inTwoDays.AddDate(0, 11, 0)},
		} {
			t.Run(tt.name, func(t *testing.T) {
				a, starts := sessionApp(t)
				res, reply := call(a.mux, "POST", sessionNextPath, body(tt.at), a.as(t, hostID))
				var q queueJSON
				decode(t, reply, &q)
				if res.StatusCode != http.StatusOK || q.NextSessionAt == nil || !q.NextSessionAt.Equal(tt.want) {
					t.Fatalf("status %d body %s, want 200 and the queue with the next session at %v", res.StatusCode, reply, tt.want)
				}
				if len(starts.planned) != 1 || !starts.planned[0].Equal(tt.want) || starts.planned[0].Nanosecond()%1000 != 0 {
					t.Errorf("the store was given %v, want %v in whole microseconds", starts.planned, tt.want)
				}
				// A plan changes no queue: there is nothing to ping, and nothing is said.
				if len(a.queue.asked) != 0 || a.chat.types() != "" {
					t.Errorf("planning applied %+v and told the room %q, want neither", a.queue.asked, a.chat.types())
				}
			})
		}
	})

	t.Run("FR-SESS-5: an empty time clears the plan", func(t *testing.T) {
		a, starts := sessionApp(t)
		a.queue.snapshot.NextSessionAt = inTwoDays
		res, reply := call(a.mux, "POST", sessionNextPath, body(""), a.as(t, hostID))
		var q queueJSON
		decode(t, reply, &q)
		if res.StatusCode != http.StatusOK || q.NextSessionAt != nil {
			t.Fatalf("status %d body %s, want 200 and no next session", res.StatusCode, reply)
		}
		if len(starts.planned) != 1 || !starts.planned[0].IsZero() {
			t.Errorf("the store was given %v, want the zero time once", starts.planned)
		}
	})

	t.Run("FR-SESS-5: a time that is past, too far away or no time at all is refused", func(t *testing.T) {
		for _, tt := range []struct {
			name string
			body string
			says string
		}{
			{"FR-SESS-5: a minute ago", body(text(time.Now().UTC().Add(-time.Minute))), "in the future"},
			{"FR-SESS-5: last year", body(text(inTwoDays.AddDate(-1, 0, 0))), "in the future"},
			{"FR-SESS-5: the year 1, which is Go's zero time", body("0001-01-01T00:00:00Z"), "in the future"},
			{"FR-SESS-5: a year and a day away", body(text(inTwoDays.AddDate(1, 0, 1))), "a year away"},
			{"FR-SESS-5: the year 9999", body("9999-12-31T23:59:59Z"), "a year away"},
			{"NFR-SEC-3: a word", body("tomorrow"), "date and time"},
			{"NFR-SEC-3: a day without a time", body("2027-01-14"), "date and time"},
			{"NFR-SEC-3: a time without a zone", body("2027-01-14T17:00:00"), "date and time"},
			{"NFR-SEC-3: a time with a space at the end", body(text(inTwoDays) + " "), "date and time"},
			{"NFR-SEC-3: nothing is said about the time", `{}`, "when the next session is"},
			{"NFR-SEC-3: the time is null", `{"at":null}`, "when the next session is"},
			{"NFR-SEC-3: the time is a number", `{"at":1794000000}`, "not the JSON"},
			{"NFR-SEC-3: a field we don't have", `{"at":"","title":"Next week"}`, "not the JSON"},
		} {
			t.Run(tt.name, func(t *testing.T) {
				a, starts := sessionApp(t)
				res, reply := call(a.mux, "POST", sessionNextPath, tt.body, a.as(t, hostID))
				refused(t, tt.name, res, reply, http.StatusBadRequest, tt.says)
				sessionUntouched(t, tt.name, a, starts)
			})
		}
	})

	t.Run("a database that fails is a 500 without its text; a server without the stores says it is not set up", func(t *testing.T) {
		a, starts := sessionApp(t)
		starts.err = errors.New("connection refused to 10.0.0.5:5432")
		res, reply := call(a.mux, "POST", sessionNextPath, body(text(inTwoDays)), a.as(t, hostID))
		refused(t, "database down", res, reply, http.StatusInternalServerError, "Try again")
		if strings.Contains(reply, "10.0.0.5") {
			t.Errorf("NFR-OPS-2: the database error reached the user: %s", reply)
		}

		for name, strip := range map[string]func(*deps){
			"no session store": func(d *deps) { d.starts = nil },
			"no queue store":   func(d *deps) { d.queue = nil },
		} {
			a, starts := sessionApp(t)
			strip(&a.deps)
			a.remux()
			res, reply := call(a.mux, "POST", sessionNextPath, body(text(inTwoDays)), a.as(t, hostID))
			refused(t, name, res, reply, http.StatusServiceUnavailable, "set up")
			sessionUntouched(t, name, a, starts)
		}
	})
}

func TestSessionRoutesAreForMaintainers(t *testing.T) {
	otherSite := func(r *http.Request) { r.Header.Set("Origin", "https://evil.example") }
	for _, c := range []struct {
		name   string
		who    string // "" is signed out
		mutate func(*http.Request)
		status int
		says   string
	}{
		{"AT-10, NFR-SEC-2: a member", askerID, nil, http.StatusForbidden, "Only a maintainer"},
		{"NFR-SEC-2: someone outside the room", strangerID, nil, http.StatusForbidden, "Only a maintainer"},
		{"NFR-SEC-2: nobody", "", nil, http.StatusUnauthorized, "Sign in first"},
		{"NFR-SEC-5: a maintainer's browser, sent by another site", hostID, otherSite, http.StatusForbidden, "did not come from Office Hours"},
	} {
		t.Run(c.name, func(t *testing.T) {
			for _, route := range sessionRoutes {
				a, starts := sessionApp(t)
				var cookie *http.Cookie
				if c.who != "" {
					cookie = a.as(t, c.who)
				}
				var mutate []func(*http.Request)
				if c.mutate != nil {
					mutate = append(mutate, c.mutate)
				}
				res, body := call(a.mux, "POST", route.path, route.body, cookie, mutate...)
				refused(t, route.path, res, body, c.status, c.says)
				// Nothing is applied, nothing is asked of GitHub, nothing is said.
				if len(a.gh.asked) != 0 {
					t.Errorf("%s: GitHub was asked %q", route.path, a.gh.asked)
				}
				sessionUntouched(t, route.path, a, starts)
			}
		})
	}
}
