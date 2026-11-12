package main

import (
	"errors"
	"fmt"
	"html"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"

	"officehours/store"
)

// faqPageFixture is an answer of PaperPlane/Stash, the project newQueueApp registers.
func faqPageFixture() store.FAQEntry {
	return store.FAQEntry{
		ID: 7, ProjectID: queueProject, HandID: 101, Slug: "retry-only-on-5xx",
		Title:     "How do I retry only on 5xx errors?",
		Question:  "I set retries: 3 and it retries on a 404 too.",
		Answer:    "Use retryIf and check the status. See https://example.com/docs/retry for the options.",
		Links:     []string{"https://example.com/docs/retry"},
		Author:    person(hostID),
		AskedBy:   3,
		CreatedAt: time.Date(2026, 11, 18, 17, 31, 0, 0, time.UTC),
	}
}

// faqPageCheck fails the test unless the reply is a whole HTML page with the wanted status and
// sentence, and the headers every FAQ page carries.
func faqPageCheck(t *testing.T, name string, res *http.Response, body string, status int, says string) {
	t.Helper()
	if res.StatusCode != status || !strings.HasPrefix(body, "<!doctype html>") || !strings.Contains(body, says) {
		t.Errorf("%s: status %d, want %d and a page that says %q:\n%s", name, res.StatusCode, status, says, body)
	}
	if got := res.Header.Get("Content-Type"); got != "text/html; charset=utf-8" {
		t.Errorf("%s: Content-Type %q, want text/html; charset=utf-8", name, got)
	}
	cache := "public, max-age=60"
	if status >= 500 {
		cache = "no-store" // a failure is not kept for a minute by every cache on the way
	}
	if got := res.Header.Get("Cache-Control"); got != cache {
		t.Errorf("%s: Cache-Control %q, want %q", name, got, cache)
	}
}

func TestFAQPageStatuses(t *testing.T) {
	t.Run("FR-FAQ-3, FR-FAQ-6: each page and each way it can fail to be one", func(t *testing.T) {
		a := newQueueApp(t)
		removed := faqPageFixture()
		removed.ID, removed.Slug, removed.Title, removed.Deleted = 8, "old-cache-format", "What was the old cache format?", true
		a.faq.entries = []store.FAQEntry{faqPageFixture(), removed}
		for _, c := range []struct {
			name, path string
			status     int
			says       string
		}{
			{"FR-FAQ-3: the list, signed out", "/p/PaperPlane/Stash/faq", http.StatusOK, "How do I retry only on 5xx errors?"},
			{"FR-FAQ-3: a search", "/p/PaperPlane/Stash/faq?q=retry", http.StatusOK, "1 answer for"},
			{"FR-FAQ-3: an answer, signed out", "/p/PaperPlane/Stash/faq/retry-only-on-5xx", http.StatusOK, "Answered live in office hours"},
			{"FR-FAQ-3: an answer's address in another letter case", "/p/paperplane/STASH/faq/retry-only-on-5xx", http.StatusOK, "Answered live in office hours"},
			{"FR-FAQ-6: a deleted answer is gone, and says so", "/p/PaperPlane/Stash/faq/old-cache-format", http.StatusGone, "This answer was removed"},
			{"an answer nobody wrote", "/p/PaperPlane/Stash/faq/no-such-answer", http.StatusNotFound, "There is no answer here"},
			{"the list of a project nobody registered", "/p/nobody/nothing/faq", http.StatusNotFound, "Nobody hosts office hours for nobody/nothing yet"},
			{"an answer of a project nobody registered", "/p/nobody/nothing/faq/retry-only-on-5xx", http.StatusNotFound, "Nobody hosts office hours"},
		} {
			res, body := call(a.mux, "GET", c.path, "", nil)
			faqPageCheck(t, c.name, res, body, c.status, c.says)
			if c.status == http.StatusGone && strings.Contains(body, "old cache format") {
				t.Errorf("%s: FR-FAQ-6: the removed answer's words are still on its page", c.name)
			}
		}
	})

	t.Run("NFR-SEC-3: an address no project or answer can have is a 404, and nothing is looked up", func(t *testing.T) {
		a := newQueueApp(t)
		a.projects.err = errors.New("the projects were asked")
		a.faq.err = errors.New("the FAQ was asked")
		for _, path := range []string{
			"/p/bad_owner/Stash/faq",
			"/p/" + strings.Repeat("a", 40) + "/Stash/faq/retry-only-on-5xx",
			"/p/PaperPlane/%3Cb%3E/faq",
			"/p/PaperPlane/Stash/faq/Not_A_Slug",
			"/p/PaperPlane/Stash/faq/%3Cscript%3E",
			"/p/PaperPlane/Stash/faq/" + strings.Repeat("a", 81),
		} {
			res, body := call(a.mux, "GET", path, "", nil)
			faqPageCheck(t, path, res, body, http.StatusNotFound, "There is no page here")
		}
		if len(a.faq.asked) != 0 {
			t.Errorf("the FAQ store was asked %v", a.faq.asked)
		}
	})

	t.Run("a server without a database says so, and no cache keeps that", func(t *testing.T) {
		mux := newMux(config{AppURL: testSite}, deps{})
		for _, path := range []string{"/p/PaperPlane/Stash/faq", "/p/PaperPlane/Stash/faq/retry-only-on-5xx"} {
			res, body := call(mux, "GET", path, "", nil)
			faqPageCheck(t, path, res, body, http.StatusServiceUnavailable, "isn't set up")
		}
	})

	t.Run("a store that fails is a 500 that tells nothing of why", func(t *testing.T) {
		const why = "dial tcp 10.0.0.7:5432: connection reset"
		a := newQueueApp(t)
		a.faq.entries = []store.FAQEntry{faqPageFixture()}
		a.faq.err = errors.New(why)
		for _, path := range []string{"/p/PaperPlane/Stash/faq", "/p/PaperPlane/Stash/faq?q=retry", "/p/PaperPlane/Stash/faq/retry-only-on-5xx"} {
			res, body := call(a.mux, "GET", path, "", nil)
			faqPageCheck(t, "FAQ store down: "+path, res, body, http.StatusInternalServerError, "Something went wrong on our side")
			if strings.Contains(body, "10.0.0.7") {
				t.Errorf("%s: the page shows the error: %s", path, body)
			}
		}
		a.faq.err, a.projects.err = nil, errors.New(why)
		res, body := call(a.mux, "GET", "/p/PaperPlane/Stash/faq", "", nil)
		faqPageCheck(t, "projects store down", res, body, http.StatusInternalServerError, "Something went wrong on our side")
	})

	t.Run("FR-FAQ-3: a page is the same for everyone, so a public cache may keep it", func(t *testing.T) {
		a := newQueueApp(t)
		a.faq.entries = []store.FAQEntry{faqPageFixture()}
		for _, path := range []string{"/p/PaperPlane/Stash/faq", "/p/PaperPlane/Stash/faq/retry-only-on-5xx"} {
			_, signedOut := call(a.mux, "GET", path, "", nil)
			_, maintainer := call(a.mux, "GET", path, "", a.as(t, hostID))
			if signedOut != maintainer {
				t.Errorf("%s: the page a maintainer gets differs from the one a visitor gets", path)
			}
		}
	})
}

func TestFAQPageEntry(t *testing.T) {
	const path = "/p/PaperPlane/Stash/faq/retry-only-on-5xx"
	withEntry := func(t *testing.T, e store.FAQEntry) *queueApp {
		a := newQueueApp(t)
		a.faq.entries = []store.FAQEntry{e}
		return a
	}

	t.Run("FR-FAQ-3: link previews name our own origin and the project's own spelling, whatever the request said", func(t *testing.T) {
		a := withEntry(t, faqPageFixture())
		res, body := call(a.mux, "GET", "/p/paperplane/stash/faq/retry-only-on-5xx", "", nil, func(r *http.Request) {
			r.Host = "evil.example"
			r.Header.Set("X-Forwarded-Host", "evil.example")
		})
		faqPageCheck(t, "entry", res, body, http.StatusOK, "<h1>How do I retry only on 5xx errors?</h1>")
		const own = "https://officehours.example/p/PaperPlane/Stash/faq/retry-only-on-5xx"
		for _, want := range []string{
			"<title>How do I retry only on 5xx errors? · PaperPlane/Stash FAQ</title>",
			`<meta name="description" content="Use retryIf and check the status. See https://example.com/docs/retry for the options.">`,
			`<meta property="og:title" content="How do I retry only on 5xx errors?">`,
			`<meta property="og:type" content="article">`,
			`<meta property="og:url" content="` + own + `">`,
			`<meta name="twitter:card" content="summary">`,
			`<link rel="canonical" href="` + own + `">`,
		} {
			if !strings.Contains(body, want) {
				t.Errorf("the page lacks %s", want)
			}
		}
		if strings.Contains(body, "evil.example") || strings.Contains(body, "paperplane/stash") {
			t.Error("the page took a name from the request instead of from the project")
		}
	})

	t.Run("FR-HAND-8, FR-FAQ-5: how many asked, and an issue link filled in from the answer", func(t *testing.T) {
		a := withEntry(t, faqPageFixture())
		_, body := call(a.mux, "GET", path, "", nil)
		if !strings.Contains(body, "asked by 3 people") || strings.Contains(body, "login-gh2") {
			t.Errorf("want the number of askers and no name but the maintainer's:\n%s", body)
		}
		m := regexp.MustCompile(`href="(https://github\.com/[^"]*)"`).FindStringSubmatch(body)
		if m == nil {
			t.Fatalf("no issue link:\n%s", body)
		}
		u, err := url.Parse(html.UnescapeString(m[1]))
		if err != nil || u.Path != "/PaperPlane/Stash/issues/new" {
			t.Fatalf("issue link %q (%v), want GitHub's new-issue page of PaperPlane/Stash", m[1], err)
		}
		if q := u.Query(); q.Get("title") != "How do I retry only on 5xx errors?" ||
			!strings.HasPrefix(q.Get("body"), "I set retries: 3 and it retries on a 404 too.") ||
			!strings.HasSuffix(q.Get("body"), "https://officehours.example"+path) {
			t.Errorf("issue query = %v, want the question, its details and the answer's address", q)
		}
	})

	t.Run("NFR-SEC-6: markup, quotes and javascript: arrive as text, and only http(s) is a link", func(t *testing.T) {
		evil := faqPageFixture()
		evil.Title = `<script>alert("t")</script> it's`
		evil.Question = `"quoted" <img src=x onerror=alert('q')>`
		evil.Answer = `javascript:alert('a') <a href="javascript:alert(1)">click</a> see https://example.com/ok`
		evil.Links = []string{"javascript:alert('l')", "https://example.com/fine"}
		a := withEntry(t, evil)
		res, body := call(a.mux, "GET", path, "", nil)
		faqPageCheck(t, "hostile entry", res, body, http.StatusOK, "<h1>&lt;script&gt;alert(&#34;t&#34;)&lt;/script&gt; it&#39;s</h1>")
		for _, bad := range []string{"<script>alert", "<img src", `href="javascript:`, `<a href="javascript`} {
			if strings.Contains(body, bad) {
				t.Errorf("the page contains %s", bad)
			}
		}
		for _, want := range []string{
			"&#34;quoted&#34; &lt;img src=x onerror=alert(&#39;q&#39;)&gt;",
			`javascript:alert(&#39;a&#39;) &lt;a href=&#34;javascript:alert(1)&#34;&gt;click&lt;/a&gt; see <a href="https://example.com/ok" rel="noopener noreferrer nofollow">https://example.com/ok</a>`,
			"<li>javascript:alert(&#39;l&#39;)</li>",
			`<a href="https://example.com/fine" rel="noopener noreferrer nofollow">https://example.com/fine</a>`,
		} {
			if !strings.Contains(body, want) {
				t.Errorf("the page lacks %s", want)
			}
		}
	})

	t.Run("FR-SESS-5: the next session, while it lies ahead, in UTC for the page's script to localise", func(t *testing.T) {
		next := now().Add(48 * time.Hour).Truncate(time.Minute)
		a := withEntry(t, faqPageFixture())
		a.queue.snapshot = store.Snapshot{NextSessionAt: next}
		res, body := call(a.mux, "GET", path, "", nil)
		want := `<time datetime="` + next.UTC().Format(time.RFC3339) + `" data-local>` + next.UTC().Format("Mon 2 Jan 2006, 15:04 UTC") + `</time>`
		faqPageCheck(t, "next session", res, body, http.StatusOK, want)
	})

	t.Run("FR-SESS-5: no next session for a time that has passed, a queue that fails, or no queue", func(t *testing.T) {
		a := withEntry(t, faqPageFixture())
		check := func(name string) {
			t.Helper()
			res, body := call(a.mux, "GET", path, "", nil)
			faqPageCheck(t, name, res, body, http.StatusOK, "Answered live in office hours")
			if strings.Contains(body, "data-local") || strings.Contains(body, "<script") {
				t.Errorf("%s: the page shows a next session", name)
			}
		}
		a.queue.snapshot = store.Snapshot{NextSessionAt: now().Add(-time.Hour)}
		check("a time that has passed")
		a.queue.snapshot = store.Snapshot{NextSessionAt: now().Add(time.Hour)}
		a.queue.err = errors.New("connection reset")
		check("a queue that fails")
		a.deps.queue = nil
		a.remux()
		check("no queue")
	})
}

func TestFAQPageList(t *testing.T) {
	const path = "/p/PaperPlane/Stash/faq"

	t.Run("FR-FAQ-3: the newest first, each a card that links to its page, and the room", func(t *testing.T) {
		a := newQueueApp(t)
		older := faqPageFixture()
		older.ID, older.Slug, older.Title, older.CreatedAt = 6, "where-is-the-cache", "Where is the cache stored?", older.CreatedAt.Add(-24*time.Hour)
		a.faq.entries = []store.FAQEntry{older, faqPageFixture()}
		res, body := call(a.mux, "GET", path, "", nil)
		faqPageCheck(t, "list", res, body, http.StatusOK, "PaperPlane/Stash · FAQ · [ 2 answers ]")
		newer, old := strings.Index(body, `href="/p/PaperPlane/Stash/faq/retry-only-on-5xx"`), strings.Index(body, `href="/p/PaperPlane/Stash/faq/where-is-the-cache"`)
		if newer < 0 || old < 0 || newer > old {
			t.Errorf("the cards are not the two answers, the newest first:\n%s", body)
		}
		if !strings.Contains(body, `href="/p/PaperPlane/Stash">Join the room</a>`) {
			t.Error("the list has no link to the room")
		}
		if got := a.faq.asked; len(got) != 1 || got[0] != "list prj1 q= limit=201" {
			t.Errorf("the store was asked %v, want one list of at most 201", got)
		}
	})

	t.Run("FR-FAQ-3: a search is cleaned, asked of the store, and kept in the field", func(t *testing.T) {
		a := newQueueApp(t)
		a.faq.entries = []store.FAQEntry{faqPageFixture()}
		res, body := call(a.mux, "GET", path+"?q="+url.QueryEscape("  retry \n  only  "), "", nil)
		faqPageCheck(t, "search", res, body, http.StatusOK, `value="retry only"`)
		if got := a.faq.asked; len(got) != 1 || got[0] != "list prj1 q=retry only limit=201" {
			t.Errorf("the store was asked %v", got)
		}
	})

	t.Run("FR-FAQ-3: a search is at most 100 characters", func(t *testing.T) {
		a := newQueueApp(t)
		res, body := call(a.mux, "GET", path+"?q="+url.QueryEscape(strings.Repeat("é", 150)), "", nil)
		kept := strings.Repeat("é", 100)
		faqPageCheck(t, "long search", res, body, http.StatusOK, `value="`+kept+`"`)
		if got := a.faq.asked; len(got) != 1 || got[0] != "list prj1 q="+kept+" limit=201" {
			t.Errorf("the store was asked %v", got)
		}
	})

	t.Run("FR-FAQ-3: the empty states, without and with a search", func(t *testing.T) {
		a := newQueueApp(t)
		res, body := call(a.mux, "GET", path, "", nil)
		faqPageCheck(t, "no answers", res, body, http.StatusOK, "No answers yet")
		res, body = call(a.mux, "GET", path+"?q=kubernetes", "", nil)
		faqPageCheck(t, "no match", res, body, http.StatusOK, "No answer matches “kubernetes”")
	})

	t.Run("FR-FAQ-3: more answers than a page holds: the newest 200, and a sentence that says so", func(t *testing.T) {
		a := newQueueApp(t)
		for i := range 201 {
			e := faqPageFixture()
			e.ID, e.Slug, e.CreatedAt = int64(1000+i), fmt.Sprintf("answer-%d", i), e.CreatedAt.Add(time.Duration(i)*time.Minute)
			a.faq.entries = append(a.faq.entries, e)
		}
		res, body := call(a.mux, "GET", path, "", nil)
		faqPageCheck(t, "a long list", res, body, http.StatusOK, "Showing the newest 200 answers")
		if n := strings.Count(body, `<a class="card"`); n != 200 {
			t.Errorf("%d cards, want 200", n)
		}
		if strings.Contains(body, `href="/p/PaperPlane/Stash/faq/answer-0"`) {
			t.Error("the oldest answer is on the page, and one of the newest is not")
		}
	})
}
