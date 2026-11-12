package main

import (
	"context"
	"errors"
	"maps"
	"net/http"
	"slices"
	"strings"
	"testing"

	"officehours/store"
)

// statsFake stands in for the store behind the stats routes. It answers with the numbers a test
// put in under a project's id ("" is the whole site) and records every id it was asked for.
type statsFake struct {
	numbers map[string]store.Stats
	asked   []string
	err     error // every call fails with it and still hands the numbers over: a route must not use them
}

func (f *statsFake) Stats(_ context.Context, projectID string) (store.Stats, error) {
	f.asked = append(f.asked, projectID)
	return f.numbers[projectID], f.err
}

// statsApp is newQueueApp with numbers behind the stats routes: those of PaperPlane/Stash and
// those of the whole site. In each set the five differ, so a field that carries another field's
// number shows.
func statsApp(t *testing.T) (*queueApp, *statsFake) {
	t.Helper()
	a := newQueueApp(t)
	fake := &statsFake{numbers: map[string]store.Stats{
		queueProject: {Sessions: 2, HandsRaised: 11, HandsAnswered: 4, FAQEntries: 3, Participants: 5},
		"":           {Sessions: 3, HandsRaised: 16, HandsAnswered: 7, FAQEntries: 5, Participants: 8},
	}}
	a.deps.stats = fake
	a.remux()
	return a, fake
}

func TestStatsRoutes(t *testing.T) {
	const ofProject, ofSite = "/api/p/paperplane/stash/stats", "/api/stats"
	// The fields web/src/stats.ts reads. Comparing whole maps also proves there is no sixth.
	wantProject := map[string]int{"sessions": 2, "handsRaised": 11, "handsAnswered": 4, "faqEntries": 3, "participants": 5}
	wantSite := map[string]int{"sessions": 3, "handsRaised": 16, "handsAnswered": 7, "faqEntries": 5, "participants": 8}

	numbers := func(t *testing.T, a *queueApp, path string, cookie *http.Cookie) map[string]int {
		t.Helper()
		res, body := call(a.mux, "GET", path, "", cookie)
		if res.StatusCode != http.StatusOK {
			t.Fatalf("GET %s: status %d body %s, want 200", path, res.StatusCode, body)
		}
		var got map[string]int
		decode(t, body, &got)
		return got
	}

	t.Run("FR-MET-1: a project's five numbers, for anyone and in any letter case", func(t *testing.T) {
		a, fake := statsApp(t)
		for name, c := range map[string]struct {
			path   string
			cookie *http.Cookie
		}{
			"signed out":                  {ofProject, nil},
			"a member":                    {ofProject, a.as(t, askerID)},
			"signed in, outside the room": {"/api/p/PaperPlane/STASH/stats", a.as(t, strangerID)},
		} {
			if got := numbers(t, a, c.path, c.cookie); !maps.Equal(got, wantProject) {
				t.Errorf("%s: %v, want exactly %v", name, got, wantProject)
			}
		}
		if want := slices.Repeat([]string{queueProject}, 3); !slices.Equal(fake.asked, want) {
			t.Errorf("the store counted for %q, want the project's own id each time: %q", fake.asked, want)
		}
	})

	t.Run("FR-MET-1: the site's numbers are those of every project together", func(t *testing.T) {
		a, fake := statsApp(t)
		for name, cookie := range map[string]*http.Cookie{"signed out": nil, "signed in": a.as(t, strangerID)} {
			if got := numbers(t, a, ofSite, cookie); !maps.Equal(got, wantSite) {
				t.Errorf("%s: %v, want exactly %v", name, got, wantSite)
			}
		}
		if want := []string{"", ""}; !slices.Equal(fake.asked, want) {
			t.Errorf("the store counted for %q, want no project each time", fake.asked)
		}
	})

	t.Run("FR-MET-1: a project nobody has used yet answers five zeros, not missing fields", func(t *testing.T) {
		a, fake := statsApp(t)
		fake.numbers = nil
		zeros := map[string]int{"sessions": 0, "handsRaised": 0, "handsAnswered": 0, "faqEntries": 0, "participants": 0}
		for _, path := range []string{ofProject, ofSite} {
			if got := numbers(t, a, path, nil); !maps.Equal(got, zeros) {
				t.Errorf("%s: %v, want %v", path, got, zeros)
			}
		}
	})

	t.Run("a project nobody registered, or a name that is none, is a 404 and nothing is counted", func(t *testing.T) {
		a, fake := statsApp(t)
		res, body := call(a.mux, "GET", "/api/p/nobody/nothing/stats", "", nil)
		refused(t, "not registered", res, body, http.StatusNotFound, "Nobody hosts office hours for nobody/nothing")
		res, body = call(a.mux, "GET", "/api/p/paper_plane/stash/stats", "", nil)
		refused(t, "no repository name", res, body, http.StatusNotFound, "no such project")
		if len(fake.asked) != 0 {
			t.Errorf("the store counted for %q, want nothing asked", fake.asked)
		}
	})

	t.Run("NFR-OPS-2: a store that fails is a 500 with our sentence, and no number", func(t *testing.T) {
		a, fake := statsApp(t)
		fake.err = errors.New("connection refused to 10.0.0.5:5432")
		for _, path := range []string{ofProject, ofSite} {
			res, body := call(a.mux, "GET", path, "", nil)
			refused(t, path, res, body, http.StatusInternalServerError, "Something went wrong")
			if strings.Contains(body, "10.0.0.5") || strings.Contains(body, "sessions") {
				t.Errorf("%s: the reply carries the database's words or a number: %s", path, body)
			}
		}
	})

	t.Run("a server without a database says the stats are not set up", func(t *testing.T) {
		a, _ := statsApp(t)
		a.deps.stats = nil
		a.remux()
		for _, path := range []string{ofProject, ofSite} {
			res, body := call(a.mux, "GET", path, "", nil)
			refused(t, path, res, body, http.StatusServiceUnavailable, "aren't set up")
		}
	})
}
