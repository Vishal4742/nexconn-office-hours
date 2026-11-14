package main

import (
	"cmp"
	"context"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode"

	"officehours/guard"
	"officehours/nexconn"
	"officehours/queue"
	"officehours/store"
)

// What a maintainer does with the queue. Owned by the maintainer-actions part of the parallel
// build (notes/phase-2-plan.md, worker 4). Every queue action is onSession or onHand with
// asMaintainer, one queue.Action with At: now() through queueStore.Apply (actionApply), or
// through queueStore.Done for a Done, a refusal through queueRefusal, and finish. Mute is no
// queue action: it goes to the chat.

// nextHand is POST /api/sessions/{id}/next: the maintainer takes the first waiting hand
// (FR-QUEUE-3). A queue.Next; the rules refuse it while another hand is being answered.
func (d deps) nextHand(w http.ResponseWriter, r *http.Request) {
	if t, ap, ok := d.actionApply(w, r, d.onSession, queue.Next); ok {
		d.finish(w, r, t, ap)
	}
}

// skipHand is POST /api/hands/{id}/skip: a waiting hand, or the one being answered, is skipped
// (FR-QUEUE-3). A queue.Skip.
func (d deps) skipHand(w http.ResponseWriter, r *http.Request) {
	if t, ap, ok := d.actionApply(w, r, d.onHand, queue.Skip); ok {
		d.finish(w, r, t, ap)
	}
}

// removeHand is POST /api/hands/{id}/remove: a maintainer takes a hand out of the queue
// (FR-MOD-1). A queue.Remove.
func (d deps) removeHand(w http.ResponseWriter, r *http.Request) {
	if t, ap, ok := d.actionApply(w, r, d.onHand, queue.Remove); ok {
		d.finish(w, r, t, ap)
	}
}

// mergeHand is POST /api/hands/{id}/merge: a waiting hand that asks the same thing joins the
// hand being answered, with its author and its +1s (FR-HAND-8). A queue.Merge.
func (d deps) mergeHand(w http.ResponseWriter, r *http.Request) {
	if t, ap, ok := d.actionApply(w, r, d.onHand, queue.Merge); ok {
		d.finish(w, r, t, ap)
	}
}

// awayHand is POST /api/hands/{id}/away: the author of the hand being answered is not there, so
// it goes back two places, to the end on a second miss, and is skipped on a third (FR-QUEUE-8).
// A queue.Away; the rules refuse it once the author said "I'm here" (FR-QUEUE-9). The room is
// told why in words, next to the ping: nobody is moved silently.
func (d deps) awayHand(w http.ResponseWriter, r *http.Request) {
	t, ap, ok := d.actionApply(w, r, d.onHand, queue.Away)
	if !ok {
		return
	}
	var said []nexconn.Message
	if line, ok := d.actionAwayLine(r.Context(), ap, t.HandID); ok {
		said = append(said, botSays(line))
	}
	d.finish(w, r, t, ap, said...)
}

// actionApply checks the caller with start (onSession or onHand) as a maintainer and applies one
// action of this kind. When it answers false it has written the refusal itself.
func (d deps) actionApply(w http.ResponseWriter, r *http.Request,
	start func(http.ResponseWriter, *http.Request, any, role) (target, bool), kind queue.Kind,
) (target, store.Applied, bool) {
	t, ok := start(w, r, nil, asMaintainer)
	if !ok {
		return target{}, store.Applied{}, false
	}
	ap, err := d.queue.Apply(r.Context(), t.SessionID, queue.Action{Kind: kind, ActorID: t.User.ID, HandID: t.HandID, At: now()})
	if err != nil {
		writeError(w, queueRefusal(err))
		return target{}, store.Applied{}, false
	}
	return t, ap, true
}

// actionAwayLine is what the room is told after an Away: who missed their turn, as @login, and
// where their question went. It answers false when the store's answer does not hold the hand.
func (d deps) actionAwayLine(ctx context.Context, ap store.Applied, handID int64) (string, bool) {
	h, ok := handIn(ap.After, handID)
	if !ok {
		return "", false
	}
	who := "Someone"
	if logins := d.actionLogins(ctx, []string{h.AuthorID}); len(logins) == 1 {
		who = "@" + logins[0]
	}
	waiting := 0 // the others in line when Away was pressed: the hand itself was being answered
	for _, other := range ap.Before.Hands {
		if other.Status == queue.Waiting {
			waiting++
		}
	}
	switch {
	case h.Status == queue.Skipped:
		return who + " missed their turn three times, so their question was skipped. They can raise it again.", true
	// ponytail: the place is told from how many wait, not read from the new order. When no
	// microsecond fits between the second and the third (plan §4) the rules send the hand to the
	// end and the room still hears "two places". Read it from After's order if that ever shows.
	case h.AwayCount == 1 && waiting >= 2:
		return who + " didn't answer when their turn came, so their question moved back two places.", true
	}
	return who + " didn't answer when their turn came, so their question moved to the end of the line.", true
}

// actionLogins looks up the logins of people the room is told about, in the order asked and
// without the ones the store does not know. The action is written by now, so the lookup outlives
// a caller who went away, like tell (plan decision 6), and a lookup that fails leaves the names
// out of the message, not the message out of the room.
func (d deps) actionLogins(ctx context.Context, userIDs []string) []string {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), tellTimeout)
	defer cancel()
	people, err := d.queue.People(ctx, userIDs)
	if err != nil {
		log.Printf("names for the room: %v", err)
		return nil
	}
	logins := make([]string, 0, len(people))
	for _, p := range people {
		logins = append(logins, p.Login)
	}
	return logins
}

// doneInput is what the Done sheet sends.
type doneInput struct {
	Answer string   `json:"answer"`
	Links  []string `json:"links"`
}

// The limits of an answer (FR-FAQ-1), in characters.
const (
	doneAnswerMin = 20
	doneAnswerMax = 2000
	doneLinksMax  = 5
	doneLinkMax   = 500
)

// doneHand is POST /api/hands/{id}/done {answer, links}: the hand being answered is finished and
// its answer becomes a FAQ entry in the same transaction (FR-FAQ-1, FR-FAQ-2). The draft needs
// the hand's own words, so the hand is read first: it is the one in queueStore.Queue(...).Open
// with this id, and it must be the one being answered. Its slug comes from its title (slugOf);
// the store makes it unique in the project. The room gets faqAdded, with the logins of everyone
// who asked (the author, then the +1s as Applied.Before has them), and the ping.
func (d deps) doneHand(w http.ResponseWriter, r *http.Request) {
	var in doneInput
	t, ok := d.onHand(w, r, &in, asMaintainer)
	if !ok {
		return
	}
	answer, links, err := doneCheck(in)
	if err != nil {
		writeError(w, err)
		return
	}
	hand, err := d.doneAnswering(r.Context(), t)
	if err != nil {
		writeError(w, queueRefusal(err))
		return
	}
	draft := store.FAQDraft{Slug: slugOf(hand.Title), Title: hand.Title, Question: hand.Details, Answer: answer, Links: links, AuthorID: t.User.ID}
	ap, entry, err := d.queue.Done(r.Context(), t.SessionID, queue.Action{Kind: queue.Done, ActorID: t.User.ID, HandID: t.HandID, At: now()}, draft)
	if err != nil {
		writeError(w, queueRefusal(err))
		return
	}
	var askedBy []string
	if h, ok := handIn(ap.Before, t.HandID); ok {
		askedBy = d.actionLogins(r.Context(), append([]string{h.AuthorID}, h.Plus...))
	}
	d.finish(w, r, t, ap, faqAdded(entry, askedBy))
}

// doneAnswering is the hand a Done is about, read from the project's queue: it must be the one
// being answered. The rules check that again inside the transaction.
func (d deps) doneAnswering(ctx context.Context, t target) (queue.Hand, error) {
	snap, err := d.queue.Queue(ctx, t.ProjectID)
	if err != nil {
		return queue.Hand{}, err
	}
	for _, h := range snap.Open {
		if h.ID == t.HandID && h.Status == queue.Answering {
			return h.Hand, nil
		}
	}
	return queue.Hand{}, queue.ErrNotAnswering
}

// doneCheck cleans what the Done sheet sent and checks it (FR-FAQ-1, NFR-SEC-3): the answer is
// trimmed and 20 to 2,000 characters; a link field left empty is no link; the others are at most
// 5 web addresses (doneLink), none given twice. "Link 3" in a refusal is the third field as sent,
// empty ones counted, so the sheet can point at it. A link is kept the way net/url writes it, so
// "HTTPS://" is "https://" and a repeat is found however it was typed.
func doneCheck(in doneInput) (string, []string, error) {
	answer := cleanText(in.Answer)
	if !lengthBetween(answer, doneAnswerMin, doneAnswerMax) {
		return "", nil, doneRefusal("The answer needs 20 to 2,000 characters.")
	}
	links := []string{}
	seen := map[string]bool{}
	for i, typed := range in.Links {
		typed = strings.TrimSpace(typed)
		if typed == "" {
			continue
		}
		link, err := doneLink(i+1, typed)
		if err != nil {
			return "", nil, err
		}
		if seen[link] {
			return "", nil, doneRefusal("Link %d repeats an earlier one.", i+1)
		}
		seen[link] = true
		links = append(links, link)
	}
	if len(links) > doneLinksMax {
		return "", nil, doneRefusal("Add at most 5 links.")
	}
	return answer, links, nil
}

// doneLink checks the nth link of an answer: an absolute http or https address with a host, at
// most 500 characters, with no white space inside (two addresses in one field, or a broken paste)
// and no name or password before its host. It answers the link as net/url writes it.
func doneLink(n int, typed string) (string, error) {
	if !lengthBetween(typed, 1, doneLinkMax) {
		return "", doneRefusal("Link %d is longer than 500 characters.", n)
	}
	u, err := url.Parse(typed)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" ||
		u.User != nil || strings.ContainsFunc(typed, unicode.IsSpace) {
		return "", doneRefusal("Link %d isn't a web address. Paste the address of a page, starting with https://.", n)
	}
	return u.String(), nil
}

// doneRefusal is a refusal of what the Done sheet sent, in a sentence it shows as it is.
func doneRefusal(format string, args ...any) error {
	return &guard.Error{Status: http.StatusBadRequest, Msg: fmt.Sprintf(format, args...)}
}

// muteInput is what a maintainer sends to mute someone, and what the route answers.
type muteInput struct {
	UserID  string `json:"userId"`
	Minutes int    `json:"minutes"`
}

// How long a mute lasts, in minutes.
const (
	muteDefault = 10
	muteMax     = 1440 // a day
)

// muteMember is POST /api/p/{owner}/{repo}/mute {userId, minutes}: a maintainer stops a member
// from posting in the room for a while (FR-MOD-1). minutes is 1 to 1,440, and 10 when it is 0.
// The person must be a member of this project and not a maintainer of it: the project as that
// person sees it says which. chatService.Mute does the muting. The answer is {"userId", "minutes"}.
func (d deps) muteMember(w http.ResponseWriter, r *http.Request) {
	var in muteInput
	_, p, ok := d.onProject(w, r, &in, asMaintainer)
	if !ok {
		return
	}
	if d.chat == nil {
		writeJSON(w, http.StatusServiceUnavailable, envelope{Error: "The chat isn't set up on this server yet."})
		return
	}
	minutes := cmp.Or(in.Minutes, muteDefault)
	switch {
	case !muteUserID(in.UserID):
		writeJSON(w, http.StatusBadRequest, envelope{Error: "That is not a user id."})
		return
	case minutes < 1 || minutes > muteMax:
		writeJSON(w, http.StatusBadRequest, envelope{Error: "Mute someone for 1 to 1,440 minutes."})
		return
	}
	theirs, found, err := d.projects.ProjectByRepo(r.Context(), p.Owner, p.Repo, in.UserID)
	switch {
	case err != nil:
		writeError(w, err)
		return
	case !found || !theirs.IsMember:
		writeJSON(w, http.StatusNotFound, envelope{Error: "That person is not in this room."})
		return
	case theirs.IsMaintainer:
		writeJSON(w, http.StatusForbidden, envelope{Error: "A maintainer can't be muted."})
		return
	}
	// The chat can't stop the bot from posting their raised hand, so the server keeps the mute too
	// (AT-18), and first: a hand raised while the chat mutes them is already refused.
	if err := d.projects.Mute(r.Context(), p.ID, in.UserID, now().Add(time.Duration(minutes)*time.Minute)); err != nil {
		writeError(w, err)
		return
	}
	if err := d.chat.Mute(r.Context(), groupID(p.ID), in.UserID, int32(minutes)); err != nil {
		log.Printf("mute %s in %s/%s: %v", in.UserID, p.Owner, p.Repo, err)
		writeJSON(w, http.StatusBadGateway, envelope{Error: "We couldn't mute them in the chat. Try again in a moment."})
		return
	}
	writeJSON(w, http.StatusOK, envelope{OK: true, Data: muteInput{UserID: in.UserID, Minutes: minutes}})
}

// muteUserID reports whether id names a person: "gh" and the number of their GitHub account.
func muteUserID(id string) bool {
	number, ok := strings.CutPrefix(id, "gh")
	_, err := strconv.ParseUint(number, 10, 63)
	return ok && err == nil
}
