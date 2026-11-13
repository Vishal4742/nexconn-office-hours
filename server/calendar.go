package main

import (
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// The project's sessions as a calendar (FR-SESS-5): a feed a calendar app subscribes to once, or
// a file it imports. It holds the session that is live now and the next one planned, so a moved
// session moves in everyone's calendar at the next refresh.

// icsDefaultMinutes is the length of a session drawn without a planned length.
const icsDefaultMinutes = 45

// icsEvent is one session in the feed.
type icsEvent struct {
	UID     string // stable for the same session, so an app updates it instead of adding a second
	Title   string // "" for the plain "Office hours: owner/repo"
	Start   time.Time
	Minutes int
}

// calendarFeed is GET /p/{owner}/{repo}/calendar.ics. Public, like the join page: it says only
// when the room is open, never who is in it.
func (d deps) calendarFeed(w http.ResponseWriter, r *http.Request) {
	owner, repo := r.PathValue("owner"), r.PathValue("repo")
	if !validOwnerRepo(owner, repo) {
		http.NotFound(w, r)
		return
	}
	if d.projects == nil || d.queue == nil {
		http.Error(w, "The calendar isn't set up on this server yet.", http.StatusServiceUnavailable)
		return
	}
	project, found, err := d.projects.ProjectByRepo(r.Context(), owner, repo, "")
	if err != nil {
		log.Printf("calendar %s/%s: %v", owner, repo, err)
		http.Error(w, "Something went wrong on our side. Try again in a moment.", http.StatusInternalServerError)
		return
	}
	if !found {
		http.NotFound(w, r)
		return
	}
	snap, err := d.queue.Queue(r.Context(), project.ID)
	if err != nil {
		log.Printf("calendar %s/%s: %v", owner, repo, err)
		http.Error(w, "Something went wrong on our side. Try again in a moment.", http.StatusInternalServerError)
		return
	}
	var events []icsEvent
	if s := snap.Session; s != nil && s.Live {
		events = append(events, icsEvent{UID: fmt.Sprintf("session-%d", s.ID), Title: s.Title, Start: s.StartedAt, Minutes: s.PlannedMinutes})
	}
	if !snap.NextSessionAt.IsZero() {
		events = append(events, icsEvent{UID: fmt.Sprintf("next-%s-%s-%d", strings.ToLower(project.Owner), strings.ToLower(project.Repo), snap.NextSessionAt.Unix()), Start: snap.NextSessionAt})
	}
	w.Header().Set("Content-Type", "text/calendar; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`inline; filename="%s-office-hours.ics"`, strings.ToLower(project.Repo)))
	w.Header().Set("Cache-Control", "public, no-cache") // an app must see a moved session
	if _, err := fmt.Fprint(w, icsCalendar(d.guard.Origin(), project.Owner, project.Repo, events, now())); err != nil {
		log.Printf("calendar %s/%s: write: %v", owner, repo, err)
	}
}

// icsCalendar writes an iCalendar (RFC 5545) with the given events. The names are the project's
// registered spelling and the address is our own origin, never anything from the request.
func icsCalendar(origin, owner, repo string, events []icsEvent, stamp time.Time) string {
	name := owner + "/" + repo
	room := origin + "/p/" + url.PathEscape(owner) + "/" + url.PathEscape(repo)
	host := strings.TrimPrefix(strings.TrimPrefix(origin, "https://"), "http://")
	var b strings.Builder
	line := func(s string) { b.WriteString(icsFold(s)) }
	line("BEGIN:VCALENDAR")
	line("VERSION:2.0")
	line("PRODID:-//Office Hours//EN")
	line("CALSCALE:GREGORIAN")
	line("METHOD:PUBLISH")
	line("X-WR-CALNAME:" + icsText("Office hours · "+name))
	line("REFRESH-INTERVAL;VALUE=DURATION:PT1H")
	line("X-PUBLISHED-TTL:PT1H")
	for _, e := range events {
		minutes := e.Minutes
		if minutes <= 0 {
			minutes = icsDefaultMinutes
		}
		summary := "Office hours: " + name
		if e.Title != "" {
			summary = e.Title + " · " + name
		}
		line("BEGIN:VEVENT")
		line("UID:" + e.UID + "@" + host)
		line("DTSTAMP:" + icsTime(stamp))
		line("DTSTART:" + icsTime(e.Start))
		line(fmt.Sprintf("DURATION:PT%dM", minutes))
		line("SUMMARY:" + icsText(summary))
		line("DESCRIPTION:" + icsText("Live office hours with the maintainers of "+name+". Raise your hand in the room: "+room))
		line("LOCATION:" + icsText(room))
		line("URL:" + room)
		line("END:VEVENT")
	}
	line("END:VCALENDAR")
	return b.String()
}

// icsTime is a moment in UTC as iCalendar writes it: 20261119T113000Z.
func icsTime(t time.Time) string { return t.UTC().Format("20060102T150405Z") }

// icsText escapes text for an iCalendar value: a backslash, a semicolon, a comma or a line break
// would otherwise end the value or split it.
func icsText(s string) string {
	return strings.NewReplacer(`\`, `\\`, ";", `\;`, ",", `\,`, "\r\n", `\n`, "\n", `\n`).Replace(s)
}

// icsFold ends a content line with CRLF and folds it at 75 octets, never inside a character: a
// fold is CRLF followed by one space, which a reader removes again.
func icsFold(s string) string {
	var b strings.Builder
	width := 0
	for _, r := range s {
		n := len(string(r))
		if width+n > 75 {
			b.WriteString("\r\n ")
			width = 1
		}
		b.WriteRune(r)
		width += n
	}
	b.WriteString("\r\n")
	return b.String()
}
