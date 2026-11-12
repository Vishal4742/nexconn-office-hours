package main

import (
	"context"
	"time"

	"officehours/store"
)

// Store interfaces and JSON shapes that more than one part of the parallel build of Phase 2
// reads. This file is a contract (notes/phase-2-plan.md): the routes and the pages are built
// against it at the same time, so it changes only through the plan.

// sessionStore is the part of the store only the session routes use (api_sessions.go).
type sessionStore interface {
	StartSession(ctx context.Context, n store.NewSession) (store.Session, error)
	SetNextSession(ctx context.Context, projectID string, at time.Time) error
	RemoveMaintainer(ctx context.Context, projectID, userID string) error
}

// faqStore is the part of the store the FAQ routes (api_faq.go) and the public FAQ pages
// (faq_pages.go) use.
type faqStore interface {
	FAQList(ctx context.Context, projectID, query string, limit int) ([]store.FAQEntry, error)
	FAQBySlug(ctx context.Context, projectID, slug string) (store.FAQEntry, bool, error)
	FAQSuggest(ctx context.Context, projectID, text string, limit int) ([]store.FAQEntry, error)
	ProjectOfFAQ(ctx context.Context, id int64) (projectID string, found bool, err error)
	DeleteFAQ(ctx context.Context, id int64) error
}

// statsStore is the part of the store the stats routes use (api_stats.go).
type statsStore interface {
	Stats(ctx context.Context, projectID string) (store.Stats, error)
}

// faqJSON is one FAQ entry as the API describes it. web/src/faq.ts is the same shape. It carries
// the slug and no address: the page builds the link on our own origin (NFR-SEC-6).
type faqJSON struct {
	ID        string    `json:"id"`
	Slug      string    `json:"slug"`
	Title     string    `json:"title"`
	Question  string    `json:"question"`
	Answer    string    `json:"answer"`
	Links     []string  `json:"links"`
	AskedBy   int       `json:"askedBy"` // a number only: who asked is not public (FR-HAND-8)
	CreatedAt time.Time `json:"createdAt"`
	Author    string    `json:"author"` // the login of the maintainer who answered
}

func describeFAQ(e store.FAQEntry) faqJSON {
	links := e.Links
	if links == nil {
		links = []string{}
	}
	return faqJSON{ID: idText(e.ID), Slug: e.Slug, Title: e.Title, Question: e.Question, Answer: e.Answer,
		Links: links, AskedBy: e.AskedBy, CreatedAt: e.CreatedAt, Author: e.Author.Login}
}

func describeFAQs(entries []store.FAQEntry) []faqJSON {
	out := make([]faqJSON, 0, len(entries)) // an empty list is [] in JSON, never null
	for _, e := range entries {
		out = append(out, describeFAQ(e))
	}
	return out
}

// statsJSON is the five numbers of FR-MET-1. web/src/stats.ts is the same shape.
type statsJSON struct {
	Sessions      int `json:"sessions"`
	HandsRaised   int `json:"handsRaised"`
	HandsAnswered int `json:"handsAnswered"`
	FAQEntries    int `json:"faqEntries"`
	Participants  int `json:"participants"`
}
