package pages

import (
	"html"
	"io"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"officehours/store"
)

const outsideRel = `rel="noopener noreferrer nofollow"`

var testSite = Site{Origin: "https://officehours.example", Owner: "PaperPlane", Repo: "Stash"}

// testEntry is an answer as the store hands it over.
func testEntry() store.FAQEntry {
	return store.FAQEntry{
		ID: 7, ProjectID: "prj1", Slug: "retry-only-on-5xx", Title: "How do I retry only on 5xx errors?",
		Question: "I set retries: 3 and it retries on a 404 too.\nIs there a filter?",
		Answer:   "Use retryIf and check the status.\n\nSee https://example.com/docs/retry for the options.",
		Links:    []string{"https://example.com/docs/retry", "https://github.com/paperplane/stash/pull/41?a=1&b=2"},
		Author:   store.Person{UserID: "gh1", Login: "mia-k", Name: "Mia Kowalski", AvatarURL: "https://avatars.example/1"},
		AskedBy:  3,
		// Late evening in India is still the 18th in UTC, and the page writes dates in UTC.
		CreatedAt: time.Date(2026, 11, 19, 2, 0, 0, 0, time.FixedZone("IST", 5*3600+1800)),
	}
}

// render is the page as a browser receives it.
func render(t *testing.T, p interface{ Render(io.Writer) error }) string {
	t.Helper()
	var b strings.Builder
	if err := p.Render(&b); err != nil {
		t.Fatalf("Render: %v", err)
	}
	return b.String()
}

func has(t *testing.T, body string, wants ...string) {
	t.Helper()
	for _, want := range wants {
		if !strings.Contains(body, want) {
			t.Errorf("the page lacks %q", want)
		}
	}
}

func lacks(t *testing.T, body string, unwanted ...string) {
	t.Helper()
	for _, bad := range unwanted {
		if strings.Contains(body, bad) {
			t.Errorf("the page contains %q", bad)
		}
	}
}

var (
	anchorRe = regexp.MustCompile(`<a\s[^>]*>`)
	hrefRe   = regexp.MustCompile(`\b(?:href|src|action)="([^"]*)"`)
)

// checkAddresses fails the test for an address on the page that is neither on our own origin
// nor http(s), and for a link that leaves the site without the rel that NFR-SEC-6 asks for.
func checkAddresses(t *testing.T, body string) {
	t.Helper()
	for _, m := range hrefRe.FindAllStringSubmatch(body, -1) {
		addr := html.UnescapeString(m[1])
		own := strings.HasPrefix(addr, "/") && !strings.HasPrefix(addr, "//")
		if !own && !strings.HasPrefix(addr, "https://") && !strings.HasPrefix(addr, "http://") && !strings.HasPrefix(addr, "data:image/svg+xml,") {
			t.Errorf("NFR-SEC-6: the page has the address %q", addr)
		}
	}
	for _, tag := range anchorRe.FindAllString(body, -1) {
		if strings.Contains(tag, `href="http`) && !strings.Contains(tag, outsideRel) {
			t.Errorf("NFR-SEC-6: a link that leaves the site has no %s: %s", outsideRel, tag)
		}
	}
}

func TestLinked(t *testing.T) {
	link := func(addr string) part { return part{Text: addr, Href: addr} }
	text := func(s string) part { return part{Text: s} }
	tests := []struct {
		name string
		in   string
		want []part
	}{
		{"text without an address is one piece", "Use retryIf.\nThen check the status.", []part{text("Use retryIf.\nThen check the status.")}},
		{"nothing is nothing", "", nil},
		{"FR-HAND-1: an https address becomes a link, and the full stop after it stays text",
			"See https://example.com/docs.", []part{text("See "), link("https://example.com/docs"), text(".")}},
		{"FR-HAND-1: http works too, with a query and a fragment",
			"http://a.example/x?y=1&z=2#top", []part{link("http://a.example/x?y=1&z=2#top")}},
		{"two addresses and the words between them",
			"https://a.example then https://b.example/b", []part{link("https://a.example"), text(" then "), link("https://b.example/b")}},
		{"brackets around an address are not part of it",
			"(see https://example.com/a)", []part{text("(see "), link("https://example.com/a"), text(")")}},
		{"a bracket that opened inside the address belongs to it",
			"https://en.wikipedia.org/wiki/Go_(game)", []part{link("https://en.wikipedia.org/wiki/Go_(game)")}},
		{"the punctuation of a sentence is not part of the address",
			"Is it https://example.com/a?b=c!? Or https://example.com/d...", []part{text("Is it "), link("https://example.com/a?b=c"), text("!? Or "), link("https://example.com/d"), text("...")}},
		{"an address in capitals is still one", "HTTPS://EXAMPLE.COM/Docs", []part{link("HTTPS://EXAMPLE.COM/Docs")}},
		{"an address with letters from another alphabet", "https://example.com/straße", []part{link("https://example.com/straße")}},
		{"markdown keeps its brackets as text", "[docs](https://example.com/a)", []part{text("[docs]("), link("https://example.com/a"), text(")")}},
		{"NFR-SEC-6: javascript: is never a link", "javascript:alert(1)", []part{text("javascript:alert(1)")}},
		{"NFR-SEC-6: no other scheme is a link",
			"JAVASCRIPT:alert(1) data:text/html,<script>x</script> vbscript:x file:///etc/passwd ftp://example.com mailto:a@b.example //example.com",
			[]part{text("JAVASCRIPT:alert(1) data:text/html,<script>x</script> vbscript:x file:///etc/passwd ftp://example.com mailto:a@b.example //example.com")}},
		{"NFR-SEC-6: markup around an address stays text",
			`<a href="https://evil.example" onclick="x()">y</a>`, []part{text(`<a href="`), link("https://evil.example"), text(`" onclick="x()">y</a>`)}},
		{"a scheme without a host is text", "https:// and http://", []part{text("https:// and http://")}},
		{"a scheme in the middle of a word is text", "xhttps://example.com", []part{text("xhttps://example.com")}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := linked(tt.in)
			if len(got) != len(tt.want) {
				t.Fatalf("linked(%q) = %+v, want %+v", tt.in, got, tt.want)
			}
			var whole strings.Builder
			for i, p := range got {
				if p != tt.want[i] {
					t.Errorf("piece %d of linked(%q) = %+v, want %+v", i, tt.in, p, tt.want[i])
				}
				whole.WriteString(p.Text)
			}
			if whole.String() != tt.in {
				t.Errorf("the pieces of linked(%q) read %q: text was lost or added", tt.in, whole.String())
			}
		})
	}
}

func TestWebAddress(t *testing.T) {
	for addr, want := range map[string]bool{
		"https://example.com":                 true,
		"http://example.com/a?b=c#d":          true,
		"https://example.com/wiki/Etc.":       true,
		"":                                    false,
		"example.com":                         false,
		"/p/a/b/faq":                          false,
		"//example.com":                       false,
		"https://":                            false,
		"javascript:alert(1)":                 false,
		"javascript://example.com/%0Aalert()": false,
		"data:text/html,x":                    false,
		"ftp://example.com":                   false,
		"mailto:a@b.example":                  false,
		" https://example.com":                false,
		"https://exa mple.com":                false,
		"https://example.com/\x00":            false,
	} {
		if got := webAddress(addr); got != want {
			t.Errorf("NFR-SEC-6: webAddress(%q) = %v, want %v", addr, got, want)
		}
	}
}

func TestOwnPath(t *testing.T) {
	for path, want := range map[string]bool{
		"/":                   true,
		"/p/a/b/faq":          true,
		"":                    false,
		"//evil.example":      false,
		`/\evil.example`:      false, // a browser reads the backslash as a second slash
		"https://example.com": false,
		"javascript:alert(1)": false,
		"p/a/b":               false,
	} {
		if got := ownPath(path); got != want {
			t.Errorf("NFR-SEC-6: ownPath(%q) = %v, want %v", path, got, want)
		}
	}
}

func TestStart(t *testing.T) {
	for _, c := range []struct {
		name, in string
		limit    int
		want     string
	}{
		{"a short text is kept", "Use retryIf.", 200, "Use retryIf."},
		{"line breaks and runs of spaces become one space", "  Use retryIf.\n\n  Then\tcheck.  ", 200, "Use retryIf. Then check."},
		{"a long text is cut and says so", "one two three four", 9, "one two…"},
		{"characters are counted, not bytes", "ééééé", 5, "ééééé"},
		{"a cut never splits a character", "éééééé", 5, "éééé…"},
		{"nothing is nothing", " \n ", 10, ""},
	} {
		if got := start(c.in, c.limit); got != c.want {
			t.Errorf("%s: start(%q, %d) = %q, want %q", c.name, c.in, c.limit, got, c.want)
		}
	}
}

func TestEntryPage(t *testing.T) {
	t.Run("FR-FAQ-3: the question, its details, the answer, the links, the date and who answered", func(t *testing.T) {
		body := render(t, Entry{Site: testSite, FAQEntry: testEntry()})
		has(t, body,
			"<h1>How do I retry only on 5xx errors?</h1>",
			// The line breaks someone typed are kept; the stylesheet shows them.
			"I set retries: 3 and it retries on a 404 too.\nIs there a filter?</div>",
			"Use retryIf and check the status.\n\nSee <a href=\"https://example.com/docs/retry\" "+outsideRel+">https://example.com/docs/retry</a> for the options.</div>",
			`Answered live in office hours on <time datetime="2026-11-18T20:30:00Z">18 Nov 2026</time> by <span class="mono">@mia-k</span>`,
			`<a href="https://github.com/paperplane/stash/pull/41?a=1&amp;b=2" `+outsideRel+`>https://github.com/paperplane/stash/pull/41?a=1&amp;b=2</a>`,
			`<img class="av" src="https://avatars.example/1" alt=""`,
			`href="/p/PaperPlane/Stash/faq"`, // back to the list
			`<html lang="en">`,
		)
		has(t, styleOf(t, body), ".text{white-space:pre-wrap")
		checkAddresses(t, body)
	})

	t.Run("FR-HAND-8: more than one asker is a number, never names", func(t *testing.T) {
		has(t, render(t, Entry{Site: testSite, FAQEntry: testEntry()}), "asked by 3 people")
		alone := testEntry()
		alone.AskedBy = 1
		lacks(t, render(t, Entry{Site: testSite, FAQEntry: alone}), "asked by")
	})

	t.Run("FR-FAQ-5: the GitHub issue link carries the title and a body, and writes nothing itself", func(t *testing.T) {
		body := render(t, Entry{Site: testSite, FAQEntry: testEntry()})
		m := regexp.MustCompile(`<a class="btn ghost" href="(https://github\.com/[^"]*)" ` + outsideRel + `>(?:<svg[^>]*>.*?</svg>)?Open a GitHub issue</a>`).FindStringSubmatch(body)
		if m == nil {
			t.Fatalf("the page has no issue link:\n%s", body)
		}
		u, err := url.Parse(html.UnescapeString(m[1]))
		if err != nil {
			t.Fatalf("the issue link %q: %v", m[1], err)
		}
		if u.Scheme != "https" || u.Host != "github.com" || u.Path != "/PaperPlane/Stash/issues/new" {
			t.Errorf("the issue link goes to %s://%s%s, want GitHub's new-issue page of the repo", u.Scheme, u.Host, u.Path)
		}
		q := u.Query()
		if len(q) != 2 || q.Get("title") != "How do I retry only on 5xx errors?" {
			t.Errorf("query = %v, want a title and a body", q)
		}
		if got := q.Get("body"); !strings.HasPrefix(got, "I set retries: 3 and it retries on a 404 too.\nIs there a filter?\n\n") ||
			!strings.HasSuffix(got, "https://officehours.example/p/PaperPlane/Stash/faq/retry-only-on-5xx") {
			t.Errorf("body = %q, want the details and then the answer's address", got)
		}
	})

	t.Run("FR-FAQ-5: an issue for a question without details still points at the answer", func(t *testing.T) {
		bare := testEntry()
		bare.Question = ""
		body := Entry{Site: testSite, FAQEntry: bare}.issueBody()
		if strings.HasPrefix(body, "\n") || !strings.HasSuffix(body, "https://officehours.example/p/PaperPlane/Stash/faq/retry-only-on-5xx") {
			t.Errorf("body = %q", body)
		}
		long := testEntry()
		long.Question = strings.Repeat("é", 20000)
		if got := len([]rune(Entry{Site: testSite, FAQEntry: long}.issueBody())); got > issueBodyMax {
			t.Errorf("an issue body of %d characters, want at most %d: GitHub cuts an address that is too long", got, issueBodyMax)
		}
	})

	t.Run("FR-FAQ-3: the link preview shows the question and the start of the answer", func(t *testing.T) {
		body := render(t, Entry{Site: testSite, FAQEntry: testEntry()})
		has(t, body,
			"<title>How do I retry only on 5xx errors? · PaperPlane/Stash FAQ</title>",
			`<meta name="description" content="Use retryIf and check the status. See https://example.com/docs/retry for the options.">`,
			`<meta property="og:title" content="How do I retry only on 5xx errors?">`,
			`<meta property="og:description" content="Use retryIf and check the status. See https://example.com/docs/retry for the options.">`,
			`<meta property="og:type" content="article">`,
			`<meta property="og:url" content="https://officehours.example/p/PaperPlane/Stash/faq/retry-only-on-5xx">`,
			`<meta property="og:site_name" content="Office Hours">`,
			`<meta name="twitter:card" content="summary">`,
			`<link rel="canonical" href="https://officehours.example/p/PaperPlane/Stash/faq/retry-only-on-5xx">`,
		)
		lacks(t, body, "noindex")

		wordy := testEntry()
		wordy.Answer = strings.Repeat("word ", 100)
		m := regexp.MustCompile(`<meta property="og:description" content="([^"]*)">`).FindStringSubmatch(render(t, Entry{Site: testSite, FAQEntry: wordy}))
		if m == nil || len([]rune(m[1])) > 200 || !strings.HasSuffix(m[1], "…") {
			t.Errorf("the description of a long answer = %q, want at most 200 characters that end in an ellipsis", m)
		}
	})

	t.Run("FR-SESS-5: the next session is written in UTC, for a script to turn into the reader's time", func(t *testing.T) {
		next := time.Date(2026, 11, 19, 17, 0, 0, 0, time.FixedZone("IST", 5*3600+1800))
		body := render(t, Entry{Site: testSite, FAQEntry: testEntry(), NextSession: next})
		has(t, body, `<time datetime="2026-11-19T11:30:00Z" data-local>Thu 19 Nov 2026, 11:30 UTC</time>`, "<script>", "toLocaleString")
		from, to := strings.Index(body, "<script>"), strings.Index(body, "</script>")
		if from < 0 || to < from || strings.Count(body[from:to], "\n") > 4 {
			t.Errorf("the page needs one short script, not %q", body[max(from, 0):max(to, 0)])
		}
	})

	t.Run("FR-FAQ-3: without a planned session the page has no script at all", func(t *testing.T) {
		body := render(t, Entry{Site: testSite, FAQEntry: testEntry()})
		lacks(t, body, "<script", "data-local", "Next session")
		has(t, body, `<a class="btn primary" href="/p/PaperPlane/Stash">Join the room</a>`)
	})

	t.Run("FR-FAQ-3: an answer without details, links or a known author leaves those parts out", func(t *testing.T) {
		bare := testEntry()
		bare.Question, bare.Links, bare.Author = "", nil, store.Person{}
		body := render(t, Entry{Site: testSite, FAQEntry: bare})
		lacks(t, body, `class="details`, "Links</h2>", " by <span", "<img")
		has(t, body, `Answered live in office hours on <time datetime="2026-11-18T20:30:00Z">18 Nov 2026</time>`)
	})
}

// hostile is what someone who wants to break the page would type.
const hostile = `<script>alert("x")</script> <img src=x onerror=alert(1)> 'single' & </title></style></textarea><b>bold</b> javascript:alert(1) [click](javascript:alert(2)) <a href="javascript:alert(3)">y</a> {{.Origin}}`

func TestUserTextIsText(t *testing.T) {
	evil := testEntry()
	evil.Title, evil.Question, evil.Answer = "T "+hostile, "Q "+hostile, "A "+hostile
	evil.Links = []string{"javascript:alert(4)", "data:text/html,<script>alert(5)</script>", " https://spaced.example", "https://ok.example/path?a=1&b=<2>"}
	evil.Author = store.Person{Login: `<b>mia</b>`, Name: `<i>Mia</i>`, AvatarURL: "javascript:alert(6)"}
	escaped := `&lt;script&gt;alert(&#34;x&#34;)&lt;/script&gt; &lt;img src=x onerror=alert(1)&gt; &#39;single&#39; &amp; &lt;/title&gt;&lt;/style&gt;&lt;/textarea&gt;&lt;b&gt;bold&lt;/b&gt; javascript:alert(1) [click](javascript:alert(2)) &lt;a href=&#34;javascript:alert(3)&#34;&gt;y&lt;/a&gt; {{.Origin}}`

	pages := map[string]string{
		"the answer's page":   render(t, Entry{Site: testSite, FAQEntry: evil, NextSession: time.Date(2026, 11, 19, 11, 30, 0, 0, time.UTC)}),
		"the list":            render(t, List{Site: testSite, Entries: []store.FAQEntry{evil}}),
		"a search":            render(t, List{Site: testSite, Query: hostile, Entries: []store.FAQEntry{evil}}),
		"a search in vain":    render(t, List{Site: testSite, Query: hostile}),
		"a notice":            render(t, Notice{Site: testSite, Title: hostile, Text: hostile, Link: "javascript:alert(7)", LinkText: hostile}),
		"a notice of no repo": render(t, Notice{Title: hostile, Text: hostile}),
	}
	for name, body := range pages {
		t.Run("NFR-SEC-6: "+name, func(t *testing.T) {
			lacks(t, body,
				"<script>alert", "<img src=x", "<b>", "<i>", `href="javascript:`, `src="javascript:`, `href="data:text`, "onerror=alert(1)>",
				"</title></style>", "https://officehours.example{{", "alert(6)", "alert(7)",
			)
			checkAddresses(t, body)
			if strings.Count(body, "<title>") != 1 || strings.Count(body, "</title>") != 1 || strings.Count(body, "</style>") != 1 {
				t.Error("the page has more than one title or style element: text broke out of its place")
			}
		})
	}

	t.Run("NFR-SEC-6: the question, the details and the answer arrive whole, as text", func(t *testing.T) {
		body := pages["the answer's page"]
		has(t, body, "<h1>T "+escaped+"</h1>", "Q "+escaped+"</div>", "A "+escaped+"</div>")
		// The addresses nobody may follow are shown as the text they are.
		has(t, body, "<li>javascript:alert(4)</li>", "<li>data:text/html,&lt;script&gt;alert(5)&lt;/script&gt;</li>", "<li> https://spaced.example</li>")
		has(t, body, `<a href="https://ok.example/path?a=1&amp;b=%3c2%3e" `+outsideRel+`>https://ok.example/path?a=1&amp;b=&lt;2&gt;</a>`)
		has(t, body, `@&lt;b&gt;mia&lt;/b&gt;`)
	})

	t.Run("NFR-SEC-6: what was searched is shown as text and kept in the field", func(t *testing.T) {
		body := pages["a search in vain"]
		has(t, body, `value="`+escaped+`"`, "No answer matches “"+escaped+"”")
	})
}

func TestListPage(t *testing.T) {
	older := testEntry()
	older.ID, older.Slug, older.Title, older.Answer = 6, "where-is-the-cache", "Where is the cache stored?", "On disk,\nunder ~/.stash,   unless you pass a directory."
	older.Author = store.Person{UserID: "gh2", Login: "diego-ruiz", Name: "diego-ruiz"}
	two := []store.FAQEntry{testEntry(), older}

	t.Run("FR-FAQ-3: the repo, how many answers, and each answer as a card that links to its page", func(t *testing.T) {
		body := render(t, List{Site: testSite, Entries: two})
		has(t, body,
			"PaperPlane/Stash · FAQ · [ 2 answers ]",
			`<a class="card" href="/p/PaperPlane/Stash/faq/retry-only-on-5xx">`,
			`<span class="q">How do I retry only on 5xx errors?</span>`,
			// The start of the answer, on one line.
			`<span class="a">Use retryIf and check the status. See https://example.com/docs/retry for the options.</span>`,
			`<time datetime="2026-11-18T20:30:00Z">18 Nov 2026</time>`,
			"@mia-k",
			`<a class="card" href="/p/PaperPlane/Stash/faq/where-is-the-cache">`,
			`<span class="a">On disk, under ~/.stash, unless you pass a directory.</span>`,
			"@diego-ruiz",
			`href="/p/PaperPlane/Stash">Join the room</a>`,
		)
		if strings.Index(body, "retry-only-on-5xx") > strings.Index(body, "where-is-the-cache") {
			t.Error("the cards are not in the order the store gave")
		}
		lacks(t, body, "<script", "No answers yet", "No answer matches")
		checkAddresses(t, body)
	})

	t.Run("FR-FAQ-3: one answer is counted as one", func(t *testing.T) {
		has(t, render(t, List{Site: testSite, Entries: two[:1]}), "[ 1 answer ]")
	})

	t.Run("FR-FAQ-3: the search is a plain GET form that needs no script", func(t *testing.T) {
		body := render(t, List{Site: testSite, Entries: two})
		form := regexp.MustCompile(`(?s)<form[^>]*>.*?</form>`).FindString(body)
		has(t, form, `method="get"`, `action="/p/PaperPlane/Stash/faq"`, `role="search"`, `type="search"`, `name="q"`, `maxlength="100"`, `value=""`, `type="submit"`)
	})

	t.Run("FR-FAQ-3: a search keeps what was searched, and says how many answers match", func(t *testing.T) {
		body := render(t, List{Site: testSite, Query: `retry "5xx"`, Entries: two[:1]})
		has(t, body, `value="retry &#34;5xx&#34;"`, "1 answer for “retry &#34;5xx&#34;”", `<a href="/p/PaperPlane/Stash/faq">Show all answers</a>`)
		lacks(t, body, "[ 1 answer ]") // the header counts the whole FAQ, which a search does not know
	})

	t.Run("FR-FAQ-3: a search that finds nothing says so, and offers the room and the whole list", func(t *testing.T) {
		body := render(t, List{Site: testSite, Query: "kubernetes"})
		has(t, body, "No answer matches “kubernetes”", `value="kubernetes"`, `href="/p/PaperPlane/Stash">Join the room</a>`, `<a href="/p/PaperPlane/Stash/faq">Show all answers</a>`)
		lacks(t, body, `class="card"`, "No answers yet")
	})

	t.Run("FR-FAQ-3: a project without answers says so, and offers the room", func(t *testing.T) {
		body := render(t, List{Site: testSite})
		has(t, body, "No answers yet", `href="/p/PaperPlane/Stash">Join the room</a>`, "PaperPlane/Stash · FAQ")
		lacks(t, body, `class="card"`, "<form", "No answer matches", "answers ]")
	})

	t.Run("FR-FAQ-3: a list that was cut says that it shows the newest", func(t *testing.T) {
		body := render(t, List{Site: testSite, Entries: two, Cut: true})
		has(t, body, "the newest 2 answers")
		lacks(t, body, "[ 2 answers ]")
		lacks(t, render(t, List{Site: testSite, Entries: two}), "the newest")
	})

	t.Run("FR-FAQ-3: the link preview names the repo", func(t *testing.T) {
		body := render(t, List{Site: testSite, Query: "retry", Entries: two})
		has(t, body,
			"<title>PaperPlane/Stash FAQ · Office Hours</title>",
			`<meta property="og:title" content="PaperPlane/Stash FAQ">`,
			`<meta property="og:type" content="website">`,
			// The address of the list, whatever was searched.
			`<meta property="og:url" content="https://officehours.example/p/PaperPlane/Stash/faq">`,
			`<meta name="twitter:card" content="summary">`,
		)
		for _, tag := range []string{`<meta name="description" content="`, `<meta property="og:description" content="`} {
			if !regexp.MustCompile(regexp.QuoteMeta(tag) + `[^"]*PaperPlane/Stash[^"]*">`).MatchString(body) {
				t.Errorf("no %s…\"> that names the repo", tag)
			}
		}
	})
}

func TestNoticePage(t *testing.T) {
	t.Run("FR-FAQ-6: a notice about a project says what happened and where to go", func(t *testing.T) {
		body := render(t, Notice{Site: testSite, Title: "This answer was removed", Text: "A maintainer took it down.", Link: "/p/PaperPlane/Stash/faq", LinkText: "Search answers"})
		has(t, body,
			"<title>This answer was removed · Office Hours</title>",
			"<h1>This answer was removed</h1>", "A maintainer took it down.",
			`href="/p/PaperPlane/Stash/faq">Search answers</a>`,
			`href="/p/PaperPlane/Stash">Join the room</a>`,
			`<meta name="robots" content="noindex">`,
		)
		lacks(t, body, "og:", "canonical", "<script")
		checkAddresses(t, body)
	})

	t.Run("a notice about no project has no room to offer", func(t *testing.T) {
		body := render(t, Notice{Title: "There is no page here", Text: "The address may be mistyped."})
		has(t, body, "<h1>There is no page here</h1>", `href="/"`)
		lacks(t, body, "Join the room", "github.com", "<script", "//faq")
		checkAddresses(t, body)
	})
}

// styleOf is the page's one stylesheet, without spaces, so values can be compared.
func styleOf(t *testing.T, body string) string {
	t.Helper()
	from, to := strings.Index(body, "<style>"), strings.Index(body, "</style>")
	if from < 0 || to < from {
		t.Fatal("the page has no inline style")
	}
	return strings.ToLower(strings.Join(strings.Fields(body[from:to]), ""))
}

func TestLook(t *testing.T) {
	body := render(t, List{Site: testSite, Entries: []store.FAQEntry{testEntry()}})
	style := styleOf(t, body)

	t.Run("the fonts come from the same Google Fonts link as the web app", func(t *testing.T) {
		index, err := os.ReadFile("../../web/index.html")
		if err != nil {
			t.Fatalf("read the web app's page: %v", err)
		}
		fonts := regexp.MustCompile(`https://fonts\.googleapis\.com/css2[^"]*`).FindString(string(index))
		if fonts == "" {
			t.Fatal("web/index.html has no Google Fonts link")
		}
		has(t, body, `href="`+html.EscapeString(html.UnescapeString(fonts))+`"`,
			`<link rel="preconnect" href="https://fonts.googleapis.com">`, `<link rel="preconnect" href="https://fonts.gstatic.com" crossorigin>`)
		has(t, style, `"instrumentsans"`, `"instrumentserif"`, `"geistmono"`)
	})

	t.Run("paper, ink and the one accent are the web app's tokens", func(t *testing.T) {
		css, err := os.ReadFile("../../web/src/styles/tokens.css")
		if err != nil {
			t.Fatalf("read the web app's tokens: %v", err)
		}
		tokens := strings.ToLower(strings.Join(strings.Fields(string(css)), ""))
		for _, name := range []string{"--paper", "--surface", "--sunken", "--ink", "--muted", "--hairline", "--blue", "--blue-hover", "--blue-ink"} {
			value := regexp.MustCompile(name + `:(#[0-9a-f]{6});`).FindStringSubmatch(tokens)
			if value == nil {
				t.Fatalf("web/src/styles/tokens.css has no %s", name)
			}
			if !strings.Contains(style, name+":"+value[1]+";") {
				t.Errorf("the page's %s is not the web app's %s", name, value[1])
			}
		}
	})

	t.Run("dark follows the system setting, with the dark column of docs/UI.md", func(t *testing.T) {
		at := strings.Index(style, "@media(prefers-color-scheme:dark){:root{")
		if at < 0 {
			t.Fatal("the style has no dark tokens for prefers-color-scheme: dark")
		}
		dark := style[at : at+strings.Index(style[at:], "}}")]
		for _, token := range []string{"--paper:#0e0f13;", "--ink:#ecebe6;", "--muted:#9a9ca6;", "--hairline:#26282f;", "--blue:#7a9bff;", "--blue-ink:#0e0f13;"} {
			if !strings.Contains(dark, token) {
				t.Errorf("the dark tokens lack %s", token)
			}
		}
		has(t, body, `<meta name="color-scheme" content="light dark">`, `<meta name="viewport" content="width=device-width, initial-scale=1">`)
	})

	t.Run("NFR-COMP-1: long words wrap, and nothing has a fixed width a phone cannot hold", func(t *testing.T) {
		has(t, style, "overflow-wrap:anywhere")
		for _, m := range regexp.MustCompile(`[^-]width:(\d+)px`).FindAllStringSubmatch(style, -1) {
			// 375 px less the 16 px each side that the page keeps free on a phone.
			if px, _ := strconv.Atoi(m[1]); px > 343 {
				t.Errorf("the style sets a width of %d px: wider than a 375 px phone leaves room for", px)
			}
		}
	})
}
