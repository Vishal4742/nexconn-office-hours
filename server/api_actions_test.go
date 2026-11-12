package main

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"

	"officehours/nexconn"
	"officehours/queue"
	"officehours/store"
)

// The session every test of the maintainer's actions starts in (actionApp): askerID's hand 101 is
// being answered and otherID's hand 102 waits behind it.
const (
	actionNowID     = int64(101)
	actionWaitingID = int64(102)
	actionTitle     = "How do I retry only on 5xx?"
	actionDetails   = "RetryOn takes one status.\nIs there a way to give it a class?"
	actionVersion   = 7 // the queue's version after every action the fake store allows
)

// actionApp is newQueueApp in the middle of a session. The host has +1'd the waiting hand, so an
// answer shows whose eyes the queue was described for.
func actionApp(t *testing.T) *queueApp {
	t.Helper()
	a := newQueueApp(t)
	a.queue.hands[actionNowID], a.queue.hands[actionWaitingID] = liveSession, liveSession
	a.queue.applied = store.Applied{Version: actionVersion}
	a.queue.snapshot = store.Snapshot{
		Session: &store.Session{ID: liveSession, ProjectID: queueProject, Number: 3, Title: "Thursday office hours", Live: true, Version: actionVersion, Host: person(hostID)},
		Open: []store.QueueHand{
			{Hand: queue.Hand{ID: actionNowID, AuthorID: askerID, Title: actionTitle, Details: actionDetails, Status: queue.Answering, TakenBy: hostID}, Author: person(askerID), Taker: person(hostID)},
			{Hand: queue.Hand{ID: actionWaitingID, AuthorID: otherID, Title: "Can the cache be shared?", Status: queue.Waiting, Plus: []string{hostID}}, Author: person(otherID)},
		},
	}
	return a
}

// actionQueued fails the test unless the reply is the queue as the host sees it.
func actionQueued(t *testing.T, res *http.Response, body string) {
	t.Helper()
	var q queueJSON
	decode(t, body, &q)
	if res.StatusCode != http.StatusOK || q.Session == nil || q.Session.Version != actionVersion || len(q.Hands) != 2 || q.Hands[0].IsMine || !q.Hands[1].PlusByMe {
		t.Fatalf("status %d body %s, want the queue at version %d as the host sees it", res.StatusCode, body, actionVersion)
	}
}

// actionAsked fails the test unless the store was asked for exactly one action: of this kind, by
// the host, on this hand, at a moment since the given one in whole microseconds.
func actionAsked(t *testing.T, a *queueApp, kind queue.Kind, handID int64, since time.Time) {
	t.Helper()
	if len(a.queue.asked) != 1 {
		t.Fatalf("the store was asked for %d actions (%+v), want 1", len(a.queue.asked), a.queue.asked)
	}
	got := a.queue.asked[0]
	at := got.At
	got.At = time.Time{}
	if want := (queue.Action{Kind: kind, ActorID: hostID, HandID: handID}); got != want {
		t.Errorf("the store was asked for %+v, want %+v", got, want)
	}
	if at.Before(since) || at.After(now()) || at.Nanosecond()%1000 != 0 {
		t.Errorf("the action happened at %v, want a moment since %v in whole microseconds", at, since)
	}
}

// actionPinged fails the test unless the room got one ping, for the live session at the version
// the store answered.
func actionPinged(t *testing.T, a *queueApp) {
	t.Helper()
	m := a.chat.one(t, typeQueue)
	var ping struct {
		SessionID string `json:"sessionId"`
		Version   int    `json:"version"`
	}
	if err := json.Unmarshal([]byte(m.Content), &ping); err != nil || ping.SessionID != idText(liveSession) || ping.Version != actionVersion || !m.Transient {
		t.Errorf("FR-QUEUE-4: the ping is %+v (%v), want session %d at version %d, not stored", m, err, liveSession, actionVersion)
	}
}

// actionUntouched fails the test if a refused request reached the store, the room or the mutes.
func actionUntouched(t *testing.T, name string, a *queueApp) {
	t.Helper()
	if len(a.queue.asked) != 0 || len(a.queue.drafts) != 0 || a.chat.types() != "" || len(a.chat.muted) != 0 {
		t.Errorf("%s: the store was asked for %+v with the drafts %+v, the room got %q and the chat muted %v; want nothing at all",
			name, a.queue.asked, a.queue.drafts, a.chat.types(), a.chat.muted)
	}
}

// actionSaid reads the words of a line the bot said.
func actionSaid(t *testing.T, m nexconn.Message) string {
	t.Helper()
	var said struct {
		Content string `json:"content"`
	}
	if err := json.Unmarshal([]byte(m.Content), &said); err != nil {
		t.Fatalf("the line is not JSON: %v in %s", err, m.Content)
	}
	return said.Content
}

// actionPeople is the fake queue store with a People of its own: one that fails with err, and
// that gives up, as a database does, for a caller who has gone away.
type actionPeople struct {
	*fakeQueue
	err error
}

func (f actionPeople) People(ctx context.Context, userIDs []string) ([]store.Person, error) {
	if err := cmp.Or(f.err, ctx.Err()); err != nil {
		return nil, err
	}
	return f.fakeQueue.People(ctx, userIDs)
}

// actionGoneAway makes a request one whose caller has left: its context is cancelled.
func actionGoneAway(r *http.Request) {
	ctx, cancel := context.WithCancel(r.Context())
	cancel()
	*r = *r.WithContext(ctx)
}

func TestActionRoutes(t *testing.T) {
	for _, c := range []struct {
		name, path string
		kind       queue.Kind
		hand       int64
	}{
		{"FR-QUEUE-3: Next takes the first waiting hand", "/api/sessions/12/next", queue.Next, 0},
		{"FR-QUEUE-3: Skip passes over the hand being answered", "/api/hands/101/skip", queue.Skip, actionNowID},
		{"FR-QUEUE-3: Skip passes over a waiting hand", "/api/hands/102/skip", queue.Skip, actionWaitingID},
		{"FR-MOD-1: Remove takes a hand out of the queue", "/api/hands/102/remove", queue.Remove, actionWaitingID},
		{"FR-HAND-8: Merge joins a waiting hand to the one being answered", "/api/hands/102/merge", queue.Merge, actionWaitingID},
	} {
		t.Run(c.name, func(t *testing.T) {
			a := actionApp(t)
			since := now()
			res, body := call(a.mux, "POST", c.path, "{}", a.as(t, hostID))
			actionQueued(t, res, body)
			actionAsked(t, a, c.kind, c.hand, since)
			if a.chat.types() != "OH:Queue" {
				t.Errorf("the room got %q, want the ping and nothing else", a.chat.types())
			}
			actionPinged(t, a)
		})
	}

	t.Run("FR-QUEUE-2, FR-QUEUE-3: what the rules refuse is a sentence, and the room hears nothing", func(t *testing.T) {
		for _, c := range []struct {
			path   string
			refuse error
			status int
			says   string
		}{
			{"/api/sessions/12/next", queue.ErrAnswering, http.StatusConflict, "Finish it first"},
			{"/api/sessions/12/next", queue.ErrNothingWaiting, http.StatusConflict, "Nobody is waiting"},
			{"/api/sessions/12/next", queue.ErrNotLive, http.StatusConflict, "no live session"},
			{"/api/hands/101/skip", queue.ErrNoSuchHand, http.StatusConflict, "no longer in the queue"},
			{"/api/hands/102/remove", queue.ErrNoSuchHand, http.StatusConflict, "no longer in the queue"},
			{"/api/hands/101/away", queue.ErrIsHere, http.StatusConflict, "said they are here"}, // FR-QUEUE-9, AT-17
			{"/api/hands/102/away", queue.ErrNotAnswering, http.StatusConflict, "not being answered"},
			{"/api/hands/101/merge", queue.ErrBadStatus, http.StatusConflict, "any more"},
			{"/api/hands/102/merge", queue.ErrNotAnswering, http.StatusConflict, "not being answered"},
			{"/api/hands/101/skip", store.ErrNotBuilt, http.StatusNotImplemented, "isn't built yet"},
			{"/api/hands/101/skip", errors.New("connection reset"), http.StatusInternalServerError, "Something went wrong"},
		} {
			a := actionApp(t)
			a.queue.refuse = c.refuse
			name := fmt.Sprintf("%s refused with %v", c.path, c.refuse)
			res, body := call(a.mux, "POST", c.path, "{}", a.as(t, hostID))
			refused(t, name, res, body, c.status, c.says)
			if len(a.queue.asked) != 1 || a.chat.types() != "" {
				t.Errorf("%s: the store was asked %d times and the room got %q; want one question and silence", name, len(a.queue.asked), a.chat.types())
			}
		}
	})

	t.Run("NFR-SEC-3: an id nobody has, and a body where none belongs, never reach the store", func(t *testing.T) {
		for _, c := range []struct {
			path, body string
			status     int
			says       string
		}{
			{"/api/sessions/999/next", "{}", http.StatusNotFound, "no such session"},
			{"/api/sessions/0/next", "{}", http.StatusNotFound, "no such session"},
			{"/api/hands/999/skip", "{}", http.StatusNotFound, "no such hand"},
			{"/api/hands/abc/remove", "{}", http.StatusNotFound, "no such hand"},
			{"/api/hands/-101/away", "{}", http.StatusNotFound, "no such hand"},
			{"/api/hands/999/merge", "{}", http.StatusNotFound, "no such hand"},
			{"/api/hands/999/done", doneBody(t, doneAnswer), http.StatusNotFound, "no such hand"},
			{"/api/sessions/12/next", `{"handId":"102"}`, http.StatusBadRequest, "not the JSON"},
			{"/api/hands/101/skip", `{"status":"answered"}`, http.StatusBadRequest, "not the JSON"},
			{"/api/hands/101/away", ``, http.StatusBadRequest, "not the JSON"},
		} {
			a := actionApp(t)
			name := c.path + " " + c.body
			res, body := call(a.mux, "POST", c.path, c.body, a.as(t, hostID))
			refused(t, name, res, body, c.status, c.says)
			actionUntouched(t, name, a)
		}
	})
}

func TestActionAway(t *testing.T) {
	const (
		back    = "@login-gh2 didn't answer when their turn came, so their question moved back two places."
		toEnd   = "@login-gh2 didn't answer when their turn came, so their question moved to the end of the line."
		skipped = "@login-gh2 missed their turn three times, so their question was skipped. They can raise it again."
	)
	// missed is what the store answers for an Away on askerID's hand: the queue before and after
	// it, with the hand as the rules left it and that many other hands waiting.
	missed := func(status queue.Status, misses, others int) store.Applied {
		before := queue.Hand{ID: actionNowID, AuthorID: askerID, Title: actionTitle, Status: queue.Answering, AwayCount: misses - 1}
		after := queue.Hand{ID: actionNowID, AuthorID: askerID, Title: actionTitle, Status: status, AwayCount: misses}
		ap := store.Applied{Version: actionVersion, Before: queue.State{Live: true, Hands: []queue.Hand{before}}, After: queue.State{Live: true, Hands: []queue.Hand{after}}}
		for i := range others {
			waiting := queue.Hand{ID: actionWaitingID + int64(i), AuthorID: fmt.Sprintf("gh%d", 50+i), Status: queue.Waiting}
			ap.Before.Hands, ap.After.Hands = append(ap.Before.Hands, waiting), append(ap.After.Hands, waiting)
		}
		return ap
	}
	// away presses Away on the hand being answered and returns what the room was told in words.
	away := func(t *testing.T, a *queueApp, mutate ...func(*http.Request)) string {
		t.Helper()
		since := now()
		res, body := call(a.mux, "POST", "/api/hands/101/away", "{}", a.as(t, hostID), mutate...)
		actionQueued(t, res, body)
		actionAsked(t, a, queue.Away, actionNowID, since)
		if a.chat.types() != "OH:Queue RC:TxtMsg" {
			t.Fatalf("the room got %q, want the ping and one line", a.chat.types())
		}
		actionPinged(t, a)
		line := a.chat.one(t, typeText)
		if line.From != botID || line.ChannelID != groupID(queueProject) || line.Transient {
			t.Errorf("FR-QUEUE-7: the line was sent by %q into %q (transient: %v), want the bot into the project's room, stored", line.From, line.ChannelID, line.Transient)
		}
		return actionSaid(t, line)
	}

	for _, c := range []struct {
		name   string
		status queue.Status
		misses int
		others int
		says   string
	}{
		{"a first miss moves the hand back two places", queue.Waiting, 1, 5, back},
		{"a first miss with two others waiting is two places back as well", queue.Waiting, 1, 2, back},
		{"a first miss with one other waiting: the room is not told a number that is wrong", queue.Waiting, 1, 1, toEnd},
		{"a first miss with nobody else waiting", queue.Waiting, 1, 0, toEnd},
		{"a second miss moves the hand to the end of the line", queue.Waiting, 2, 5, toEnd},
		{"a third miss skips the hand, and its author may raise it again", queue.Skipped, 3, 5, skipped},
	} {
		t.Run("FR-QUEUE-8, AT-17: "+c.name, func(t *testing.T) {
			a := actionApp(t)
			a.queue.applied = missed(c.status, c.misses, c.others)
			if got := away(t, a); got != c.says {
				t.Errorf("the room was told %q, want %q", got, c.says)
			}
		})
	}

	t.Run("FR-QUEUE-8: an author nobody can name is still not moved silently", func(t *testing.T) {
		const unnamed = "Someone didn't answer when their turn came, so their question moved back two places."
		a := actionApp(t)
		a.queue.applied = missed(queue.Waiting, 1, 3)
		delete(a.queue.people, askerID)
		if got := away(t, a); got != unnamed {
			t.Errorf("an author the store does not know: the room was told %q, want %q", got, unnamed)
		}

		a = actionApp(t)
		a.queue.applied = missed(queue.Waiting, 1, 3)
		a.deps.queue = actionPeople{fakeQueue: a.queue, err: errors.New("connection reset")}
		a.remux()
		if got := away(t, a); got != unnamed {
			t.Errorf("a lookup that fails: the room was told %q, want %q", got, unnamed)
		}
	})

	t.Run("FR-QUEUE-8: a store whose answer does not hold the hand: the action stands and the room gets the ping", func(t *testing.T) {
		a := actionApp(t) // its store answers with a queue that has no hands in it
		since := now()
		res, body := call(a.mux, "POST", "/api/hands/101/away", "{}", a.as(t, hostID))
		actionQueued(t, res, body)
		actionAsked(t, a, queue.Away, actionNowID, since)
		if a.chat.types() != "OH:Queue" {
			t.Errorf("the room got %q, want the ping alone: nothing is known that could be said in words", a.chat.types())
		}
	})

	t.Run("plan decision 6: the room is told who it was even when the maintainer's page went away once the action was written", func(t *testing.T) {
		a := actionApp(t)
		a.queue.applied = missed(queue.Waiting, 1, 3)
		a.deps.queue = actionPeople{fakeQueue: a.queue}
		a.remux()
		if got := away(t, a, actionGoneAway); got != back {
			t.Errorf("the room was told %q, want %q", got, back)
		}
	})
}

// doneAnswer is an answer a Done accepts.
const doneAnswer = "Give RetryOn a class: RetryOn(Status5xx) retries every 5xx reply and nothing else."

// doneBody is the JSON the Done sheet sends.
func doneBody(t *testing.T, answer string, links ...string) string {
	t.Helper()
	body, err := json.Marshal(map[string]any{"answer": answer, "links": links})
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

// doneCard is what an OH:FAQ card carries.
type doneCard struct {
	FAQID   string   `json:"faqId"`
	Slug    string   `json:"slug"`
	Title   string   `json:"title"`
	AskedBy []string `json:"askedBy"`
}

// doneCardSent reads the one FAQ card the room got.
func doneCardSent(t *testing.T, a *queueApp) doneCard {
	t.Helper()
	var card doneCard
	m := a.chat.one(t, typeFAQ)
	if err := json.Unmarshal([]byte(m.Content), &card); err != nil {
		t.Fatalf("the FAQ card is not JSON: %v in %s", err, m.Content)
	}
	return card
}

func TestDoneHand(t *testing.T) {
	const path = "/api/hands/101/done"
	// answered is what the store answers for a Done on askerID's hand. The transaction saw two
	// more askers than the queue the route read a moment before it: someone merged a hand in.
	answered := func(a *queueApp) {
		a.queue.applied = store.Applied{Version: actionVersion, Before: queue.State{Live: true, Hands: []queue.Hand{
			{ID: actionNowID, AuthorID: askerID, Title: actionTitle, Details: actionDetails, Status: queue.Answering, Plus: []string{otherID, "gh404", hostID}},
		}}}
		a.queue.entry = store.FAQEntry{ID: 7, ProjectID: queueProject, HandID: actionNowID, Slug: "how-do-i-retry-only-on-5xx-2", Title: actionTitle}
	}

	t.Run("FR-FAQ-1, FR-FAQ-2, FR-HAND-8: the answer becomes a FAQ entry, and its card names everyone who asked", func(t *testing.T) {
		a := actionApp(t)
		answered(a)
		since := now()
		typed := "  Give RetryOn a class:\r\nRetryOn(Status5xx) retries every 5xx reply.\r\n  "
		res, body := call(a.mux, "POST", path, doneBody(t, typed, " https://example.com/docs/retry ", "", "HTTPS://Example.com/issues/12#note"), a.as(t, hostID))
		actionQueued(t, res, body)
		actionAsked(t, a, queue.Done, actionNowID, since)

		want := store.FAQDraft{
			Slug: "how-do-i-retry-only-on-5xx", Title: actionTitle, Question: actionDetails,
			Answer:   "Give RetryOn a class:\nRetryOn(Status5xx) retries every 5xx reply.",
			Links:    []string{"https://example.com/docs/retry", "https://Example.com/issues/12#note"},
			AuthorID: hostID,
		}
		if len(a.queue.drafts) != 1 || !reflect.DeepEqual(a.queue.drafts[0], want) {
			t.Errorf("the store got the drafts %+v, want one: %+v", a.queue.drafts, want)
		}

		if a.chat.types() != "OH:FAQ OH:Queue" {
			t.Fatalf("the room got %q, want the FAQ card and the ping", a.chat.types())
		}
		actionPinged(t, a)
		// The slug is the one the store made unique, and the names are the author's and then the
		// +1s' in the order of the hand the transaction saw; an id the store does not know has none.
		wantCard := doneCard{FAQID: "7", Slug: "how-do-i-retry-only-on-5xx-2", Title: actionTitle, AskedBy: []string{"login-gh2", "login-gh3", "login-gh1"}}
		if card := doneCardSent(t, a); !reflect.DeepEqual(card, wantCard) {
			t.Errorf("the FAQ card is %+v, want %+v", card, wantCard)
		}
	})

	t.Run("FR-FAQ-1, NFR-SEC-3: an answer is 20 to 2,000 characters with at most 5 web addresses", func(t *testing.T) {
		link := func(n int) string { return fmt.Sprintf("https://example.com/docs/%d", n) }
		const site = "https://example.com/" // 20 characters
		for _, c := range []struct {
			name   string
			answer string
			links  []string
			says   string // "" when the answer is taken
		}{
			{"19 characters", strings.Repeat("a", 19), nil, "20 to 2,000 characters"},
			{"20 characters", strings.Repeat("a", 20), nil, ""},
			{"2,000 characters", strings.Repeat("a", 2000), nil, ""},
			{"2,001 characters", strings.Repeat("a", 2001), nil, "20 to 2,000 characters"},
			{"19 characters between spaces and line breaks", " \r\n\t" + strings.Repeat("a", 19) + " \n\n ", nil, "20 to 2,000 characters"},
			{"20 characters of three bytes each", strings.Repeat("語", 20), nil, ""},
			{"2,000 characters of three bytes each", strings.Repeat("語", 2000), nil, ""},
			{"no answer", "", nil, "20 to 2,000 characters"},
			{"5 links", doneAnswer, []string{link(1), link(2), link(3), link(4), link(5)}, ""},
			{"6 links", doneAnswer, []string{link(1), link(2), link(3), link(4), link(5), link(6)}, "at most 5 links"},
			{"5 links between fields left empty", doneAnswer, []string{"", link(1), " ", link(2), link(3), "\n", link(4), link(5), ""}, ""},
			{"a javascript: link", doneAnswer, []string{"javascript:alert(document.cookie)"}, "Link 1 isn't a web address"},
			{"a javascript: link in capitals, after a good one", doneAnswer, []string{link(1), "JaVaScRiPt:alert(1)"}, "Link 2 isn't a web address"},
			{"a data: link", doneAnswer, []string{"data:text/html,<script>alert(1)</script>"}, "Link 1 isn't a web address"},
			{"an ftp: link", doneAnswer, []string{"ftp://example.com/stash.tar.gz"}, "Link 1 isn't a web address"},
			{"a mailto: link", doneAnswer, []string{"mailto:mia@example.com"}, "Link 1 isn't a web address"},
			{"a link without a host", doneAnswer, []string{"https:///docs/retry"}, "Link 1 isn't a web address"},
			{"a scheme and nothing else", doneAnswer, []string{"https://"}, "Link 1 isn't a web address"},
			{"a port without a host", doneAnswer, []string{"http://:8080/docs"}, "Link 1 isn't a web address"},
			{"no slashes after the scheme", doneAnswer, []string{"https:example.com/docs"}, "Link 1 isn't a web address"},
			{"a link to a path of ours", doneAnswer, []string{"/p/paperplane/stash/faq"}, "Link 1 isn't a web address"},
			{"a host without a scheme", doneAnswer, []string{"example.com/docs"}, "Link 1 isn't a web address"},
			{"a link that borrows the page's scheme", doneAnswer, []string{"//example.com/docs"}, "Link 1 isn't a web address"},
			{"words", doneAnswer, []string{"see the docs"}, "Link 1 isn't a web address"},
			{"a space in the host", doneAnswer, []string{"https://exa mple.com/docs"}, "Link 1 isn't a web address"},
			{"two links in one field", doneAnswer, []string{"https://example.com/a https://example.com/b"}, "Link 1 isn't a web address"},
			{"two links on two lines of one field", doneAnswer, []string{"https://example.com/a\nhttps://example.com/b"}, "Link 1 isn't a web address"},
			{"a space that does not break in a path", doneAnswer, []string{"https://example.com/a b"}, "Link 1 isn't a web address"},
			{"a broken escape", doneAnswer, []string{"https://example.com/%zz"}, "Link 1 isn't a web address"},
			{"a link of 500 characters", doneAnswer, []string{site + strings.Repeat("a", 480)}, ""},
			{"a link of 500 characters of three bytes each", doneAnswer, []string{site + strings.Repeat("語", 480)}, ""},
			{"a link of 501 characters", doneAnswer, []string{link(1), site + strings.Repeat("a", 481)}, "Link 2 is longer than 500 characters"},
			{"a repeated link", doneAnswer, []string{link(1), link(2), link(1)}, "Link 3 repeats"},
			{"a repeated link, typed with capitals and spaces around it", doneAnswer, []string{link(1), "  HTTPS" + strings.TrimPrefix(link(1), "https") + " "}, "Link 2 repeats"},
		} {
			a := actionApp(t)
			answered(a)
			res, body := call(a.mux, "POST", path, doneBody(t, c.answer, c.links...), a.as(t, hostID))
			if c.says != "" {
				refused(t, c.name, res, body, http.StatusBadRequest, c.says)
				actionUntouched(t, c.name, a)
				continue
			}
			if res.StatusCode != http.StatusOK || len(a.queue.drafts) != 1 || a.chat.types() != "OH:FAQ OH:Queue" {
				t.Errorf("%s: status %d body %s, %d drafts, the room got %q; want the answer taken", c.name, res.StatusCode, body, len(a.queue.drafts), a.chat.types())
			}
		}
	})

	t.Run("NFR-SEC-3: a body that is not an answer and its links is refused", func(t *testing.T) {
		for _, body := range []string{`{"answer":20}`, `{"answer":"` + doneAnswer + `","links":"https://example.com"}`, `{"answer":"` + doneAnswer + `","links":[1]}`, `{"answer":"` + doneAnswer + `","slug":"mine"}`, `[]`, ``} {
			a := actionApp(t)
			res, reply := call(a.mux, "POST", path, body, a.as(t, hostID))
			refused(t, body, res, reply, http.StatusBadRequest, "not the JSON")
			actionUntouched(t, body, a)
		}
	})

	t.Run("FR-QUEUE-2: only the hand being answered can be finished", func(t *testing.T) {
		a := actionApp(t)
		res, body := call(a.mux, "POST", "/api/hands/102/done", doneBody(t, doneAnswer), a.as(t, hostID))
		refused(t, "a hand that waits", res, body, http.StatusConflict, "not being answered")
		actionUntouched(t, "a hand that waits", a)

		a = actionApp(t)
		a.queue.hands[100] = liveSession // answered a while ago: it is no longer among the open hands
		res, body = call(a.mux, "POST", "/api/hands/100/done", doneBody(t, doneAnswer), a.as(t, hostID))
		refused(t, "a hand that left the queue", res, body, http.StatusConflict, "not being answered")
		actionUntouched(t, "a hand that left the queue", a)

		// Two maintainers press Done at the same moment: the second one's hand was being answered
		// when the route read it, and the rules refuse it inside the transaction.
		for _, c := range []struct {
			refuse error
			status int
			says   string
		}{
			{queue.ErrNotAnswering, http.StatusConflict, "not being answered"},
			{queue.ErrNotLive, http.StatusConflict, "no live session"},
			{store.ErrNotBuilt, http.StatusNotImplemented, "isn't built yet"},
			{errors.New("connection reset"), http.StatusInternalServerError, "Something went wrong"},
		} {
			a = actionApp(t)
			a.queue.refuse = c.refuse
			res, body = call(a.mux, "POST", path, doneBody(t, doneAnswer), a.as(t, hostID))
			refused(t, c.refuse.Error(), res, body, c.status, c.says)
			if len(a.queue.asked) != 1 || a.chat.types() != "" {
				t.Errorf("refused with %v: the store was asked %d times and the room got %q; want one question and silence", c.refuse, len(a.queue.asked), a.chat.types())
			}
		}
	})

	t.Run("FR-FAQ-1: a title without a Latin letter still gets a slug", func(t *testing.T) {
		a := actionApp(t)
		answered(a)
		a.queue.snapshot.Open[0].Title = "リトライの設定方法を教えてください"
		res, body := call(a.mux, "POST", path, doneBody(t, doneAnswer), a.as(t, hostID))
		if res.StatusCode != http.StatusOK || len(a.queue.drafts) != 1 || a.queue.drafts[0].Slug != "question" || a.queue.drafts[0].Title != "リトライの設定方法を教えてください" {
			t.Fatalf("status %d body %s, drafts %+v; want one with the slug \"question\" and the title as it was typed", res.StatusCode, body, a.queue.drafts)
		}
		if got := a.queue.drafts[0].Links; got == nil || len(got) != 0 {
			t.Errorf("an answer without links has the links %#v, want an empty list that is not nil", got)
		}
	})

	t.Run("plan decision 6: an entry that is written is announced, with or without the names", func(t *testing.T) {
		a := actionApp(t)
		answered(a)
		a.deps.queue = actionPeople{fakeQueue: a.queue, err: errors.New("connection reset")}
		a.remux()
		res, body := call(a.mux, "POST", path, doneBody(t, doneAnswer), a.as(t, hostID))
		actionQueued(t, res, body)
		if card := doneCardSent(t, a); card.Slug != "how-do-i-retry-only-on-5xx-2" || card.AskedBy == nil || len(card.AskedBy) != 0 {
			t.Errorf("names that cannot be looked up: the card is %+v, want the entry with an empty list of names", card)
		}

		a = actionApp(t)
		answered(a)
		a.deps.queue = actionPeople{fakeQueue: a.queue}
		a.remux()
		res, body = call(a.mux, "POST", path, doneBody(t, doneAnswer), a.as(t, hostID), actionGoneAway)
		actionQueued(t, res, body)
		if card := doneCardSent(t, a); len(card.AskedBy) != 3 {
			t.Errorf("a maintainer whose page went away: the card names %v, want all three who asked", card.AskedBy)
		}
	})
}

func TestMuteMember(t *testing.T) {
	const path = "/api/p/paperplane/stash/mute"

	t.Run("FR-MOD-1, AT-18: a maintainer mutes a member in the project's room for the minutes asked", func(t *testing.T) {
		for _, c := range []struct {
			body    string
			minutes int
		}{
			{`{"userId":"gh2","minutes":30}`, 30},
			{`{"userId":"gh2","minutes":1}`, 1},
			{`{"userId":"gh2","minutes":1440}`, 1440},
			{`{"userId":"gh2","minutes":0}`, 10},
			{`{"userId":"gh2"}`, 10},
		} {
			a := actionApp(t)
			res, body := call(a.mux, "POST", path, c.body, a.as(t, hostID))
			var got struct {
				UserID  string `json:"userId"`
				Minutes int    `json:"minutes"`
			}
			decode(t, body, &got)
			if res.StatusCode != http.StatusOK || got.UserID != askerID || got.Minutes != c.minutes {
				t.Errorf("%s: status %d body %s, want %s muted for %d minutes", c.body, res.StatusCode, body, askerID, c.minutes)
			}
			if want := []string{fmt.Sprintf("%s %s %d", groupID(queueProject), askerID, c.minutes)}; !reflect.DeepEqual(a.chat.muted, want) {
				t.Errorf("%s: the chat muted %v, want %v", c.body, a.chat.muted, want)
			}
			if len(a.queue.asked) != 0 || a.chat.types() != "" {
				t.Errorf("%s: a mute changed the queue (%+v) or was said in the room (%q)", c.body, a.queue.asked, a.chat.types())
			}
		}
	})

	t.Run("FR-MOD-1: only someone in the room who does not maintain the project can be muted", func(t *testing.T) {
		for _, c := range []struct {
			name, userID string
			status       int
			says         string
		}{
			{"the maintainer who asks", hostID, http.StatusForbidden, "maintainer can't be muted"},
			{"another maintainer", "gh4", http.StatusForbidden, "maintainer can't be muted"},
			{"someone who is not in the room", strangerID, http.StatusNotFound, "not in this room"},
			{"someone nobody knows", "gh123456789", http.StatusNotFound, "not in this room"},
		} {
			a := actionApp(t)
			a.projects.maintainers[queueProject+" gh4"] = true
			a.projects.join(queueProject, "gh4")
			res, body := call(a.mux, "POST", path, `{"userId":"`+c.userID+`","minutes":10}`, a.as(t, hostID))
			refused(t, c.name, res, body, c.status, c.says)
			actionUntouched(t, c.name, a)
		}
	})

	t.Run("NFR-SEC-3: an id that is no person's and minutes out of range are refused", func(t *testing.T) {
		for _, c := range []struct{ body, says string }{
			{`{"minutes":10}`, "not a user id"},
			{`{"userId":"","minutes":10}`, "not a user id"},
			{`{"userId":"gh","minutes":10}`, "not a user id"},
			{`{"userId":"oh_bot","minutes":10}`, "not a user id"},
			{`{"userId":"2","minutes":10}`, "not a user id"},
			{`{"userId":"GH2","minutes":10}`, "not a user id"},
			{`{"userId":"ghx2","minutes":10}`, "not a user id"},
			{`{"userId":"gh-2","minutes":10}`, "not a user id"},
			{`{"userId":"gh+2","minutes":10}`, "not a user id"},
			{`{"userId":"gh2 ","minutes":10}`, "not a user id"},
			{`{"userId":" gh2","minutes":10}`, "not a user id"},
			{`{"userId":"gh2/../gh3","minutes":10}`, "not a user id"},
			{`{"userId":"gh٢","minutes":10}`, "not a user id"},
			{`{"userId":"gh99999999999999999999","minutes":10}`, "not a user id"},
			{`{"userId":"gh2","minutes":-1}`, "1 to 1,440 minutes"},
			{`{"userId":"gh2","minutes":1441}`, "1 to 1,440 minutes"},
			{`{"userId":"gh2","minutes":4294967297}`, "1 to 1,440 minutes"},
			{`{"userId":"gh2","minutes":"10"}`, "not the JSON"},
			{`{"userId":"gh2","minutes":1.5}`, "not the JSON"},
			{`{"userId":"gh2","hours":1}`, "not the JSON"},
			{`{"userId":["gh2","gh3"],"minutes":10}`, "not the JSON"},
		} {
			a := actionApp(t)
			res, body := call(a.mux, "POST", path, c.body, a.as(t, hostID))
			refused(t, c.body, res, body, http.StatusBadRequest, c.says)
			actionUntouched(t, c.body, a)
		}
	})

	t.Run("a server without a chat says so, and a chat that fails is a 502 in our own words", func(t *testing.T) {
		a := actionApp(t)
		a.deps.chat = nil
		a.remux()
		res, body := call(a.mux, "POST", path, `{"userId":"gh2","minutes":10}`, a.as(t, hostID))
		refused(t, "without a chat", res, body, http.StatusServiceUnavailable, "isn't set up")

		a = actionApp(t)
		a.chat.err = errors.New("nexconn mute: http 200, code 1004: signature error for app key pk_live")
		res, body = call(a.mux, "POST", path, `{"userId":"gh2","minutes":10}`, a.as(t, hostID))
		refused(t, "a chat that fails", res, body, http.StatusBadGateway, "couldn't mute")
		if strings.Contains(body, "1004") || strings.Contains(body, "pk_live") {
			t.Errorf("the reply carries the chat service's own words: %s", body)
		}

		a = actionApp(t)
		a.projects.err = errors.New("connection reset")
		res, body = call(a.mux, "POST", path, `{"userId":"gh2","minutes":10}`, a.as(t, hostID))
		refused(t, "a store that fails", res, body, http.StatusInternalServerError, "Something went wrong")
		actionUntouched(t, "a store that fails", a)
	})
}

// AT-10: what a maintainer does with the queue is refused for everyone else, before anything is
// applied, written or said.
func TestActionsAreForMaintainersOnly(t *testing.T) {
	routes := func(t *testing.T) []struct{ path, body string } {
		return []struct{ path, body string }{
			{"/api/sessions/12/next", "{}"},
			{"/api/hands/101/skip", "{}"},
			{"/api/hands/102/remove", "{}"},
			{"/api/hands/101/away", "{}"},
			{"/api/hands/102/merge", "{}"},
			{"/api/hands/101/done", doneBody(t, doneAnswer, "https://example.com/docs/retry")},
			{"/api/p/paperplane/stash/mute", `{"userId":"gh3","minutes":10}`},
		}
	}
	otherSite := func(r *http.Request) { r.Header.Set("Origin", "https://evil.example") }
	noOrigin := func(r *http.Request) { r.Header.Del("Origin") }
	form := func(r *http.Request) { r.Header.Set("Content-Type", "application/x-www-form-urlencoded") }

	for _, c := range []struct {
		name   string
		userID string // "" is signed out
		mutate func(*http.Request)
		status int
		says   string
	}{
		{"AT-10, NFR-SEC-2: a member, who is also the author of the hand being answered", askerID, nil, http.StatusForbidden, "Only a maintainer"},
		{"AT-10, NFR-SEC-2: a member who maintains another project", otherID, nil, http.StatusForbidden, "Only a maintainer"},
		{"NFR-SEC-2: someone who is not in the room", strangerID, nil, http.StatusForbidden, "Only a maintainer"},
		{"NFR-SEC-2: someone who is not signed in", "", nil, http.StatusUnauthorized, "Sign in first"},
		{"AT-10, NFR-SEC-5: the maintainer's browser, sent by another site", hostID, otherSite, http.StatusForbidden, "did not come from Office Hours"},
		{"NFR-SEC-5: the maintainer, without an Origin", hostID, noOrigin, http.StatusForbidden, "did not come from Office Hours"},
		{"NFR-SEC-5: the maintainer, with a form that is not JSON", hostID, form, http.StatusUnsupportedMediaType, "JSON"},
	} {
		t.Run(c.name, func(t *testing.T) {
			for _, route := range routes(t) {
				a := actionApp(t)
				kite := store.Project{ID: "prj2", GitHubID: int64(githubID("PaperPlane", "Kite")), Owner: "PaperPlane", Repo: "Kite"}
				a.projects.byRepo[repoKey(kite.Owner, kite.Repo)] = kite
				a.projects.maintainers[kite.ID+" "+otherID] = true
				a.projects.join(kite.ID, otherID)

				var cookie *http.Cookie
				if c.userID != "" {
					cookie = a.as(t, c.userID)
				}
				var mutate []func(*http.Request)
				if c.mutate != nil {
					mutate = append(mutate, c.mutate)
				}
				res, body := call(a.mux, "POST", route.path, route.body, cookie, mutate...)
				refused(t, route.path, res, body, c.status, c.says)
				actionUntouched(t, route.path, a)
			}
		})
	}
}
