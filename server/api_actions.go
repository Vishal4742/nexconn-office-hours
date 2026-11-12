package main

import "net/http"

// What a maintainer does with the queue. Owned by the maintainer-actions part of the parallel
// build (notes/phase-2-plan.md, worker 4); the comments below are what each route must do. Every
// queue action is onSession or onHand with asMaintainer, one queue.Action through
// queueStore.Apply with At: now(), a refusal through queueRefusal, and finish.

// nextHand is POST /api/sessions/{id}/next: the maintainer takes the first waiting hand
// (FR-QUEUE-3). A queue.Next; the rules refuse it while another hand is being answered.
func (d deps) nextHand(w http.ResponseWriter, r *http.Request) { notBuilt(w) }

// skipHand is POST /api/hands/{id}/skip: a waiting hand, or the one being answered, is skipped
// (FR-QUEUE-3). A queue.Skip.
func (d deps) skipHand(w http.ResponseWriter, r *http.Request) { notBuilt(w) }

// removeHand is POST /api/hands/{id}/remove: a maintainer takes a hand out of the queue
// (FR-MOD-1). A queue.Remove.
func (d deps) removeHand(w http.ResponseWriter, r *http.Request) { notBuilt(w) }

// awayHand is POST /api/hands/{id}/away: the author of the hand being answered is not there, so
// it goes back two places, to the end on a second miss, and is skipped on a third (FR-QUEUE-8).
// A queue.Away. The room is told why in words (botSays, with the author's @login), next to the
// ping: nobody is moved silently.
func (d deps) awayHand(w http.ResponseWriter, r *http.Request) { notBuilt(w) }

// mergeHand is POST /api/hands/{id}/merge: a waiting hand that asks the same thing joins the
// hand being answered, with its author and its +1s (FR-HAND-8). A queue.Merge.
func (d deps) mergeHand(w http.ResponseWriter, r *http.Request) { notBuilt(w) }

// doneHand is POST /api/hands/{id}/done {answer, links}: the hand being answered is finished and
// its answer becomes a FAQ entry in the same transaction (FR-FAQ-1, FR-FAQ-2). The answer is 20
// to 2,000 characters after trimming; links are at most 5 http or https addresses of at most 500
// characters each. The draft needs the hand's own words, so the hand is read first: it is the one
// in queueStore.Queue(...).Open with this id, and it must be the one being answered. The draft's
// slug is made from its title (lower case, digits and hyphens, at most 72 characters, never
// empty); the store makes it unique in the project and keeps it within the 80 of slugRe. Then
// queueStore.Done with a queue.Done. The room gets faqAdded, with the logins of everyone who
// asked (the author, then the +1s as Applied.Before has them: queueStore.People), and the ping.
func (d deps) doneHand(w http.ResponseWriter, r *http.Request) { notBuilt(w) }

// muteMember is POST /api/p/{owner}/{repo}/mute {userId, minutes}: a maintainer stops a member
// from posting in the room for a while (FR-MOD-1). minutes is 1 to 1,440, and 10 when it is 0.
// The person must be a member of this project and not a maintainer of it: ask
// d.projects.ProjectByRepo for the project as that person sees it. chatService.Mute does the
// muting. The answer is {"userId", "minutes"}.
func (d deps) muteMember(w http.ResponseWriter, r *http.Request) { notBuilt(w) }
