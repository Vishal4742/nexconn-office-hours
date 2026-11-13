package main

import (
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"officehours/store"
)

func TestICSCalendar(t *testing.T) {
	stamp := time.Date(2026, 11, 13, 6, 0, 0, 0, time.UTC)
	next := time.Date(2026, 11, 19, 11, 30, 0, 0, time.UTC)
	cal := icsCalendar(testSite, "PaperPlane", "Stash", []icsEvent{
		{UID: "next-1795606200", Start: next, Minutes: 45},
	}, stamp)

	t.Run("FR-SESS-5: one event for the next session, in UTC, with the room as its place", func(t *testing.T) {
		for _, want := range []string{
			"BEGIN:VCALENDAR\r\n", "VERSION:2.0\r\n", "PRODID:-//Office Hours//EN\r\n",
			"X-WR-CALNAME:Office hours · PaperPlane/Stash\r\n",
			"BEGIN:VEVENT\r\n", "UID:next-1795606200@officehours.example\r\n",
			"DTSTAMP:20261113T060000Z\r\n", "DTSTART:20261119T113000Z\r\n", "DURATION:PT45M\r\n",
			"SUMMARY:Office hours: PaperPlane/Stash\r\n",
			"URL:https://officehours.example/p/PaperPlane/Stash\r\n",
			"END:VEVENT\r\n", "END:VCALENDAR\r\n",
		} {
			if !strings.Contains(cal, want) {
				t.Errorf("the calendar has no %q:\n%s", want, cal)
			}
		}
		if strings.Contains(strings.ReplaceAll(cal, "\r\n", ""), "\n") {
			t.Error("RFC 5545: every line ends with CRLF")
		}
	})

	t.Run("RFC 5545: no line is longer than 75 octets, and a folded line reads back whole", func(t *testing.T) {
		long := icsCalendar(testSite, "PaperPlane", strings.Repeat("r", 100), []icsEvent{{UID: "x", Start: next, Minutes: 45}}, stamp)
		for _, line := range strings.Split(strings.TrimSuffix(long, "\r\n"), "\r\n") {
			if len(line) > 75 {
				t.Errorf("a line of %d octets: %q", len(line), line)
			}
		}
		unfolded := strings.ReplaceAll(long, "\r\n ", "")
		if !strings.Contains(unfolded, "SUMMARY:Office hours: PaperPlane/"+strings.Repeat("r", 100)+"\r\n") {
			t.Error("folding lost or changed text")
		}
	})

	t.Run("text is escaped: a comma, a semicolon or a backslash can't break the line", func(t *testing.T) {
		if got := icsText(`a,b;c\d` + "\ne"); got != `a\,b\;c\\d\ne` {
			t.Errorf("icsText = %q", got)
		}
	})

	t.Run("no session planned: an empty calendar, still a valid one", func(t *testing.T) {
		empty := icsCalendar(testSite, "PaperPlane", "Stash", nil, stamp)
		if strings.Contains(empty, "BEGIN:VEVENT") || !strings.HasSuffix(empty, "END:VCALENDAR\r\n") {
			t.Errorf("empty calendar:\n%s", empty)
		}
	})
}

func TestCalendarFeed(t *testing.T) {
	const path = "/p/PaperPlane/Stash/calendar.ics"
	next := now().Add(48 * time.Hour)

	t.Run("FR-SESS-5: the live session and the next one, as text/calendar, for anyone", func(t *testing.T) {
		a := newQueueApp(t)
		a.queue.snapshot = store.Snapshot{
			Session:       &store.Session{ID: liveSession, Title: "Thursday office hours", Live: true, PlannedMinutes: 30, StartedAt: now().Add(-10 * time.Minute)},
			NextSessionAt: next,
		}
		res, body := call(a.mux, "GET", path, "", nil)
		if res.StatusCode != http.StatusOK || !strings.HasPrefix(res.Header.Get("Content-Type"), "text/calendar") {
			t.Fatalf("status %d, type %q", res.StatusCode, res.Header.Get("Content-Type"))
		}
		if strings.Count(body, "BEGIN:VEVENT") != 2 || !strings.Contains(body, "DURATION:PT30M") || !strings.Contains(body, "SUMMARY:Thursday office hours · PaperPlane/Stash") {
			t.Errorf("body:\n%s", body)
		}
		if res.Header.Get("Cache-Control") != "public, no-cache" {
			t.Errorf("Cache-Control %q: a calendar app must see a moved session", res.Header.Get("Cache-Control"))
		}
	})

	t.Run("an ended session is not an event, and a passed plan is none", func(t *testing.T) {
		a := newQueueApp(t)
		a.queue.snapshot = store.Snapshot{Session: &store.Session{ID: liveSession, Live: false, StartedAt: now().Add(-2 * time.Hour)}}
		_, body := call(a.mux, "GET", path, "", nil)
		if strings.Contains(body, "BEGIN:VEVENT") {
			t.Errorf("events for nothing to come:\n%s", body)
		}
	})

	t.Run("unknown or malformed projects are 404, a store that fails a 500", func(t *testing.T) {
		a := newQueueApp(t)
		if res, _ := call(a.mux, "GET", "/p/nobody/nothing/calendar.ics", "", nil); res.StatusCode != http.StatusNotFound {
			t.Errorf("unknown project: %d", res.StatusCode)
		}
		if res, _ := call(a.mux, "GET", "/p/bad_owner/x/calendar.ics", "", nil); res.StatusCode != http.StatusNotFound {
			t.Errorf("bad owner: %d", res.StatusCode)
		}
		a.queue.err = errors.New("connection reset")
		if res, _ := call(a.mux, "GET", path, "", nil); res.StatusCode != http.StatusInternalServerError {
			t.Errorf("store down: %d", res.StatusCode)
		}
	})
}
