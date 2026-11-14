package main

import (
	"bytes"
	"context"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"officehours/pages"
	"officehours/store"
)

// The public FAQ pages, rendered from server/pages (FR-FAQ-3), and their versions for agents: an
// answer as Markdown, a project's answers as its llms.txt (FR-FAQ-7). They are the same for
// everyone, so they never ask who is signed in, and a cache may keep one if it asks again first; a
// failure is never kept. An owner, repo or slug that no project or answer can have is a 404
// before anything is looked up (NFR-SEC-3).

const (
	// faqPageListMax bounds the list. ponytail: no paging. Past this the page shows the newest and
	// says so, and search reaches the rest; page it when a project gets there.
	faqPageListMax = 200
	// faqPageQueryMax is the longest search, in characters. The search field takes no more.
	faqPageQueryMax = 100
)

// faqListPage is GET /p/{owner}/{repo}/faq, with an optional ?q= search: the project's answers,
// the newest first, or the ones that match the search, the best first.
func (d deps) faqListPage(w http.ResponseWriter, r *http.Request) {
	site, project, ok := d.faqPageProject(w, r)
	if !ok {
		return
	}
	list, err := d.faqPageList(r.Context(), site, project.ID, clip(cleanLine(r.URL.Query().Get("q")), faqPageQueryMax))
	if err != nil {
		faqPageFailed(w, r, err)
		return
	}
	faqPageWrite(w, http.StatusOK, list)
}

// faqLLMsTxt is GET /p/{owner}/{repo}/llms.txt (FR-FAQ-7): the project's answers for agents, the
// newest first, each linked to its Markdown (llmstxt.org).
func (d deps) faqLLMsTxt(w http.ResponseWriter, r *http.Request) {
	site, project, ok := d.faqPageProject(w, r)
	if !ok {
		return
	}
	list, err := d.faqPageList(r.Context(), site, project.ID, "")
	if err != nil {
		faqPageFailed(w, r, err)
		return
	}
	faqTextWrite(w, "text/plain; charset=utf-8", list.LLMsTxt())
}

// faqPageList is the project's answers that match query, or the newest without one: at most
// faqPageListMax of them, and whether there were more.
func (d deps) faqPageList(ctx context.Context, site pages.Site, projectID, query string) (pages.List, error) {
	entries, err := d.faq.FAQList(ctx, projectID, query, faqPageListMax+1)
	if err != nil {
		return pages.List{}, err
	}
	cut := len(entries) > faqPageListMax
	if cut {
		entries = entries[:faqPageListMax]
	}
	return pages.List{Site: site, Query: query, Entries: entries, Cut: cut}, nil
}

// faqEntryPage is GET /p/{owner}/{repo}/faq/{slug}: one answer, with link-preview metadata, or
// with ".md" after the slug the same answer as Markdown for agents (FR-FAQ-7). A deleted answer
// is 410 (FR-FAQ-6), one nobody wrote 404.
func (d deps) faqEntryPage(w http.ResponseWriter, r *http.Request) {
	slug, isMarkdown := strings.CutSuffix(r.PathValue("slug"), ".md")
	if !slugRe.MatchString(slug) {
		faqPageMissing(w)
		return
	}
	site, project, ok := d.faqPageProject(w, r)
	if !ok {
		return
	}
	entry, found, err := d.faq.FAQBySlug(r.Context(), project.ID, slug)
	switch {
	case err != nil:
		faqPageFailed(w, r, err)
	case !found:
		faqPageWrite(w, http.StatusNotFound, pages.Notice{Site: site, Title: "There is no answer here",
			Text: "The link may be mistyped. The other answers of " + site.Owner + "/" + site.Repo + " are a search away.",
			Link: site.ListPath(), LinkText: "Search answers"})
	case entry.Deleted:
		faqPageWrite(w, http.StatusGone, pages.Notice{Site: site, Title: "This answer was removed",
			Text: "A maintainer took it down. Search the other answers, or ask your question live in the room.",
			Link: site.ListPath(), LinkText: "Search answers"})
	case isMarkdown:
		faqTextWrite(w, "text/markdown; charset=utf-8", pages.Entry{Site: site, FAQEntry: entry}.Markdown())
	default:
		faqPageWrite(w, http.StatusOK, pages.Entry{Site: site, FAQEntry: entry, NextSession: d.faqPageNextSession(r.Context(), project)})
	}
}

// faqPageProject finds the project a FAQ page is about. When there is none to return it has
// answered with a page itself.
func (d deps) faqPageProject(w http.ResponseWriter, r *http.Request) (pages.Site, store.Project, bool) {
	owner, repo := r.PathValue("owner"), r.PathValue("repo")
	if !validOwnerRepo(owner, repo) {
		faqPageMissing(w)
		return pages.Site{}, store.Project{}, false
	}
	if d.projects == nil || d.faq == nil {
		faqPageWrite(w, http.StatusServiceUnavailable, pages.Notice{Title: "The FAQ isn't set up on this server yet",
			Text: "This server has no database to read answers from. Try again later."})
		return pages.Site{}, store.Project{}, false
	}
	// Asked for nobody: what someone is in the project doesn't change the page.
	project, found, err := d.projects.ProjectByRepo(r.Context(), owner, repo, "")
	if err != nil {
		faqPageFailed(w, r, err)
		return pages.Site{}, store.Project{}, false
	}
	if !found {
		faqPageWrite(w, http.StatusNotFound, pages.Notice{Title: "Nobody hosts office hours for " + owner + "/" + repo + " yet",
			Text: "The address may be mistyped, or the repository's maintainers haven't opened a room here.",
			Link: "/", LinkText: "Back to the start"})
		return pages.Site{}, store.Project{}, false
	}
	// The name the project was registered under and our own origin, never the request's spelling
	// or its Host: these go into link previews and issues.
	return pages.Site{Origin: d.guard.Origin(), Owner: project.Owner, Repo: project.Repo}, project, true
}

// faqPageNextSession is when the project's next session is planned (FR-SESS-5), or zero: none
// is, it has passed, or the queue couldn't say. The page is whole without it.
func (d deps) faqPageNextSession(ctx context.Context, project store.Project) time.Time {
	if d.queue == nil {
		return time.Time{}
	}
	snap, err := d.queue.Queue(ctx, project.ID)
	if err != nil {
		log.Printf("faq page %s/%s: next session: %v", project.Owner, project.Repo, err)
		return time.Time{}
	}
	if !snap.NextSessionAt.After(now()) {
		return time.Time{}
	}
	return snap.NextSessionAt
}

// faqPageMissing answers an address no project or answer can have.
func faqPageMissing(w http.ResponseWriter) {
	faqPageWrite(w, http.StatusNotFound, pages.Notice{Title: "There is no page here",
		Text: "The address may be mistyped. A project's answers live at /p/owner/repo/faq.",
		Link: "/", LinkText: "Back to the start"})
}

// faqPageFailed answers a failure of ours. What went wrong goes to the log, never to the page.
func faqPageFailed(w http.ResponseWriter, r *http.Request, err error) {
	log.Printf("faq page %s: %v", r.URL.Path, err)
	faqPageWrite(w, http.StatusInternalServerError, pages.Notice{Title: "Something went wrong on our side",
		Text: "We couldn't read the answers just now. Try again in a moment."})
}

// faqPageWrite writes a public page. A cache must ask again before it shows one (no-cache), so an
// answer a maintainer deleted is gone at once; a failure is never kept at all.
func faqPageWrite(w http.ResponseWriter, status int, page interface{ Render(io.Writer) error }) {
	cache := "public, no-cache"
	if status >= http.StatusInternalServerError {
		cache = "no-store"
	}
	writePage(w, status, page, cache)
}

// writePage renders a page whole before it answers, so a template that fails is a 500 and never
// half a page.
func writePage(w http.ResponseWriter, status int, page interface{ Render(io.Writer) error }, cache string) {
	var body bytes.Buffer
	if err := page.Render(&body); err != nil {
		log.Printf("page: render: %v", err)
		w.Header().Set("Cache-Control", "no-store")
		http.Error(w, "Something went wrong on our side. Try again in a moment.", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", cache)
	w.WriteHeader(status)
	if _, err := body.WriteTo(w); err != nil {
		log.Printf("page: write: %v", err)
	}
}

// faqTextWrite answers with text for agents: Markdown or an llms.txt. A cache must ask again
// first, as for the pages, and nosniff keeps a browser from reading the typed text in it as a page.
func faqTextWrite(w http.ResponseWriter, contentType, text string) {
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "public, no-cache")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if _, err := io.WriteString(w, text); err != nil {
		log.Printf("faq text: write: %v", err)
	}
}
