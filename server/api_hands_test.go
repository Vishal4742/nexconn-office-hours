package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"officehours/queue"
	"officehours/store"
)

// What the hands tests add to the project, the session and the people of newQueueApp.
const (
	handsRaisePath    = "/api/sessions/12/hands" // liveSession
	handsWithdrawPath = "/api/hands/101/withdraw"
	handsPlusPath     = "/api/hands/101/plus"
	handsHerePath     = "/api/hands/101/here"

	handsTheirs     = int64(101) // the other member's waiting hand, in liveSession
	handsNew        = int64(103) // the hand a raise adds: the asker's
	handsOlder      = int64(88)  // a hand of handsOldSession
	handsOldSession = int64(11)  // an earlier session of the same project
	handsFar        = int64(301) // a hand in another project's room, which nobody here has joined
	handsFarSession = int64(30)
	handsVersion    = 7 // the queue's version after any action the store of handsApp applies

	handsQuestion = "How do I retry only on 5xx?"
	handsDetails  = "It retries on 404 too.\n\nI use v2."
)

// handsApp is newQueueApp with hands to act on (one in the live session, one in an earlier session
// of the project, one in another project's room) and a store that answers every action the way
// the real one answers a raise: the queue is at handsVersion, the asker's new hand is in
// Applied.After with its id, and the queue holds it behind the other member's hand, which has the
// asker's +1.
func handsApp(t *testing.T) *queueApp {
	t.Helper()
	at := time.Date(2026, 11, 14, 11, 30, 0, 0, time.UTC)
	theirs := queue.Hand{ID: handsTheirs, AuthorID: otherID, Title: "Can the cache be shared?", Status: queue.Waiting, RaisedAt: at, SortAt: at, Plus: []string{askerID}}
	fresh := queue.Hand{ID: handsNew, AuthorID: askerID, Title: handsQuestion, Details: handsDetails, Status: queue.Waiting, RaisedAt: at.Add(time.Minute), SortAt: at.Add(time.Minute)}

	a := newQueueApp(t)
	a.queue.sessions[handsOldSession] = queueProject
	a.queue.sessions[handsFarSession] = "prj2"
	a.queue.hands[handsTheirs] = liveSession
	a.queue.hands[handsOlder] = handsOldSession
	a.queue.hands[handsFar] = handsFarSession
	a.queue.applied = store.Applied{
		Version: handsVersion,
		Before:  queue.State{Live: true, Hands: []queue.Hand{theirs}},
		After:   queue.State{Live: true, Hands: []queue.Hand{theirs, fresh}},
	}
	a.queue.snapshot = store.Snapshot{
		Session: &store.Session{ID: liveSession, Title: "Thursday office hours", Live: true, Version: handsVersion},
		Open:    []store.QueueHand{{Hand: theirs, Author: person(otherID)}, {Hand: fresh, Author: person(askerID)}},
	}
	return a
}

// handsBody is the JSON the raise-hand sheet sends.
func handsBody(t *testing.T, title, details string) string {
	t.Helper()
	body, err := json.Marshal(map[string]string{"title": title, "details": details})
	if err != nil {
		t.Fatalf("the question as JSON: %v", err)
	}
	return string(body)
}

// handsCall sends one of the four routes what a page sends it: a question to the raise route, {}
// to the others. An empty userID is someone who is signed out.
func handsCall(t *testing.T, a *queueApp, path, userID string, mutate ...func(*http.Request)) (*http.Response, string) {
	t.Helper()
	body := "{}"
	if strings.HasSuffix(path, "/hands") {
		body = handsBody(t, handsQuestion, handsDetails)
	}
	var cookie *http.Cookie
	if userID != "" {
		cookie = a.as(t, userID)
	}
	return call(a.mux, "POST", path, body, cookie, mutate...)
}

// handsAsked returns the one action the store was asked to apply. Its time must be now, in whole
// microseconds; it is then set to zero, so that the rest can be compared as one value.
func handsAsked(t *testing.T, a *queueApp) queue.Action {
	t.Helper()
	if len(a.queue.asked) != 1 {
		t.Fatalf("the store was asked to apply %d actions, want 1: %+v", len(a.queue.asked), a.queue.asked)
	}
	got := a.queue.asked[0]
	if age := time.Since(got.At); got.At.IsZero() || got.At.Nanosecond()%1000 != 0 || age < 0 || age > time.Minute {
		t.Errorf("the action's time is %v, want now in whole microseconds", got.At)
	}
	got.At = time.Time{}
	return got
}

// handsQueue fails the test unless the reply is the queue of handsApp as viewer sees it: at
// handsVersion, the other member's hand and then the asker's, each marked for the one it belongs
// to, and the asker's +1 marked for the asker only.
func handsQueue(t *testing.T, res *http.Response, body, viewer string) {
	t.Helper()
	var q queueJSON
	decode(t, body, &q)
	if res.StatusCode != http.StatusOK || q.Session == nil || q.Session.ID != idText(liveSession) || q.Session.Version != handsVersion || len(q.Hands) != 2 {
		t.Fatalf("status %d body %s, want 200 and the queue of session %d at version %d with its two hands", res.StatusCode, body, liveSession, handsVersion)
	}
	theirs, fresh := q.Hands[0], q.Hands[1]
	if theirs.ID != idText(handsTheirs) || fresh.ID != idText(handsNew) || fresh.Title != handsQuestion || fresh.Details != handsDetails {
		t.Errorf("the answer's hands are %+v, want hand %d and then hand %d with its words", q.Hands, handsTheirs, handsNew)
	}
	if theirs.IsMine != (viewer == otherID) || theirs.PlusByMe != (viewer == askerID) || fresh.IsMine != (viewer == askerID) || fresh.PlusByMe {
		t.Errorf("the answer's hands are %+v, want them as %s sees them", q.Hands, viewer)
	}
}

// handsPing reads the one ping the room got: which session it names and which version.
func handsPing(t *testing.T, a *queueApp) (sessionID string, version int) {
	t.Helper()
	ping := a.chat.one(t, typeQueue)
	var sent struct {
		SessionID string `json:"sessionId"`
		Version   int    `json:"version"`
	}
	if err := json.Unmarshal([]byte(ping.Content), &sent); err != nil {
		t.Fatalf("the ping is not JSON: %v in %s", err, ping.Content)
	}
	if ping.From != botID || ping.ChannelID != groupID(queueProject) || !ping.Transient {
		t.Errorf("FR-QUEUE-7: the ping was sent by %q into %q (stored: %v), want the bot into the project's room, not stored", ping.From, ping.ChannelID, !ping.Transient)
	}
	return sent.SessionID, sent.Version
}

// handsUntouched fails the test if anything reached the rules or the room.
func handsUntouched(t *testing.T, a *queueApp, who string) {
	t.Helper()
	if len(a.queue.asked) != 0 || a.chat.types() != "" {
		t.Errorf("%s: the store was asked %+v and the room got %q, want neither", who, a.queue.asked, a.chat.types())
	}
}

func TestHandsRaise(t *testing.T) {
	t.Run("FR-HAND-1, FR-HAND-5: a member's question is applied as theirs and as cleaned, the room gets its card, the answer is the queue", func(t *testing.T) {
		a := handsApp(t)
		typed := handsBody(t, "  How do I\tretry \r\n only on 5xx?  ", "\r\n It retries on 404 too.\r\n\r\nI use v2.\n  ")
		res, body := call(a.mux, "POST", handsRaisePath, typed, a.as(t, askerID))

		handsQueue(t, res, body, askerID)
		// The actor is who the session cookie says, the hand has no id yet, and the time of the
		// caller's last hand is the store's to fill in.
		if got, want := handsAsked(t, a), (queue.Action{Kind: queue.Raise, ActorID: askerID, Title: handsQuestion, Details: handsDetails}); got != want {
			t.Errorf("applied %+v, want %+v", got, want)
		}

		if a.chat.types() != "OH:Hand OH:Queue" {
			t.Fatalf("the room got %q, want the hand's card and the ping", a.chat.types())
		}
		type cardContent struct {
			HandID      string `json:"handId"`
			SessionID   string `json:"sessionId"`
			Title       string `json:"title"`
			AuthorLogin string `json:"authorLogin"`
		}
		card := a.chat.one(t, typeHand)
		var shown cardContent
		if err := json.Unmarshal([]byte(card.Content), &shown); err != nil {
			t.Fatalf("the card is not JSON: %v in %s", err, card.Content)
		}
		if want := (cardContent{"103", "12", handsQuestion, "login-gh2"}); shown != want {
			t.Errorf("the card says %+v, want %+v", shown, want)
		}
		if card.From != botID || card.ChannelID != groupID(queueProject) || card.Transient || !strings.Contains(card.Push, "@login-gh2") {
			t.Errorf("FR-QUEUE-7: the card is %+v, want it stored, from the bot, in the project's room, with the author in its push line", card)
		}
		if session, version := handsPing(t, a); session != "12" || version != handsVersion {
			t.Errorf("the ping names session %q at version %d, want \"12\" at %d", session, version, handsVersion)
		}
	})

	t.Run("FR-HAND-1: the details are optional: a title alone is a hand", func(t *testing.T) {
		a := handsApp(t)
		res, body := call(a.mux, "POST", handsRaisePath, `{"title":"How do I retry only on 5xx?"}`, a.as(t, askerID))
		handsQueue(t, res, body, askerID)
		if got, want := handsAsked(t, a), (queue.Action{Kind: queue.Raise, ActorID: askerID, Title: handsQuestion}); got != want {
			t.Errorf("applied %+v, want %+v", got, want)
		}
	})

	t.Run("FR-HAND-5: a store that names no new hand: the hand is raised and pinged, and the log says why no card went out", func(t *testing.T) {
		var logged bytes.Buffer
		log.SetOutput(&logged)
		defer log.SetOutput(os.Stderr)

		a := handsApp(t)
		a.queue.applied.After = a.queue.applied.Before
		res, body := handsCall(t, a, handsRaisePath, askerID)
		handsQueue(t, res, body, askerID)
		if a.chat.types() != typeQueue {
			t.Errorf("the room got %q, want the ping alone: there is no hand to put on a card", a.chat.types())
		}
		if !strings.Contains(logged.String(), "no new hand") || !strings.Contains(logged.String(), "session 12") {
			t.Errorf("the log does not say that session 12 got no card: %q", logged.String())
		}
	})

	t.Run("ARCHITECTURE §6: a room that cannot be told still has the hand, and its author the queue", func(t *testing.T) {
		a := handsApp(t)
		a.chat.err = errors.New("nexconn is down")
		res, body := handsCall(t, a, handsRaisePath, askerID)
		handsQueue(t, res, body, askerID)
		handsAsked(t, a)
	})
}

func TestHandsRaiseValidation(t *testing.T) {
	const (
		short = "A question needs at least 10 characters."
		long  = "A question can have at most 120 characters. Put the rest in the details."
		wordy = "The details can have at most 2,000 characters."
		nasty = `Why does "retry" fail? </script><b>x</b> &amp; 'q'`
	)
	q := func(n int) string { return strings.Repeat("q", n) }
	smile := func(n int) string { return strings.Repeat("🙂", n) } // one character, four bytes
	show := func(s string) string {
		return fmt.Sprintf("%q (%d characters)", clip(s, 40), utf8.RuneCountInString(s))
	}

	for _, c := range []struct {
		name           string
		title, details string // as typed
		says           string // the refusal, or "" when the hand is raised
		asTitle        string // what the rules get when it is raised
		asDetails      string
	}{
		{"FR-HAND-1: a title of 9 characters", q(9), "", short, "", ""},
		{"FR-HAND-1: a title of 10 characters", q(10), "", "", q(10), ""},
		{"FR-HAND-1: a title of 120 characters", q(120), "", "", q(120), ""},
		{"FR-HAND-1: a title of 121 characters", q(121), "", long, "", ""},
		{"NFR-SEC-3: 9 emoji are 9 characters, whatever their bytes", smile(9), "", short, "", ""},
		{"NFR-SEC-3: 10 emoji are 10 characters", smile(10), "", "", smile(10), ""},
		{"NFR-SEC-3: 121 emoji are 121 characters", smile(121), "", long, "", ""},
		{"NFR-SEC-3: the longest hand there can be, in emoji, fits in one request", smile(120), smile(2000), "", smile(120), smile(2000)},
		{"FR-HAND-1: no title", "", "", short, "", ""},
		{"FR-HAND-1: a title of nothing but spaces, tabs and line breaks", " \t\r\n  　 ", "", short, "", ""},
		{"FR-HAND-1: 9 characters with spaces around and between", "  retry \t  5xx \n", "", short, "", ""},
		{"FR-HAND-1: 10 characters once the spaces are cleaned", "  retry \t  5xxx \n", "", "", "retry 5xxx", ""},
		{"FR-HAND-1: 123 characters as typed, 120 as one line", q(60) + " \r\n " + q(59), "", "", q(60) + " " + q(59), ""},
		{"FR-HAND-1: a title of two lines becomes one line", "Why does retry fail\non a 503?", "", "", "Why does retry fail on a 503?", ""},
		{"NFR-SEC-6: markup stays the text it is", nasty, nasty, "", nasty, nasty},
		{"FR-HAND-1: details of 2,000 characters", q(10), q(2000), "", q(10), q(2000)},
		{"FR-HAND-1: details of 2,001 characters", q(10), q(2001), wordy, "", ""},
		{"NFR-SEC-3: 2,001 emoji of details are 2,001 characters", q(10), smile(2001), wordy, "", ""},
		{"FR-HAND-1: details of nothing but spaces are no details", q(10), " \r\n\t ", "", q(10), ""},
		{"FR-HAND-1: details over 2,000 as typed and 2,000 once trimmed", q(10), "\n\n" + q(2000) + "  \r\n", "", q(10), q(2000)},
		{"FR-HAND-1: a Windows line break counts once", q(10), strings.Repeat("a\r\n", 999) + "ab", "", q(10), strings.Repeat("a\n", 999) + "ab"},
		{"FR-HAND-1: the details keep their lines and the spaces inside them", q(10), "  step 1:\r\n\r\n    indented   code  \n", "", q(10), "step 1:\n\n    indented   code"},
		{"FR-HAND-1: a short title with long details: the title is named first", q(9), q(2001), short, "", ""},
	} {
		t.Run(c.name, func(t *testing.T) {
			a := handsApp(t)
			res, body := call(a.mux, "POST", handsRaisePath, handsBody(t, c.title, c.details), a.as(t, askerID))
			if c.says != "" {
				refused(t, c.name, res, body, http.StatusBadRequest, c.says)
				handsUntouched(t, a, c.name)
				return
			}
			handsQueue(t, res, body, askerID)
			if got := handsAsked(t, a); got.Title != c.asTitle || got.Details != c.asDetails {
				t.Errorf("the rules got the title %s and the details %s, want %s and %s", show(got.Title), show(got.Details), show(c.asTitle), show(c.asDetails))
			}
			if a.chat.types() != "OH:Hand OH:Queue" {
				t.Errorf("the room got %q, want the hand's card and the ping", a.chat.types())
			}
		})
	}

	t.Run("NFR-SEC-3: a body that is not a question is refused before the rules see it", func(t *testing.T) {
		for name, c := range map[string]struct {
			body   string
			status int
			says   string
		}{
			"a field we don't have":        {`{"title":"How do I retry only on 5xx?","details":"","status":"answering"}`, http.StatusBadRequest, "not the JSON"},
			"someone else as the author":   {`{"title":"How do I retry only on 5xx?","authorId":"gh3"}`, http.StatusBadRequest, "not the JSON"},
			"a number as the title":        {`{"title":1234567890,"details":""}`, http.StatusBadRequest, "not the JSON"},
			"a list":                       {`["How do I retry only on 5xx?"]`, http.StatusBadRequest, "not the JSON"},
			"nothing":                      {``, http.StatusBadRequest, "not the JSON"},
			"two questions in one request": {`{"title":"How do I retry only on 5xx?"}{"title":"And on a 4xx, what then?"}`, http.StatusBadRequest, "not the JSON"},
			"more than the guard reads":    {handsBody(t, handsQuestion, strings.Repeat("d", 20000)), http.StatusRequestEntityTooLarge, "too large"},
		} {
			a := handsApp(t)
			res, body := call(a.mux, "POST", handsRaisePath, c.body, a.as(t, askerID))
			refused(t, name, res, body, c.status, c.says)
			handsUntouched(t, a, name)
		}
	})
}

func TestHandsWithdrawPlusHere(t *testing.T) {
	for _, c := range []struct {
		name, path, as string
		kind           queue.Kind
		hand           int64
		session        string // the session the action must go to
	}{
		{"FR-HAND-4: the author takes a waiting hand back", handsWithdrawPath, otherID, queue.Withdraw, handsTheirs, "12"},
		{"FR-HAND-7: a member adds their +1, or takes it back", handsPlusPath, askerID, queue.Plus, handsTheirs, "12"},
		{"FR-QUEUE-9: the author says \"I'm here\"", handsHerePath, otherID, queue.Here, handsTheirs, "12"},
		{"FR-QUEUE-1: the action goes to the session the hand is in", "/api/hands/88/plus", askerID, queue.Plus, handsOlder, "11"},
	} {
		t.Run(c.name, func(t *testing.T) {
			a := handsApp(t)
			res, body := handsCall(t, a, c.path, c.as)

			handsQueue(t, res, body, c.as)
			// Whether it is theirs to do is the rules' question: the route says who, what and which hand.
			if got, want := handsAsked(t, a), (queue.Action{Kind: c.kind, ActorID: c.as, HandID: c.hand}); got != want {
				t.Errorf("applied %+v, want %+v", got, want)
			}
			if a.chat.types() != typeQueue {
				t.Errorf("the room got %q, want the ping alone", a.chat.types())
			}
			if session, version := handsPing(t, a); session != c.session || version != handsVersion {
				t.Errorf("the ping names session %q at version %d, want %q at %d", session, version, c.session, handsVersion)
			}
		})
	}

	t.Run("NFR-SEC-2: none of them takes more than {}, so nobody acts in another member's name", func(t *testing.T) {
		a := handsApp(t)
		for _, path := range []string{handsWithdrawPath, handsPlusPath, handsHerePath} {
			res, body := call(a.mux, "POST", path, `{"userId":"gh3"}`, a.as(t, askerID))
			refused(t, path, res, body, http.StatusBadRequest, "not the JSON")
		}
		handsUntouched(t, a, "a body with a user id")
	})
}

func TestHandsRefusals(t *testing.T) {
	for _, c := range []struct {
		name, path, as string
		err            error // what the rules, or the store, answer
		status         int
		says           string
	}{
		{"FR-SESS-6, AT-18: raising a hand during last call", handsRaisePath, askerID, queue.ErrHandsClosed, http.StatusConflict, "Hands are closed for this session."},
		{"FR-HAND-2, AT-6: a second open hand", handsRaisePath, otherID, queue.ErrAlreadyOpen, http.StatusConflict, "You already have an open hand in this session."},
		{"FR-HAND-6, AT-6: a hand within 60 s of the last one, with the seconds left", handsRaisePath, askerID, &queue.TooSoonError{Wait: 39400 * time.Millisecond}, http.StatusTooManyRequests, "You raised a hand a moment ago. Wait 40 s before raising another."},
		{"FR-HAND-1: raising a hand once the session has ended", handsRaisePath, askerID, queue.ErrNotLive, http.StatusConflict, "There is no live session right now."},
		{"FR-HAND-4: withdrawing someone else's hand", handsWithdrawPath, askerID, queue.ErrNotYours, http.StatusForbidden, "Only the person who raised this hand can do that."},
		{"FR-HAND-4, FR-QUEUE-2: withdrawing a hand that is being answered", handsWithdrawPath, otherID, queue.ErrBadStatus, http.StatusConflict, "That can't be done with this hand any more."},
		{"FR-HAND-4: withdrawing a hand that has left the queue", handsWithdrawPath, otherID, queue.ErrNoSuchHand, http.StatusConflict, "That hand is no longer in the queue."},
		{"FR-HAND-4: withdrawing once the session has ended", handsWithdrawPath, otherID, queue.ErrNotLive, http.StatusConflict, "There is no live session right now."},
		{"FR-HAND-7, AT-19: a +1 on one's own hand", handsPlusPath, otherID, queue.ErrOwnHand, http.StatusConflict, "You can't +1 your own hand."},
		{"FR-HAND-7: a +1 on a hand that is being answered", handsPlusPath, askerID, queue.ErrBadStatus, http.StatusConflict, "That can't be done with this hand any more."},
		{"FR-HAND-7: a +1 once the session has ended", handsPlusPath, askerID, queue.ErrNotLive, http.StatusConflict, "There is no live session right now."},
		{"FR-QUEUE-9: \"I'm here\" on someone else's hand", handsHerePath, askerID, queue.ErrNotYours, http.StatusForbidden, "Only the person who raised this hand can do that."},
		{"FR-QUEUE-9: \"I'm here\" before the hand's turn", handsHerePath, otherID, queue.ErrNotAnswering, http.StatusConflict, "That hand is not being answered right now."},
		{"FR-QUEUE-9: \"I'm here\" once the session has ended", handsHerePath, otherID, queue.ErrNotLive, http.StatusConflict, "There is no live session right now."},
		{"NFR-OPS-2: a store that fails while raising", handsRaisePath, askerID, errors.New("connection reset by 10.0.0.5:5432"), http.StatusInternalServerError, "Something went wrong on our side."},
		{"NFR-OPS-2: a store that fails while withdrawing", handsWithdrawPath, otherID, errors.New("connection reset by 10.0.0.5:5432"), http.StatusInternalServerError, "Something went wrong on our side."},
	} {
		t.Run(c.name, func(t *testing.T) {
			a := handsApp(t)
			a.queue.refuse = c.err
			res, body := handsCall(t, a, c.path, c.as)
			refused(t, c.name, res, body, c.status, c.says)
			if strings.Contains(body, "10.0.0.5") {
				t.Errorf("NFR-OPS-2: the store's error reached the caller: %s", body)
			}
			// The rules were asked once, said no, and the room heard nothing.
			if got := handsAsked(t, a); got.ActorID != c.as {
				t.Errorf("the rules were asked on behalf of %q, want %q", got.ActorID, c.as)
			}
			if a.chat.types() != "" {
				t.Errorf("the room got %q after a refusal, want nothing", a.chat.types())
			}
		})
	}
}

func TestHandsOutsiders(t *testing.T) {
	asIs := func(*http.Request) {}
	otherSite := func(r *http.Request) { r.Header.Set("Origin", "https://evil.example") }
	form := func(r *http.Request) { r.Header.Set("Content-Type", "application/x-www-form-urlencoded") }
	a := handsApp(t)

	for _, path := range []string{handsRaisePath, handsWithdrawPath, handsPlusPath, handsHerePath} {
		for _, c := range []struct {
			name, as string
			mutate   func(*http.Request)
			status   int
			says     string
		}{
			{"NFR-SEC-2: signed out", "", asIs, http.StatusUnauthorized, "Sign in first."},
			{"NFR-SEC-2: someone outside the room", strangerID, asIs, http.StatusForbidden, "Join this room first."},
			{"NFR-SEC-5: a request from another site", askerID, otherSite, http.StatusForbidden, "did not come from Office Hours"},
			{"NFR-SEC-5: a form post", askerID, form, http.StatusUnsupportedMediaType, "JSON"},
		} {
			res, body := handsCall(t, a, path, c.as, c.mutate)
			refused(t, c.name+" on "+path, res, body, c.status, c.says)
		}
	}

	for _, c := range []struct {
		name, path string
		status     int
		says       string
	}{
		{"a session that does not exist", "/api/sessions/999/hands", http.StatusNotFound, "There is no such session."},
		{"a session id that is no number", "/api/sessions/twelve/hands", http.StatusNotFound, "There is no such session."},
		{"a hand nobody raised", "/api/hands/999/withdraw", http.StatusNotFound, "There is no such hand."},
		{"a hand id of zero", "/api/hands/0/plus", http.StatusNotFound, "There is no such hand."},
		{"a hand id below zero", "/api/hands/-101/here", http.StatusNotFound, "There is no such hand."},
		{"NFR-SEC-2: raising a hand in another room's session", "/api/sessions/30/hands", http.StatusForbidden, "Join this room first."},
		{"NFR-SEC-2: withdrawing a hand of another room", "/api/hands/301/withdraw", http.StatusForbidden, "Join this room first."},
		{"NFR-SEC-2: a +1 on a hand of another room", "/api/hands/301/plus", http.StatusForbidden, "Join this room first."},
		{"NFR-SEC-2: \"I'm here\" on a hand of another room", "/api/hands/301/here", http.StatusForbidden, "Join this room first."},
	} {
		res, body := handsCall(t, a, c.path, askerID)
		refused(t, c.name, res, body, c.status, c.says)
	}

	handsUntouched(t, a, "the callers above")
}
