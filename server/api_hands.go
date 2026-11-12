package main

import "net/http"

// What a member does with hands. Owned by the hands part of the parallel build
// (notes/phase-2-plan.md, worker 3); the comments below are what each route must do. Every one
// of them is onSession or onHand with asMember, one queue.Action through queueStore.Apply with
// At: now(), a refusal through queueRefusal, and finish.

// raiseHand is POST /api/sessions/{id}/hands {title, details}: a member raises a hand
// (FR-HAND-1, FR-HAND-2, FR-HAND-6). The title is 10 to 120 characters after trimming, the
// details at most 2,000; both are plain text. It is a queue.Raise; the new hand, with its id, is
// in Applied.After. The room gets handRaised (the author's login is in their session) and the
// ping (FR-HAND-5).
func (d deps) raiseHand(w http.ResponseWriter, r *http.Request) { notBuilt(w) }

// withdrawHand is POST /api/hands/{id}/withdraw: the author takes a waiting hand back
// (FR-HAND-4). A queue.Withdraw; the rules refuse anyone else.
func (d deps) withdrawHand(w http.ResponseWriter, r *http.Request) { notBuilt(w) }

// plusHand is POST /api/hands/{id}/plus: a member adds their "+1 me too" to someone else's
// waiting hand, or takes it back (FR-HAND-7). A queue.Plus. It never changes the order.
func (d deps) plusHand(w http.ResponseWriter, r *http.Request) { notBuilt(w) }

// imHere is POST /api/hands/{id}/here: the author of the hand being answered says "I'm here"
// (FR-QUEUE-9). A queue.Here.
func (d deps) imHere(w http.ResponseWriter, r *http.Request) { notBuilt(w) }
