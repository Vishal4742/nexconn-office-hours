package main

import (
	"fmt"
	"log"
	"math"
	"net/http"

	"officehours/nexconn"
	"officehours/queue"
)

// What a member does with hands: raise one, take it back, add a "+1 me too" to someone else's,
// and say "I'm here" when it is their turn. Every route is onSession or onHand with asMember, one
// queue.Action through queueStore.Apply with At: now(), a refusal through queueRefusal, and
// finish. Whose hand it is and what state it is in are the rules' questions (queue.Step): these
// routes ask neither.

// The limits of a question (FR-HAND-1), in characters after cleaning. handProblem has the
// sentence for each.
const (
	handTitleMin   = 10
	handTitleMax   = 120
	handDetailsMax = 2000
)

// handInput is what the raise-hand sheet sends.
type handInput struct {
	Title   string `json:"title"`
	Details string `json:"details"`
}

// handProblem says what is wrong with a question that was cleaned, or "" when it may be raised.
func handProblem(title, details string) string {
	switch {
	case lengthBetween(title, 0, handTitleMin-1):
		return "A question needs at least 10 characters."
	case !lengthBetween(title, handTitleMin, handTitleMax):
		return "A question can have at most 120 characters. Put the rest in the details."
	case !lengthBetween(details, 0, handDetailsMax):
		return "The details can have at most 2,000 characters."
	}
	return ""
}

// raiseHand is POST /api/sessions/{id}/hands {title, details}: a member raises a hand
// (FR-HAND-1, FR-HAND-2, FR-HAND-6). The title is one line of 10 to 120 characters, the details
// keep their line breaks and are at most 2,000; both are plain text. It is a queue.Raise; the new
// hand, with its id, is in Applied.After. The room gets handRaised (the author's login is in
// their session) and the ping (FR-HAND-5).
func (d deps) raiseHand(w http.ResponseWriter, r *http.Request) {
	var in handInput
	t, ok := d.onSession(w, r, &in, asMember)
	if !ok {
		return
	}
	// A hand's title is posted to the room by the bot, so a muted member can't raise one (FR-MOD-1).
	mutedUntil, err := d.projects.MutedUntil(r.Context(), t.ProjectID, t.User.ID)
	if err != nil {
		writeError(w, err)
		return
	}
	if left := mutedUntil.Sub(now()); left > 0 {
		writeJSON(w, http.StatusForbidden, envelope{Error: fmt.Sprintf("A maintainer muted you in this room. You can raise a hand again in %d min.", int(math.Ceil(left.Minutes())))})
		return
	}
	title, details := cleanLine(in.Title), cleanText(in.Details)
	if problem := handProblem(title, details); problem != "" {
		writeJSON(w, http.StatusBadRequest, envelope{Error: problem})
		return
	}
	ap, err := d.queue.Apply(r.Context(), t.SessionID, queue.Action{Kind: queue.Raise, ActorID: t.User.ID, At: now(), Title: title, Details: details})
	if err != nil {
		writeError(w, queueRefusal(err))
		return
	}
	// The hand is written by now. If the store's answer does not name it there is nothing to put
	// on a card, and failing the request would only make its author raise it twice: the room
	// gets the ping, and the queue shows the hand.
	var cards []nexconn.Message
	if hand, ok := raisedHand(ap); ok {
		cards = append(cards, handRaised(ap.SessionID, hand, t.User.Login))
	} else {
		log.Printf("raise hand: the store named no new hand in session %d, so the room got no card", ap.SessionID)
	}
	d.finish(w, r, t, ap, cards...)
}

// handAct is a member's action on one hand that needs nothing but the hand's id: the body is {},
// and the actor is whoever the session says.
func (d deps) handAct(w http.ResponseWriter, r *http.Request, kind queue.Kind) {
	t, ok := d.onHand(w, r, nil, asMember)
	if !ok {
		return
	}
	ap, err := d.queue.Apply(r.Context(), t.SessionID, queue.Action{Kind: kind, ActorID: t.User.ID, HandID: t.HandID, At: now()})
	if err != nil {
		writeError(w, queueRefusal(err))
		return
	}
	d.finish(w, r, t, ap)
}

// withdrawHand is POST /api/hands/{id}/withdraw: the author takes a waiting hand back
// (FR-HAND-4). A queue.Withdraw; the rules refuse anyone else.
func (d deps) withdrawHand(w http.ResponseWriter, r *http.Request) { d.handAct(w, r, queue.Withdraw) }

// plusHand is POST /api/hands/{id}/plus: a member adds their "+1 me too" to someone else's
// waiting hand, or takes it back (FR-HAND-7). A queue.Plus. It never changes the order.
func (d deps) plusHand(w http.ResponseWriter, r *http.Request) { d.handAct(w, r, queue.Plus) }

// imHere is POST /api/hands/{id}/here: the author of the hand being answered says "I'm here"
// (FR-QUEUE-9). A queue.Here.
func (d deps) imHere(w http.ResponseWriter, r *http.Request) { d.handAct(w, r, queue.Here) }
