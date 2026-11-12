package main

import (
	"fmt"
	"html"
	"net/http"
)

// The public FAQ pages, rendered by Go (FR-FAQ-3). Owned by the FAQ-pages part of the parallel
// build (notes/phase-2-plan.md, worker 7), which replaces the placeholder below with
// html/template pages from server/pages.
//
// faqListPage is GET /p/{owner}/{repo}/faq, with an optional ?q= search: the project's entries,
// readable signed out and without JavaScript. faqEntryPage is GET /p/{owner}/{repo}/faq/{slug}:
// one answer with link-preview metadata; a deleted entry answers 410 "removed" (FR-FAQ-6), an
// unknown one 404.

func (d deps) faqListPage(w http.ResponseWriter, r *http.Request) { faqPlaceholder(w, r) }

func (d deps) faqEntryPage(w http.ResponseWriter, r *http.Request) { faqPlaceholder(w, r) }

// faqPlaceholder shows that these routes reach the Go server, until the real pages replace it.
func faqPlaceholder(w http.ResponseWriter, r *http.Request) {
	owner, repo, slug := r.PathValue("owner"), r.PathValue("repo"), r.PathValue("slug")
	if !validOwnerRepo(owner, repo) || (slug != "" && !slugRe.MatchString(slug)) {
		http.NotFound(w, r)
		return
	}
	what := "FAQ list"
	if slug != "" {
		what = "FAQ entry: " + slug
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprintf(w, `<!doctype html><html lang="en"><meta charset="utf-8">`+
		`<meta name="viewport" content="width=device-width, initial-scale=1">`+
		`<title>%[1]s FAQ · Office Hours</title><h1>%[1]s</h1><p>%[2]s</p>`+
		`<p>The public FAQ pages arrive in P3. This page shows the route reaches the Go server.</p>`,
		html.EscapeString(owner+"/"+repo), html.EscapeString(what))
}
