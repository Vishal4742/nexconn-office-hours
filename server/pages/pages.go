// Package pages renders the public FAQ pages (FR-FAQ-3) with html/template: readable signed out
// and without JavaScript, and the same for everyone who opens them, so a cache may keep them.
// What people typed is text on these pages (text.go). The style is inline in layout.html, since
// the web app's hashed CSS can't be reached from Go; its tokens and fonts are the web app's.
package pages

import (
	"embed"
	"html/template"
	"io"
	"net/url"
	"time"

	"officehours/store"
)

//go:embed *.html
var files embed.FS

// dayLayout writes a day the way every page and Markdown version does: "18 Nov 2026".
const dayLayout = "2 Jan 2006"

var funcs = template.FuncMap{
	"linked":     linked,
	"webAddress": webAddress,
	"start":      start,
	"count":      count,
	"iso":        func(t time.Time) string { return t.UTC().Format(time.RFC3339) },
	"day":        func(t time.Time) string { return t.UTC().Format(dayLayout) },
	"when":       func(t time.Time) string { return t.UTC().Format("Mon 2 Jan 2006, 15:04 UTC") },
}

// parse joins a page to the layout they all share. A template that doesn't parse stops the
// server at start, not a visitor later.
func parse(page string) *template.Template {
	return template.Must(template.New(page).Funcs(funcs).ParseFS(files, "layout.html", page))
}

var (
	listPage    = parse("list.html")
	entryPage   = parse("entry.html")
	noticePage  = parse("notice.html")
	consentPage = parse("consent.html")
)

// Consent is the page where a person lets an agent's app act as them, or says no (FR-AGENT-2).
type Consent struct {
	Site                  // zero: the page is about no project
	App            string // what the app called itself: text, not proof
	ReturnsTo      string // the host it sends the person back to
	OnYourComputer bool   // that host is this computer, where any app could have started this
	Login          string // who the app will act as
	AvatarURL      string
	Form           string // the sealed request the buttons send back
}

// Render writes the page.
func (c Consent) Render(w io.Writer) error { return consentPage.ExecuteTemplate(w, "page", c) }

// Site is the project a page is about: its name as it was registered, and our own origin, which
// link previews and issues point at (never the Host a request named). A zero Site is no project.
type Site struct {
	Origin string // APP_URL's origin, like https://officehours.example
	Owner  string
	Repo   string
}

// RoomPath is the project's room, where its sessions happen.
func (s Site) RoomPath() string { return "/p/" + s.Owner + "/" + s.Repo }

// ListPath is the project's FAQ.
func (s Site) ListPath() string { return s.RoomPath() + "/faq" }

// CalendarPath is the project's sessions as an .ics feed (FR-SESS-5).
func (s Site) CalendarPath() string { return s.RoomPath() + "/calendar.ics" }

// ListURL is ListPath on our own origin, for link previews.
func (s Site) ListURL() string { return s.Origin + s.ListPath() }

// EntryPath is one answer's page.
func (s Site) EntryPath(slug string) string { return s.ListPath() + "/" + slug }

// MarkdownURL is one answer as Markdown, for agents (FR-FAQ-7).
func (s Site) MarkdownURL(slug string) string { return s.Origin + s.EntryPath(slug) + ".md" }

// LLMsPath is the project's answers as an llms.txt, for agents (FR-FAQ-7).
func (s Site) LLMsPath() string { return s.RoomPath() + "/llms.txt" }

// List is the page of a project's answers, or of those that match a search.
type List struct {
	Site
	Query   string           // what was searched, already cleaned; "" lists the newest
	Entries []store.FAQEntry // in the order to show them
	Cut     bool             // there are more than Entries: the page says it shows a part
}

// Render writes the page.
func (l List) Render(w io.Writer) error { return listPage.ExecuteTemplate(w, "page", l) }

// Entry is the page of one answer.
type Entry struct {
	Site
	store.FAQEntry
	NextSession time.Time // when the project's next session is planned; zero leaves it out (FR-SESS-5)
}

// Render writes the page.
func (e Entry) Render(w io.Writer) error { return entryPage.ExecuteTemplate(w, "page", e) }

// URL is the answer's own address.
func (e Entry) URL() string { return e.Origin + e.EntryPath(e.Slug) }

// LD is the answer as schema.org data for search engines and agents (FR-FAQ-7): an FAQPage of
// its one question, since only maintainers answer here. The template writes it as JSON inside its
// script, escaped there. Who asked is not named, as on the page (FR-HAND-8).
func (e Entry) LD() map[string]any {
	answer := map[string]any{
		"@type":         "Answer",
		"text":          e.Answer,
		"url":           e.URL(),
		"datePublished": e.CreatedAt.UTC().Format(time.RFC3339),
	}
	if e.Author.Login != "" {
		answer["author"] = map[string]any{"@type": "Person", "name": e.Author.Login, "url": "https://github.com/" + url.PathEscape(e.Author.Login)}
	}
	question := map[string]any{"@type": "Question", "name": e.Title, "acceptedAnswer": answer}
	if e.Question != "" {
		question["text"] = e.Question
	}
	return map[string]any{"@context": "https://schema.org", "@type": "FAQPage", "url": e.URL(), "mainEntity": []any{question}}
}

// issueBodyMax is the longest body an issue link carries, counted as it is written in the
// address, as in web/src/issueLink.ts: GitHub turns away an address that is too long, and a
// letter outside ASCII takes up to 12 characters there.
const issueBodyMax = 4000

// IssueURL is GitHub's form for a new issue in the project's repository, filled in with the
// question (FR-FAQ-5). The reader still presses GitHub's own button: nothing is written for them.
func (e Entry) IssueURL() string {
	query := url.Values{"title": {e.Title}, "body": {e.issueBody()}}
	return "https://github.com/" + url.PathEscape(e.Owner) + "/" + url.PathEscape(e.Repo) + "/issues/new?" + query.Encode()
}

// issueBody is the question's details, cut to fit, then where the answer is, so whoever reads
// the issue sees what was answered already.
func (e Entry) issueBody() string {
	link := "Answered in office hours: " + e.URL()
	if e.Question == "" {
		return link
	}
	details, cut := []rune(e.Question), false
	body := func() string {
		if cut {
			return string(details) + "…\n\n" + link
		}
		return string(details) + "\n\n" + link
	}
	for len(details) > 0 && len(url.QueryEscape(body())) > issueBodyMax {
		details, cut = details[:len(details)*9/10], true // a tenth at a time: a few rounds at most
	}
	if len(details) == 0 {
		return link
	}
	return body()
}

// Notice is a page that says why there is no page: an unknown or removed answer, a server that
// can't answer. It is never indexed.
type Notice struct {
	Site           // zero when the page is about no project we know
	Title, Text    string
	Link, LinkText string // a path on our own origin, and what the button says; "" for none
}

// Render writes the page. A Link that leaves the site is left out.
func (n Notice) Render(w io.Writer) error {
	if !ownPath(n.Link) {
		n.Link = ""
	}
	return noticePage.ExecuteTemplate(w, "page", n)
}
