package main

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"time"

	"officehours/guard"
	"officehours/nexconn"
	"officehours/queue"
	"officehours/session"
	"officehours/store"
)

// What every queue route shares: who may call it, how a refusal of the rules is worded, how the
// room is told, and the one JSON shape of a queue. The routes themselves are in api_sessions.go,
// api_hands.go and api_actions.go. This file is a contract (notes/phase-2-plan.md).

// queueStore is the part of the store the queue routes use.
type queueStore interface {
	Queue(ctx context.Context, projectID string) (store.Snapshot, error)
	Apply(ctx context.Context, sessionID int64, a queue.Action) (store.Applied, error)
	Done(ctx context.Context, sessionID int64, a queue.Action, draft store.FAQDraft) (store.Applied, store.FAQEntry, error)
	ProjectOfSession(ctx context.Context, sessionID int64) (projectID string, found bool, err error)
	ProjectOfHand(ctx context.Context, handID int64) (projectID string, sessionID int64, found bool, err error)
	People(ctx context.Context, userIDs []string) ([]store.Person, error)
}

const (
	queueNotSetUp = "The queue isn't set up on this server yet."
	notBuiltYet   = "This part of Office Hours isn't built yet."
)

// notBuilt is what a route answers until its part of Phase 2 is built.
func notBuilt(w http.ResponseWriter) {
	writeJSON(w, http.StatusNotImplemented, envelope{Error: notBuiltYet})
}

// now is the moment an action happens. Postgres keeps microseconds, so the rules never see more:
// a time that is written and read back is the same time.
func now() time.Time { return time.Now().UTC().Truncate(time.Microsecond) }

// role is what a route asks of its caller.
type role int

const (
	asMember role = iota
	asMaintainer
)

// target is what a queue mutation is about, once its caller has been checked.
type target struct {
	User      session.User
	ProjectID string
	SessionID int64
	HandID    int64 // 0 on a session's route
}

// pathID reads the {id} of /api/sessions/{id}/… and /api/hands/{id}/….
func pathID(r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	return id, err == nil && id > 0
}

// allow asks the roles tables whether the user may act in the project (NFR-SEC-2).
func (d deps) allow(ctx context.Context, u session.User, projectID string, need role) error {
	if need == asMaintainer {
		return d.guard.Maintainer(ctx, u, projectID)
	}
	return d.guard.Member(ctx, u, projectID)
}

// onSession starts a mutation on /api/sessions/{id}/…: the guard's checks (our origin, signed
// in, JSON decoded into in), then the session's project, then the caller's role in it. When it
// answers false it has written the refusal itself.
func (d deps) onSession(w http.ResponseWriter, r *http.Request, in any, need role) (target, bool) {
	u, err := d.guard.Mutation(w, r, in)
	if err != nil {
		writeError(w, err)
		return target{}, false
	}
	if d.queue == nil {
		writeJSON(w, http.StatusServiceUnavailable, envelope{Error: queueNotSetUp})
		return target{}, false
	}
	id, ok := pathID(r)
	if !ok {
		writeJSON(w, http.StatusNotFound, envelope{Error: "There is no such session."})
		return target{}, false
	}
	projectID, found, err := d.queue.ProjectOfSession(r.Context(), id)
	if err != nil {
		writeError(w, queueRefusal(err))
		return target{}, false
	}
	if !found {
		writeJSON(w, http.StatusNotFound, envelope{Error: "There is no such session."})
		return target{}, false
	}
	if err := d.allow(r.Context(), u, projectID, need); err != nil {
		writeError(w, err)
		return target{}, false
	}
	return target{User: u, ProjectID: projectID, SessionID: id}, true
}

// onHand is onSession for /api/hands/{id}/….
func (d deps) onHand(w http.ResponseWriter, r *http.Request, in any, need role) (target, bool) {
	u, err := d.guard.Mutation(w, r, in)
	if err != nil {
		writeError(w, err)
		return target{}, false
	}
	if d.queue == nil {
		writeJSON(w, http.StatusServiceUnavailable, envelope{Error: queueNotSetUp})
		return target{}, false
	}
	id, ok := pathID(r)
	if !ok {
		writeJSON(w, http.StatusNotFound, envelope{Error: "There is no such hand."})
		return target{}, false
	}
	projectID, sessionID, found, err := d.queue.ProjectOfHand(r.Context(), id)
	if err != nil {
		writeError(w, queueRefusal(err))
		return target{}, false
	}
	if !found {
		writeJSON(w, http.StatusNotFound, envelope{Error: "There is no such hand."})
		return target{}, false
	}
	if err := d.allow(r.Context(), u, projectID, need); err != nil {
		writeError(w, err)
		return target{}, false
	}
	return target{User: u, ProjectID: projectID, SessionID: sessionID, HandID: id}, true
}

// onProject starts a mutation on /api/p/{owner}/{repo}/…: the guard's checks, then the project
// with the caller's role in it. When it answers false it has written the refusal itself.
func (d deps) onProject(w http.ResponseWriter, r *http.Request, in any, need role) (session.User, store.Project, bool) {
	u, err := d.guard.Mutation(w, r, in)
	if err != nil {
		writeError(w, err)
		return session.User{}, store.Project{}, false
	}
	project, ok := d.projectOfPath(w, r, u.ID)
	if !ok {
		return session.User{}, store.Project{}, false
	}
	switch {
	case need == asMaintainer && !project.IsMaintainer:
		writeError(w, &guard.Error{Status: http.StatusForbidden, Msg: "Only a maintainer of this project can do that."})
		return session.User{}, store.Project{}, false
	case !project.IsMember:
		writeError(w, &guard.Error{Status: http.StatusForbidden, Msg: "Join this room first."})
		return session.User{}, store.Project{}, false
	}
	return u, project, true
}

// queueRefusal turns what the rules and the store refuse into a status and a sentence the page
// can show as it is. Anything else passes through, and writeError makes it a 500.
func queueRefusal(err error) error {
	refuse := func(status int, msg string) error { return &guard.Error{Status: status, Msg: msg} }
	var soon *queue.TooSoonError
	switch {
	case errors.As(err, &soon):
		wait := max(1, int(math.Ceil(soon.Wait.Seconds())))
		return refuse(http.StatusTooManyRequests, fmt.Sprintf("You raised a hand a moment ago. Wait %d s before raising another.", wait)) // FR-HAND-6
	case errors.Is(err, queue.ErrNotLive):
		return refuse(http.StatusConflict, "There is no live session right now.")
	case errors.Is(err, queue.ErrHandsClosed):
		return refuse(http.StatusConflict, "Hands are closed for this session.") // FR-SESS-6
	case errors.Is(err, queue.ErrAlreadyOpen):
		return refuse(http.StatusConflict, "You already have an open hand in this session.") // FR-HAND-2
	case errors.Is(err, queue.ErrNoSuchHand):
		return refuse(http.StatusConflict, "That hand is no longer in the queue.")
	case errors.Is(err, queue.ErrNotYours):
		return refuse(http.StatusForbidden, "Only the person who raised this hand can do that.")
	case errors.Is(err, queue.ErrOwnHand):
		return refuse(http.StatusConflict, "You can't +1 your own hand.") // FR-HAND-7
	case errors.Is(err, queue.ErrNothingWaiting):
		return refuse(http.StatusConflict, "Nobody is waiting.")
	case errors.Is(err, queue.ErrAnswering):
		return refuse(http.StatusConflict, "A question is being answered. Finish it first.") // FR-QUEUE-3
	case errors.Is(err, queue.ErrNotAnswering):
		return refuse(http.StatusConflict, "That hand is not being answered right now.")
	case errors.Is(err, queue.ErrIsHere):
		return refuse(http.StatusConflict, "They said they are here, so they can't be marked away.") // FR-QUEUE-9
	case errors.Is(err, queue.ErrBadStatus):
		return refuse(http.StatusConflict, "That can't be done with this hand any more.") // FR-QUEUE-2
	case errors.Is(err, store.ErrSessionLive):
		return refuse(http.StatusConflict, "A session is already live in this room.") // FR-SESS-2
	case errors.Is(err, queue.ErrNotImplemented), errors.Is(err, store.ErrNotBuilt):
		return refuse(http.StatusNotImplemented, notBuiltYet)
	}
	return err
}

// finish ends a queue mutation that was written: it tells the room (the version ping, and the
// cards given) and answers with the queue as the caller sees it now.
func (d deps) finish(w http.ResponseWriter, r *http.Request, t target, ap store.Applied, cards ...nexconn.Message) {
	d.tell(r.Context(), t.ProjectID, append(cards, queuePing(ap.SessionID, ap.Version))...)
	d.answerQueue(w, r, t.ProjectID, t.User.ID)
}

// raisedHand is the hand an action added to the queue, with the id the store gave it.
func raisedHand(ap store.Applied) (queue.Hand, bool) {
	before := make(map[int64]bool, len(ap.Before.Hands))
	for _, h := range ap.Before.Hands {
		before[h.ID] = true
	}
	for _, h := range ap.After.Hands {
		if !before[h.ID] {
			return h, true
		}
	}
	return queue.Hand{}, false
}

// handIn finds a hand of a state by its id.
func handIn(s queue.State, id int64) (queue.Hand, bool) {
	for _, h := range s.Hands {
		if h.ID == id {
			return h, true
		}
	}
	return queue.Hand{}, false
}

// answerQueue answers with a project's queue as one of its members sees it.
func (d deps) answerQueue(w http.ResponseWriter, r *http.Request, projectID, viewerID string) {
	snap, err := d.queue.Queue(r.Context(), projectID)
	if err != nil {
		writeError(w, queueRefusal(err))
		return
	}
	writeJSON(w, http.StatusOK, envelope{OK: true, Data: describeQueue(snap, viewerID, now())})
}

// queuePage is GET /api/p/{owner}/{repo}/queue: what the room shows about the session and its
// queue (FR-QUEUE-4, FR-QUEUE-6). Anyone may ask, so the join page can say that a session is live
// (FR-PROJ-3), but the hands, with their authors and questions, go to members only.
func (d deps) queuePage(w http.ResponseWriter, r *http.Request) {
	viewer, _ := d.guard.User(r) // signed out is fine here: the zero user is nobody
	project, ok := d.projectOfPath(w, r, viewer.ID)
	if !ok {
		return
	}
	if d.queue == nil {
		writeJSON(w, http.StatusServiceUnavailable, envelope{Error: queueNotSetUp})
		return
	}
	snap, err := d.queue.Queue(r.Context(), project.ID)
	if err != nil {
		writeError(w, queueRefusal(err))
		return
	}
	if !project.IsMember {
		snap.Open, snap.Closed = nil, nil
	}
	writeJSON(w, http.StatusOK, envelope{OK: true, Data: describeQueue(snap, viewer.ID, now())})
}

// The queue as the API describes it to one caller. web/src/queue/types.ts is the same shape.
// Ids are strings: the cards in the chat carry them as strings too, and the page compares them.

type personJSON struct {
	UserID    string `json:"userId"`
	Login     string `json:"login"`
	Name      string `json:"name"`
	AvatarURL string `json:"avatarUrl"`
}

type sessionJSON struct {
	ID             string      `json:"id"`
	Number         int         `json:"number"`
	Title          string      `json:"title"`
	Live           bool        `json:"live"`
	HandsClosed    bool        `json:"handsClosed"`
	StartedAt      time.Time   `json:"startedAt"`
	EndedAt        *time.Time  `json:"endedAt"`
	PlannedMinutes int         `json:"plannedMinutes"` // 0 when no length was planned
	Version        int         `json:"version"`
	Host           *personJSON `json:"host"`
}

type handJSON struct {
	ID        string      `json:"id"`
	Title     string      `json:"title"`
	Details   string      `json:"details"`
	Status    string      `json:"status"`
	Author    personJSON  `json:"author"`
	RaisedAt  time.Time   `json:"raisedAt"`
	TakenAt   *time.Time  `json:"takenAt"`
	TakenBy   *personJSON `json:"takenBy"`
	AckedAt   *time.Time  `json:"ackedAt"` // "I'm here" (FR-QUEUE-9)
	ClosedAt  *time.Time  `json:"closedAt"`
	AwayCount int         `json:"awayCount"`
	PlusCount int         `json:"plusCount"`
	PlusByMe  bool        `json:"plusByMe"`
	IsMine    bool        `json:"isMine"`
	FAQID     string      `json:"faqId"`   // "" unless the hand was answered and its entry still exists
	FAQSlug   string      `json:"faqSlug"` // "" with an empty faqId
}

type queueJSON struct {
	Session             *sessionJSON `json:"session"`             // the live session, else the last one, else null
	Hands               []handJSON   `json:"hands"`               // open: the one being answered first, then the waiting ones in order
	Done                []handJSON   `json:"done"`                // the session's other hands, the latest first
	NextSessionAt       *time.Time   `json:"nextSessionAt"`       // FR-SESS-5
	MedianAnswerSeconds int          `json:"medianAnswerSeconds"` // 0 until two hands were answered (FR-QUEUE-10)
	ServerTime          time.Time    `json:"serverTime"`          // so a page can count minutes without trusting its own clock
}

func timeOrNull(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}

func personOrNull(p store.Person) *personJSON {
	if p.UserID == "" {
		return nil
	}
	out := personJSON(p)
	return &out
}

func describeHands(hands []store.QueueHand, viewerID string) []handJSON {
	out := make([]handJSON, 0, len(hands)) // an empty list is [] in JSON, never null
	for _, h := range hands {
		plusByMe := false
		for _, id := range h.Plus {
			plusByMe = plusByMe || (viewerID != "" && id == viewerID)
		}
		// What a maintainer removed is no longer shown to the room (FR-MOD-1): the hand stays in
		// the list, so its card can say what became of it, but without its words.
		if h.Status == queue.Removed {
			h.Title, h.Details = "", ""
		}
		faqID := ""
		if h.FAQID != 0 {
			faqID = idText(h.FAQID)
		}
		out = append(out, handJSON{
			ID: idText(h.ID), Title: h.Title, Details: h.Details, Status: string(h.Status),
			Author: personJSON(h.Author), RaisedAt: h.RaisedAt,
			TakenAt: timeOrNull(h.TakenAt), TakenBy: personOrNull(h.Taker),
			AckedAt: timeOrNull(h.AckedAt), ClosedAt: timeOrNull(h.ClosedAt),
			AwayCount: h.AwayCount, PlusCount: len(h.Plus), PlusByMe: plusByMe,
			IsMine: viewerID != "" && h.AuthorID == viewerID, FAQID: faqID, FAQSlug: h.FAQSlug,
		})
	}
	return out
}

// describeQueue is a snapshot as one viewer sees it. Only "is this mine" and "did I +1 this"
// depend on the viewer.
func describeQueue(snap store.Snapshot, viewerID string, at time.Time) queueJSON {
	out := queueJSON{
		Hands: describeHands(snap.Open, viewerID), Done: describeHands(snap.Closed, viewerID),
		NextSessionAt:       timeOrNull(snap.NextSessionAt),
		MedianAnswerSeconds: int(snap.MedianAnswer.Round(time.Second).Seconds()),
		ServerTime:          at,
	}
	if s := snap.Session; s != nil {
		out.Session = &sessionJSON{
			ID: idText(s.ID), Number: s.Number, Title: s.Title, Live: s.Live, HandsClosed: s.HandsClosed,
			StartedAt: s.StartedAt, EndedAt: timeOrNull(s.EndedAt), PlannedMinutes: s.PlannedMinutes,
			Version: s.Version, Host: personOrNull(s.Host),
		}
	}
	return out
}
