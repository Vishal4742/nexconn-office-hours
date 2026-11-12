package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"time"

	"officehours/github"
	"officehours/guard"
	"officehours/nexconn"
	"officehours/queue"
	"officehours/session"
	"officehours/store"
)

// The routes that start, end and plan sessions (FR-SESS-1..7).

// What a session may be (FR-SESS-1).
const (
	sessionTitleMin   = 5
	sessionTitleMax   = 80
	sessionMinutesMin = 15
	sessionMinutesMax = 180

	sessionsNotSetUp = "Sessions aren't set up on this server yet."
)

// startSession is POST /api/p/{owner}/{repo}/sessions {title, plannedMinutes}: a maintainer
// starts a live session (FR-SESS-1..3, FR-SESS-7). The title is 5 to 80 characters after
// trimming; plannedMinutes is 0 (not planned) or 15 to 180.
//
// Before the session exists, GitHub is asked again whether the caller still maintains this very
// repository (sessionStillMaintained). Then sessionStore.StartSession, which also brings the
// carried hands in; a session that is live already is a 409, decided by the store's unique index.
// The room gets the session card, which mentions everyone, and the ping; the answer is the queue.
func (d deps) startSession(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Title          string `json:"title"`
		PlannedMinutes int    `json:"plannedMinutes"`
	}
	u, project, ok := d.onProject(w, r, &in, asMaintainer)
	if !ok {
		return
	}
	title := cleanLine(in.Title)
	switch {
	case !lengthBetween(title, sessionTitleMin, sessionTitleMax):
		writeJSON(w, http.StatusBadRequest, envelope{Error: "A session's title needs 5 to 80 characters."})
		return
	case in.PlannedMinutes != 0 && (in.PlannedMinutes < sessionMinutesMin || in.PlannedMinutes > sessionMinutesMax):
		writeJSON(w, http.StatusBadRequest, envelope{Error: "A session is planned for 15 to 180 minutes, or for no fixed length."})
		return
	case d.starts == nil || d.queue == nil || d.github == nil:
		writeJSON(w, http.StatusServiceUnavailable, envelope{Error: sessionsNotSetUp})
		return
	}
	if err := d.sessionStillMaintained(r.Context(), u, project); err != nil {
		writeError(w, err)
		return
	}
	started, err := d.starts.StartSession(r.Context(), store.NewSession{ProjectID: project.ID, HostID: u.ID, Title: title, PlannedMinutes: in.PlannedMinutes})
	if err != nil {
		writeError(w, queueRefusal(err))
		return
	}
	d.tell(r.Context(), project.ID, sessionStarted(started), queuePing(started.ID, started.Version))
	d.answerQueue(w, r, project.ID, u.ID)
}

// sessionStillMaintained asks GitHub, with the token in the caller's session, whether they still
// maintain the project's repository: until now a maintainer's rights were checked only once, when
// they registered it (plan decision 7). nil means yes. Only a clear no takes the role away
// (sessionRightsRefusal); the caller stays a member of the room.
func (d deps) sessionStillMaintained(ctx context.Context, u session.User, p store.Project) error {
	repo, err := d.github.Repo(ctx, u.GitHubToken, p.Owner, p.Repo)
	refusal, lostRole := sessionRightsRefusal(p, repo, err)
	if refusal == nil {
		return nil
	}
	if lostRole {
		if err := d.starts.RemoveMaintainer(ctx, p.ID, u.ID); err != nil {
			return fmt.Errorf("start session: %w", err)
		}
		log.Printf("start session %s/%s: %s is no longer a maintainer: GitHub reports neither admin nor maintain rights", p.Owner, p.Repo, u.ID)
	}
	return refusal
}

// sessionRightsRefusal reads GitHub's answer about the caller and a project's repository. It has
// three outcomes. Yes, which is no refusal: GitHub answered for the repository the project was
// registered for, known by its id, and reports admin or maintain rights. A clear no, which is a
// refusal with lostRole: the same repository, and neither right. Anything else is a refusal that
// says nothing about the caller's rights, so it must never cost a maintainer their role: GitHub
// did not answer, the sign-in is out of date, the repository is hidden or gone, the name now
// belongs to another repository, or the reply carries no permissions.
func sessionRightsRefusal(p store.Project, repo github.Repo, err error) (refusal *guard.Error, lostRole bool) {
	// The name the room is registered under. GitHub may call the repository something else by now.
	name := p.Owner + "/" + p.Repo
	lost := "GitHub no longer shows admin or maintain rights for you on " + name + ", so you are no longer a maintainer of this room."
	refuse := func(status int, msg string) (*guard.Error, bool) {
		return &guard.Error{Status: status, Msg: msg}, false
	}
	switch {
	case errors.Is(err, github.ErrUnauthorized):
		return refuse(http.StatusUnauthorized, "GitHub no longer accepts your sign-in. Sign in again, then start the session.")
	case errors.Is(err, github.ErrForbidden):
		return refuse(http.StatusForbidden, "GitHub refused to show us "+name+", so we couldn't check that you still maintain it. If it belongs to an organization, ask one of its owners to approve Office Hours, then try again.")
	case errors.Is(err, github.ErrNotFound):
		return refuse(http.StatusConflict, "GitHub no longer shows us a public repository called "+name+". If it was made private or deleted, office hours can't be started for it.")
	case err != nil:
		log.Printf("start session %s: %v", name, err)
		return refuse(http.StatusBadGateway, githubSilent)
	case repo.ID != p.GitHubID:
		// A name can change hands. What GitHub says about the newcomer says nothing about ours.
		log.Printf("start session %s: refused, the name now belongs to another GitHub repository", name)
		return refuse(http.StatusConflict, name+" on GitHub is now a different repository from the one this room was registered for, so we couldn't check that you still maintain it.")
	case !repo.KnowsPermissions:
		log.Printf("start session %s: assumption A-7 failed: GitHub's reply has no permissions", name)
		return refuse(http.StatusBadGateway, "GitHub didn't say what rights you have on "+name+", so we can't tell that you still maintain it. Try again in a moment.")
	case !repo.CanMaintain && repo.OwnerIsOrg:
		// An organization that restricts OAuth apps makes its own maintainers look like outsiders.
		return &guard.Error{Status: http.StatusForbidden, Msg: lost + " If you still have them, ask an owner of " + p.Owner + " to approve Office Hours for the organization, then register the repo again."}, true
	case !repo.CanMaintain:
		return &guard.Error{Status: http.StatusForbidden, Msg: lost + " If you get them back, register the repo again."}, true
	case repo.Private:
		return refuse(http.StatusForbidden, name+" is private now. Office Hours only works with public repositories.") // NFR-PRIV-1
	}
	return nil, false
}

// setNextSession is POST /api/p/{owner}/{repo}/next-session {at}: a maintainer says when the
// next session is planned (FR-SESS-5). at is an RFC 3339 time in the future and less than a year
// away, or "" to clear it. The answer is the queue. Nothing is told to the room: a plan changes
// no queue, so there is no version to ping, and a page shows it the next time it reads the queue.
func (d deps) setNextSession(w http.ResponseWriter, r *http.Request) {
	var in struct {
		At *string `json:"at"`
	}
	u, project, ok := d.onProject(w, r, &in, asMaintainer)
	if !ok {
		return
	}
	at, refusal := sessionNextTime(in.At)
	if refusal != nil {
		writeError(w, refusal)
		return
	}
	if d.starts == nil || d.queue == nil {
		writeJSON(w, http.StatusServiceUnavailable, envelope{Error: sessionsNotSetUp})
		return
	}
	if err := d.starts.SetNextSession(r.Context(), project.ID, at); err != nil {
		writeError(w, err)
		return
	}
	d.answerQueue(w, r, project.ID, u.ID)
}

// sessionNextTime reads when the next session is planned: the zero time for "", which clears the
// plan, else a moment that is still to come and less than a year away. It comes back in whole
// microseconds, which is what Postgres keeps. A body that does not mention the time is refused:
// it would clear the plan by accident.
func sessionNextTime(text *string) (time.Time, *guard.Error) {
	refuse := func(msg string) (time.Time, *guard.Error) {
		return time.Time{}, &guard.Error{Status: http.StatusBadRequest, Msg: msg}
	}
	if text == nil {
		return refuse("Say when the next session is, or send an empty time to clear it.")
	}
	if *text == "" {
		return time.Time{}, nil
	}
	at, err := time.Parse(time.RFC3339, *text)
	switch from := now(); {
	case err != nil:
		return refuse("That isn't a date and time we can read. Pick the next session's time again.")
	case !at.After(from):
		return refuse("The next session has to be in the future.")
	case !at.Before(from.AddDate(1, 0, 0)):
		return refuse("The next session can be at most a year away.")
	}
	return at.UTC().Truncate(time.Microsecond), nil
}

// endSession is POST /api/sessions/{id}/end {carry}: a maintainer ends the session (FR-SESS-4).
// It is a queue.End through queueStore.Apply: waiting hands are carried to the next session, or
// not reached when carry is false; a missing carry means true (FR-SESS-7). The rules refuse it
// while a hand is being answered. The room gets the ended card and the ping.
func (d deps) endSession(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Carry *bool `json:"carry"`
	}
	t, ok := d.onSession(w, r, &in, asMaintainer)
	if !ok {
		return
	}
	carry := in.Carry == nil || *in.Carry
	ap, err := d.queue.Apply(r.Context(), t.SessionID, queue.Action{Kind: queue.End, ActorID: t.User.ID, At: now(), Carry: carry})
	if err != nil {
		writeError(w, queueRefusal(err))
		return
	}
	d.finish(w, r, t, ap, d.sessionEndCards(r.Context(), t, ap)...)
}

// sessionEndCards is the card that closes a session, as the list finish takes (FR-SESS-4,
// FR-SESS-7). How many hands were carried or not reached is what the action did; the title and
// how many were answered are read from the queue. By now the session is over whatever happens
// here, so the read outlives a caller who went away, and a queue that cannot be read, or that
// shows a session started in the meantime, costs the room the card and never the ping.
func (d deps) sessionEndCards(ctx context.Context, t target, ap store.Applied) []nexconn.Message {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), tellTimeout)
	defer cancel()
	snap, err := d.queue.Queue(ctx, t.ProjectID)
	if err != nil {
		log.Printf("end session %d: no ended card: %v", t.SessionID, err)
		return nil
	}
	if snap.Session == nil || snap.Session.ID != t.SessionID {
		log.Printf("end session %d: no ended card: the queue shows another session", t.SessionID)
		return nil
	}
	answered := 0
	for _, h := range snap.Closed {
		if h.Status == queue.Answered {
			answered++
		}
	}
	return []nexconn.Message{sessionEnded(t.SessionID, snap.Session.Title, answered,
		sessionCount(ap.After, queue.Carried), sessionCount(ap.After, queue.NotReached))}
}

// sessionCount says how many hands of a state have a status.
func sessionCount(s queue.State, status queue.Status) int {
	n := 0
	for _, h := range s.Hands {
		if h.Status == status {
			n++
		}
	}
	return n
}

// lastCall is POST /api/sessions/{id}/lastcall {closed}: a maintainer closes new hands while the
// session stays live, or opens them again (FR-SESS-6). It is a queue.LastCall through Apply. The
// room is told in words, next to the ping, and only when something changed: a second press, or
// a second maintainer's, says nothing twice. A body that does not say closed is refused: it
// would reopen hands by accident.
func (d deps) lastCall(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Closed *bool `json:"closed"`
	}
	t, ok := d.onSession(w, r, &in, asMaintainer)
	if !ok {
		return
	}
	if in.Closed == nil {
		writeJSON(w, http.StatusBadRequest, envelope{Error: "Say whether hands are closed or open."})
		return
	}
	ap, err := d.queue.Apply(r.Context(), t.SessionID, queue.Action{Kind: queue.LastCall, ActorID: t.User.ID, At: now(), Closed: *in.Closed})
	if err != nil {
		writeError(w, queueRefusal(err))
		return
	}
	var said []nexconn.Message
	if ap.After.HandsClosed != ap.Before.HandsClosed {
		said = append(said, botSays(sessionLastCallLine(ap.After)))
	}
	d.finish(w, r, t, ap, said...)
}

// sessionLastCallLine is what the room is told when hands close or open again (FR-SESS-6:
// "Hands closed, N left"). In line are the hands that wait; the one being answered has its turn.
func sessionLastCallLine(after queue.State) string {
	if !after.HandsClosed {
		return "Hands are open again: you can raise yours."
	}
	switch waiting := sessionCount(after, queue.Waiting); waiting {
	case 0:
		return "Last call: hands are closed. Nobody is left in line."
	case 1:
		return "Last call: hands are closed. 1 question is left in line."
	default:
		return fmt.Sprintf("Last call: hands are closed. %d questions are left in line.", waiting)
	}
}
