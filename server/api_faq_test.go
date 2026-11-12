package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"officehours/store"
)

// faqAPIOtherProject is a second project, which strangerID maintains (faqAPIDeleteApp).
const faqAPIOtherProject = "prj2"

// faqAPIEntry is an entry of the queue tests' project, written the given number of minutes before
// a fixed moment: the larger the number, the older the entry.
func faqAPIEntry(id int64, title string, minutesAgo int) store.FAQEntry {
	return store.FAQEntry{
		ID: id, ProjectID: queueProject, HandID: 100 + id, Slug: fmt.Sprintf("question-%d", id), Title: title,
		Question: "What I tried so far.", Answer: "Use retryIf and check the status.",
		Links: []string{"https://example.com/docs/retry"}, Author: person(hostID), AskedBy: 2,
		CreatedAt: time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC).Add(-time.Duration(minutesAgo) * time.Minute),
	}
}

// faqAPIApp is newQueueApp with n entries behind the FAQ routes: ids 1 to n, the highest the newest.
func faqAPIApp(t *testing.T, n int) *queueApp {
	t.Helper()
	a := newQueueApp(t)
	for id := 1; id <= n; id++ {
		a.faq.entries = append(a.faq.entries, faqAPIEntry(int64(id), fmt.Sprintf("Question %d", id), n-id))
	}
	return a
}

// faqAPIIDs reads the ids of the entries in an answer, in its order: "7 6 5".
func faqAPIIDs(t *testing.T, body string) string {
	t.Helper()
	var entries []faqJSON
	decode(t, body, &entries)
	ids := make([]string, 0, len(entries))
	for _, e := range entries {
		ids = append(ids, e.ID)
	}
	return strings.Join(ids, " ")
}

// faqAPIText is the body of a similar-questions request for a text, whatever characters it holds.
func faqAPIText(text string) string {
	body, err := json.Marshal(map[string]string{"text": text})
	if err != nil {
		panic(err)
	}
	return string(body)
}

// What a request can be made to look like when it does not come from one of our pages.
func faqAPIOtherSite(r *http.Request) { r.Header.Set("Origin", "https://evil.example") }
func faqAPINoOrigin(r *http.Request)  { r.Header.Del("Origin") }
func faqAPIForm(r *http.Request) {
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
}

// faqAPIRefusal is one request that must be refused.
type faqAPIRefusal struct {
	name, path, body string
	cookie           *http.Cookie
	mutate           func(*http.Request)
	status           int
	says             string
}

func (c faqAPIRefusal) check(t *testing.T, mux *http.ServeMux) {
	t.Helper()
	var mutate []func(*http.Request)
	if c.mutate != nil {
		mutate = append(mutate, c.mutate)
	}
	res, body := call(mux, "POST", c.path, c.body, c.cookie, mutate...)
	refused(t, c.name, res, body, c.status, c.says)
}

func TestFAQAPISimilar(t *testing.T) {
	const (
		path     = "/api/p/paperplane/stash/faq/similar"
		question = "How do I retry only on 5xx errors?"
	)

	t.Run("FR-FAQ-4, FR-HAND-3: at most 3 entries of the project, in the order the store ranked them", func(t *testing.T) {
		a := faqAPIApp(t, 4)
		res, body := call(a.mux, "POST", path, faqAPIText(question), a.as(t, askerID))
		if got := faqAPIIDs(t, body); res.StatusCode != http.StatusOK || got != "4 3 2" {
			t.Fatalf("status %d, entries %q, want 200 and 4 3 2: %s", res.StatusCode, got, body)
		}
		if want := []string{"suggest prj1 text=" + question + " limit=3"}; !slices.Equal(a.faq.asked, want) {
			t.Errorf("the store was asked %q, want %q", a.faq.asked, want)
		}
	})

	t.Run("FR-HAND-3: anyone signed in may ask, in the room or not", func(t *testing.T) {
		for _, id := range []string{hostID, otherID, strangerID} {
			a := faqAPIApp(t, 2)
			res, body := call(a.mux, "POST", path, faqAPIText(question), a.as(t, id))
			if got := faqAPIIDs(t, body); res.StatusCode != http.StatusOK || got != "2 1" {
				t.Errorf("as %s: status %d, entries %q, want 200 and 2 1: %s", id, res.StatusCode, got, body)
			}
		}
	})

	t.Run("FR-FAQ-4: a project without answers yet, or nothing alike, is an empty list, never null", func(t *testing.T) {
		a := faqAPIApp(t, 0)
		res, body := call(a.mux, "POST", path, faqAPIText(question), a.as(t, askerID))
		if res.StatusCode != http.StatusOK || !strings.Contains(body, `"data":[]`) || len(a.faq.asked) != 1 {
			t.Fatalf("status %d body %s, asked %q; want 200 with an empty list from the store", res.StatusCode, body, a.faq.asked)
		}
	})

	t.Run("NFR-SEC-3: the text is searched as one clean line", func(t *testing.T) {
		a := faqAPIApp(t, 1)
		call(a.mux, "POST", path, faqAPIText("  How do I\r\n\tretry   only on 5xx errors?  \n"), a.as(t, askerID))
		if want := []string{"suggest prj1 text=" + question + " limit=3"}; !slices.Equal(a.faq.asked, want) {
			t.Errorf("the store was asked %q, want %q", a.faq.asked, want)
		}
	})

	t.Run("FR-FAQ-4: fewer than 3 characters after trimming is an empty list, and the store is not asked", func(t *testing.T) {
		a := faqAPIApp(t, 4)
		for name, body := range map[string]string{
			"nothing typed yet":                  faqAPIText(""),
			"no text at all":                     `{}`,
			"only spaces and line breaks":        faqAPIText("  \n\t \r\n"),
			"two letters":                        faqAPIText("ab"),
			"two letters between spaces":         faqAPIText("  ab \n"),
			"two characters of three bytes each": faqAPIText("日本"),
		} {
			res, text := call(a.mux, "POST", path, body, a.as(t, askerID))
			if res.StatusCode != http.StatusOK || !strings.Contains(text, `"data":[]`) {
				t.Errorf("%s: status %d body %s, want 200 with an empty list", name, res.StatusCode, text)
			}
		}
		if len(a.faq.asked) != 0 {
			t.Errorf("the store was asked %q, want nothing asked", a.faq.asked)
		}
	})

	t.Run("FR-FAQ-4: 3 characters are a question, counted the way a person counts them", func(t *testing.T) {
		for _, c := range []struct{ typed, searched string }{
			{"abc", "abc"}, {"a b", "a b"}, {"  tls\n", "tls"}, {"日本語", "日本語"},
		} {
			a := faqAPIApp(t, 1)
			res, body := call(a.mux, "POST", path, faqAPIText(c.typed), a.as(t, askerID))
			if want := []string{"suggest prj1 text=" + c.searched + " limit=3"}; res.StatusCode != http.StatusOK || !slices.Equal(a.faq.asked, want) {
				t.Errorf("%q: status %d body %s, the store was asked %q, want %q", c.typed, res.StatusCode, body, a.faq.asked, want)
			}
		}
	})

	t.Run("NFR-SEC-3: 2,200 characters are read, one more is refused and the store is not asked", func(t *testing.T) {
		for _, c := range []struct {
			name, text string
			read       bool
		}{
			{"2,200 letters", strings.Repeat("x", 2200), true},
			{"2,200 characters of two bytes each", strings.Repeat("é", 2200), true},
			{"2,200 letters between spaces", "  " + strings.Repeat("x", 2200) + " \n", true},
			{"2,201 letters", strings.Repeat("x", 2201), false},
			{"2,201 characters of two bytes each", strings.Repeat("é", 2201), false},
		} {
			a := faqAPIApp(t, 1)
			res, body := call(a.mux, "POST", path, faqAPIText(c.text), a.as(t, askerID))
			if c.read {
				if res.StatusCode != http.StatusOK || len(a.faq.asked) != 1 {
					t.Errorf("%s: status %d, the store was asked %d times; want 200 and once", c.name, res.StatusCode, len(a.faq.asked))
				}
				continue
			}
			refused(t, c.name, res, body, http.StatusBadRequest, "2,200 characters")
			if len(a.faq.asked) != 0 {
				t.Errorf("%s: the store was asked although the text was refused", c.name)
			}
		}
	})

	t.Run("NFR-SEC-2, NFR-SEC-3, NFR-SEC-5: what is refused, and the store is not asked", func(t *testing.T) {
		a := faqAPIApp(t, 4)
		member, ask := a.as(t, askerID), faqAPIText(question)
		for _, c := range []faqAPIRefusal{
			{"signed out", path, ask, nil, nil, http.StatusUnauthorized, "Sign in first"},
			{"another site's Origin", path, ask, member, faqAPIOtherSite, http.StatusForbidden, "did not come from Office Hours"},
			{"no Origin", path, ask, member, faqAPINoOrigin, http.StatusForbidden, "did not come from Office Hours"},
			{"a form post", path, ask, member, faqAPIForm, http.StatusUnsupportedMediaType, "JSON"},
			{"a text that is a number", path, `{"text":5}`, member, nil, http.StatusBadRequest, "JSON"},
			{"a field this route does not have", path, `{"text":"` + question + `","limit":50}`, member, nil, http.StatusBadRequest, "JSON"},
			{"more than the guard reads", path, faqAPIText(strings.Repeat("x", 17000)), member, nil, http.StatusRequestEntityTooLarge, "too large"},
			{"a project nobody registered", "/api/p/nobody/nothing/faq/similar", ask, member, nil, http.StatusNotFound, "Nobody hosts office hours for nobody/nothing"},
			{"a name that is no repository", "/api/p/paper_plane/stash/faq/similar", ask, member, nil, http.StatusNotFound, "no such project"},
		} {
			c.check(t, a.mux)
		}
		if len(a.faq.asked) != 0 {
			t.Errorf("a refused request reached the store: %q", a.faq.asked)
		}
	})

	t.Run("plan decision 10: the search is never a GET, so a draft is never part of an address", func(t *testing.T) {
		a := faqAPIApp(t, 4)
		res, body := call(a.mux, "GET", path+"?text=How+do+I+retry+only+on+5xx", "", a.as(t, askerID))
		if res.StatusCode != http.StatusMethodNotAllowed || len(a.faq.asked) != 0 {
			t.Errorf("status %d body %s, the store was asked %q; want 405 and nothing asked", res.StatusCode, body, a.faq.asked)
		}
	})

	t.Run("NFR-OPS-2: a store that fails is a 500 with our sentence, and none of its words", func(t *testing.T) {
		a := faqAPIApp(t, 4)
		a.faq.err = errors.New("connection refused to 10.0.0.5:5432")
		res, body := call(a.mux, "POST", path, faqAPIText(question), a.as(t, askerID))
		refused(t, "store down", res, body, http.StatusInternalServerError, "Something went wrong")
		if strings.Contains(body, "10.0.0.5") || strings.Contains(body, "Question") {
			t.Errorf("the reply carries the database's words or an entry: %s", body)
		}
	})

	t.Run("a server without a database says the FAQ is not set up", func(t *testing.T) {
		a := faqAPIApp(t, 4)
		a.deps.faq = nil
		a.remux()
		res, body := call(a.mux, "POST", path, faqAPIText(question), a.as(t, askerID))
		refused(t, "no FAQ store", res, body, http.StatusServiceUnavailable, "isn't set up")
	})
}

func TestFAQAPILatest(t *testing.T) {
	const path = "/api/p/paperplane/stash/faq/latest"

	t.Run("FR-PROJ-3: the project's 5 newest entries, the newest first, for anyone", func(t *testing.T) {
		a := faqAPIApp(t, 7)
		for name, c := range map[string]struct {
			path   string
			cookie *http.Cookie
		}{
			"signed out":                  {path, nil},
			"a member":                    {path, a.as(t, askerID)},
			"signed in, outside the room": {"/api/p/PaperPlane/STASH/faq/latest", a.as(t, strangerID)},
		} {
			res, body := call(a.mux, "GET", c.path, "", c.cookie)
			if got := faqAPIIDs(t, body); res.StatusCode != http.StatusOK || got != "7 6 5 4 3" {
				t.Errorf("%s: status %d, entries %q, want 200 and 7 6 5 4 3: %s", name, res.StatusCode, got, body)
			}
		}
		if want := slices.Repeat([]string{"list prj1 q= limit=5"}, 3); !slices.Equal(a.faq.asked, want) {
			t.Errorf("the store was asked %q, want %q", a.faq.asked, want)
		}
	})

	t.Run("FR-PROJ-3: a project without answers yet is an empty list, never null", func(t *testing.T) {
		a := faqAPIApp(t, 0)
		res, body := call(a.mux, "GET", path, "", nil)
		if res.StatusCode != http.StatusOK || !strings.Contains(body, `"data":[]`) {
			t.Fatalf("status %d body %s, want 200 with an empty list", res.StatusCode, body)
		}
	})

	t.Run("FR-HAND-8, NFR-SEC-6: an entry is its words, a slug and a count of askers: no names, no address, no id of the project", func(t *testing.T) {
		const nasty = `Why does "retry" fail? </script><b>x</b>`
		a := newQueueApp(t)
		entry := faqAPIEntry(7, nasty, 0)
		entry.Links = nil
		a.faq.entries = []store.FAQEntry{entry}
		res, body := call(a.mux, "GET", path, "", nil)
		var got []map[string]any
		decode(t, body, &got)
		want := map[string]any{
			"id": "7", "slug": "question-7", "title": nasty, "question": "What I tried so far.",
			"answer": "Use retryIf and check the status.", "links": []any{}, "askedBy": float64(2),
			"createdAt": "2026-11-14T12:00:00Z", "author": "login-gh1",
		}
		if res.StatusCode != http.StatusOK || len(got) != 1 || !reflect.DeepEqual(got[0], want) {
			t.Fatalf("status %d, entries %v, want exactly %v", res.StatusCode, got, want)
		}
	})

	t.Run("a project nobody registered, or a name that is none, is a 404 and the store is not asked", func(t *testing.T) {
		a := faqAPIApp(t, 7)
		res, body := call(a.mux, "GET", "/api/p/nobody/nothing/faq/latest", "", nil)
		refused(t, "not registered", res, body, http.StatusNotFound, "Nobody hosts office hours for nobody/nothing")
		res, body = call(a.mux, "GET", "/api/p/paper_plane/stash/faq/latest", "", nil)
		refused(t, "no repository name", res, body, http.StatusNotFound, "no such project")
		if len(a.faq.asked) != 0 {
			t.Errorf("the store was asked %q, want nothing asked", a.faq.asked)
		}
	})

	t.Run("NFR-OPS-2: a store that fails is a 500 with our sentence, and none of its words", func(t *testing.T) {
		a := faqAPIApp(t, 7)
		a.faq.err = errors.New("connection refused to 10.0.0.5:5432")
		res, body := call(a.mux, "GET", path, "", nil)
		refused(t, "store down", res, body, http.StatusInternalServerError, "Something went wrong")
		if strings.Contains(body, "10.0.0.5") || strings.Contains(body, "Question") {
			t.Errorf("the reply carries the database's words or an entry: %s", body)
		}
	})

	t.Run("a server without a database says the FAQ is not set up", func(t *testing.T) {
		a := faqAPIApp(t, 7)
		a.deps.faq = nil
		a.remux()
		res, body := call(a.mux, "GET", path, "", nil)
		refused(t, "no FAQ store", res, body, http.StatusServiceUnavailable, "isn't set up")
	})
}

// faqAPIDeleteApp is newQueueApp with two entries of its project (7 and 8) and one of another
// project (9), which strangerID maintains.
func faqAPIDeleteApp(t *testing.T) *queueApp {
	t.Helper()
	a := newQueueApp(t)
	foreign := faqAPIEntry(9, "How do I add a map layer?", 0)
	foreign.ProjectID = faqAPIOtherProject
	a.faq.entries = []store.FAQEntry{
		faqAPIEntry(7, "How do I retry only on 5xx errors?", 2),
		faqAPIEntry(8, "Can two processes share the cache?", 1),
		foreign,
	}
	a.projects.maintainers[faqAPIOtherProject+" "+strangerID] = true
	return a
}

// faqAPIDeleted lists the ids of the entries the store holds as deleted: "7 9".
func faqAPIDeleted(a *queueApp) string {
	var ids []string
	for _, e := range a.faq.entries {
		if e.Deleted {
			ids = append(ids, idText(e.ID))
		}
	}
	return strings.Join(ids, " ")
}

// faqAPIDeleteFails is the shared fakeFAQ in front of a database that gives up while the delete
// is written: the entry is found, and it stays.
type faqAPIDeleteFails struct{ *fakeFAQ }

func (faqAPIDeleteFails) DeleteFAQ(context.Context, int64) error {
	return errors.New("connection reset by 10.0.0.5:5432")
}

func TestFAQAPIDelete(t *testing.T) {
	deleteOf := func(id string) string { return "/api/faq/" + id + "/delete" }

	t.Run("FR-FAQ-6: a maintainer of the entry's project deletes it, and doing it again is fine", func(t *testing.T) {
		a := faqAPIDeleteApp(t)
		for _, attempt := range []string{"the first time", "again"} {
			res, body := call(a.mux, "POST", deleteOf("7"), `{}`, a.as(t, hostID))
			var got map[string]string
			decode(t, body, &got)
			if res.StatusCode != http.StatusOK || !maps.Equal(got, map[string]string{"id": "7"}) {
				t.Fatalf("%s: status %d body %s, want 200 and the id as text", attempt, res.StatusCode, body)
			}
			if deleted := faqAPIDeleted(a); deleted != "7" {
				t.Fatalf("%s: deleted entries %q, want only 7", attempt, deleted)
			}
		}
		if len(a.faq.asked) == 0 || a.faq.asked[0] != "delete 7" {
			t.Errorf("the store was asked %q, want it to start with the delete of 7", a.faq.asked)
		}
	})

	t.Run("FR-FAQ-6, NFR-SEC-2: the role is the one in the entry's own project", func(t *testing.T) {
		a := faqAPIDeleteApp(t)
		// strangerID has no role in PaperPlane/Stash and maintains the project of entry 9.
		res, body := call(a.mux, "POST", deleteOf("9"), `{}`, a.as(t, strangerID))
		if res.StatusCode != http.StatusOK || faqAPIDeleted(a) != "9" {
			t.Fatalf("status %d body %s, deleted %q; want 200 and only 9 deleted", res.StatusCode, body, faqAPIDeleted(a))
		}
	})

	t.Run("AT-10, NFR-SEC-2, NFR-SEC-5: who is refused, and nothing is deleted", func(t *testing.T) {
		a := faqAPIDeleteApp(t)
		host := a.as(t, hostID)
		for _, c := range []faqAPIRefusal{
			{"a member", deleteOf("7"), `{}`, a.as(t, askerID), nil, http.StatusForbidden, "Only a maintainer"},
			{"someone with no role anywhere", deleteOf("7"), `{}`, a.as(t, "gh77"), nil, http.StatusForbidden, "Only a maintainer"},
			{"a maintainer of another project", deleteOf("7"), `{}`, a.as(t, strangerID), nil, http.StatusForbidden, "Only a maintainer"},
			{"this project's maintainer on another project's entry", deleteOf("9"), `{}`, host, nil, http.StatusForbidden, "Only a maintainer"},
			{"signed out", deleteOf("7"), `{}`, nil, nil, http.StatusUnauthorized, "Sign in first"},
			{"another site's Origin", deleteOf("7"), `{}`, host, faqAPIOtherSite, http.StatusForbidden, "did not come from Office Hours"},
			{"no Origin", deleteOf("7"), `{}`, host, faqAPINoOrigin, http.StatusForbidden, "did not come from Office Hours"},
			{"a form post", deleteOf("7"), `{}`, host, faqAPIForm, http.StatusUnsupportedMediaType, "JSON"},
			{"a body that names another entry", deleteOf("7"), `{"id":"8"}`, host, nil, http.StatusBadRequest, "JSON"},
		} {
			c.check(t, a.mux)
		}
		// A link or an image on another page can only make a GET.
		if res, _ := call(a.mux, "GET", deleteOf("7"), "", host); res.StatusCode != http.StatusMethodNotAllowed {
			t.Errorf("a GET: status %d, want 405", res.StatusCode)
		}
		if deleted := faqAPIDeleted(a); deleted != "" || len(a.faq.asked) != 0 {
			t.Errorf("deleted entries %q, the store was asked %q; want nothing deleted and nothing asked", deleted, a.faq.asked)
		}
	})

	t.Run("FR-FAQ-6: an id that names no entry is a 404, and nothing is deleted", func(t *testing.T) {
		a := faqAPIDeleteApp(t)
		for _, id := range []string{"999", "0", "-7", "7.5", "7abc", "abc", "99999999999999999999"} {
			res, body := call(a.mux, "POST", deleteOf(id), `{}`, a.as(t, hostID))
			refused(t, "id "+id, res, body, http.StatusNotFound, "no such FAQ entry")
		}
		if deleted := faqAPIDeleted(a); deleted != "" || len(a.faq.asked) != 0 {
			t.Errorf("deleted entries %q, the store was asked %q; want nothing deleted and nothing asked", deleted, a.faq.asked)
		}
	})

	t.Run("NFR-OPS-2: a database that fails is a 500 with our sentence, and never reads as deleted", func(t *testing.T) {
		for name, breakIt := range map[string]func(*queueApp){
			"the entry cannot be looked up":         func(a *queueApp) { a.faq.err = errors.New("connection refused to 10.0.0.5:5432") },
			"the caller's role cannot be looked up": func(a *queueApp) { a.projects.err = errors.New("connection refused to 10.0.0.5:5432") },
			"the delete itself fails":               func(a *queueApp) { a.deps.faq = faqAPIDeleteFails{a.faq}; a.remux() },
		} {
			a := faqAPIDeleteApp(t)
			breakIt(a)
			res, body := call(a.mux, "POST", deleteOf("7"), `{}`, a.as(t, hostID))
			refused(t, name, res, body, http.StatusInternalServerError, "Something went wrong")
			if strings.Contains(body, "10.0.0.5") || faqAPIDeleted(a) != "" {
				t.Errorf("%s: body %s, deleted %q; want none of the database's words and nothing deleted", name, body, faqAPIDeleted(a))
			}
		}
	})

	t.Run("a server without a database says the FAQ is not set up", func(t *testing.T) {
		a := faqAPIDeleteApp(t)
		a.deps.faq = nil
		a.remux()
		res, body := call(a.mux, "POST", deleteOf("7"), `{}`, a.as(t, hostID))
		refused(t, "no FAQ store", res, body, http.StatusServiceUnavailable, "isn't set up")
	})
}
