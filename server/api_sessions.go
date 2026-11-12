package main

import "net/http"

// The routes that start, end and plan sessions. Owned by the sessions part of the parallel build
// (notes/phase-2-plan.md, worker 2); the comments below are what each route must do.

// startSession is POST /api/p/{owner}/{repo}/sessions {title, plannedMinutes}: a maintainer
// starts a live session (FR-SESS-1..3, FR-SESS-7). The title is 5 to 80 characters after
// trimming; plannedMinutes is 0 (not planned) or 15 to 180.
//
// Before the session exists, GitHub is asked again whether the caller still has admin or
// maintain rights on this very repository (the same check as registering, with the token in
// their session; ARCHITECTURE §7: a maintainer's rights were checked only once until now). Only
// a clear "no" takes the role away: GitHub answered for the repository with the project's own
// GitHub id and reported neither right. Then the start is refused and the caller is no longer a
// maintainer here (sessionStore.RemoveMaintainer). Anything else that is not a "yes" (GitHub
// did not answer, the sign-in is out of date, the repository is hidden or the name now belongs
// to another repository) refuses the start and removes nothing.
//
// Then sessionStore.StartSession, which also brings the carried hands in; store.ErrSessionLive
// is a 409 through queueRefusal. The room gets sessionStarted and a queuePing, and the answer is
// the queue (answerQueue).
func (d deps) startSession(w http.ResponseWriter, r *http.Request) { notBuilt(w) }

// setNextSession is POST /api/p/{owner}/{repo}/next-session {at}: a maintainer says when the
// next session is planned (FR-SESS-5). at is an RFC 3339 time in the future, or "" to clear it.
// The answer is the queue.
func (d deps) setNextSession(w http.ResponseWriter, r *http.Request) { notBuilt(w) }

// endSession is POST /api/sessions/{id}/end {carry}: a maintainer ends the session (FR-SESS-4).
// It is a queue.End through queueStore.Apply: waiting hands are carried to the next session, or
// not reached when carry is false; a missing carry means true. The room gets sessionEnded and the
// ping. The card's title and counts come from queueStore.Queue after the Apply: the session's
// title, and how many hands of Closed are answered, carried and not reached.
func (d deps) endSession(w http.ResponseWriter, r *http.Request) { notBuilt(w) }

// lastCall is POST /api/sessions/{id}/lastcall {closed}: a maintainer closes new hands while the
// session stays live, or opens them again (FR-SESS-6). It is a queue.LastCall through Apply. The
// room is told in words how many are left (botSays), next to the ping.
func (d deps) lastCall(w http.ResponseWriter, r *http.Request) { notBuilt(w) }
