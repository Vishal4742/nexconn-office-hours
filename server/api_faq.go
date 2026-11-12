package main

import "net/http"

// The FAQ as JSON, for the room's pages. Owned by the FAQ-and-stats API part of the parallel
// build (notes/phase-2-plan.md, worker 8); the comments below are what each route must do. The
// shapes (faqJSON, describeFAQs) and the store interface (faqStore) are in shapes.go.

// faqSimilar is POST /api/p/{owner}/{repo}/faq/similar {text}: up to 3 entries that look like a
// question someone is typing, the best match first (FR-HAND-3, FR-FAQ-4). A POST with the text
// in its body, so a draft question is never part of an address. For anyone signed in
// (guard.Mutation; the FAQ itself is public). text is at most 2,200 characters; fewer than 3
// characters after trimming answers an empty list without asking the store.
func (d deps) faqSimilar(w http.ResponseWriter, r *http.Request) { notBuilt(w) }

// faqLatest is GET /api/p/{owner}/{repo}/faq/latest: the project's 5 newest entries, for the
// join page (FR-PROJ-3). Public.
func (d deps) faqLatest(w http.ResponseWriter, r *http.Request) { notBuilt(w) }

// deleteFAQ is POST /api/faq/{id}/delete: a maintainer of the entry's project deletes it, and
// its page answers 410 from then on (FR-FAQ-6). guard.Mutation, faqStore.ProjectOfFAQ, then
// guard.Maintainer. Deleting an entry that is already deleted is fine. The answer is {"id"}.
func (d deps) deleteFAQ(w http.ResponseWriter, r *http.Request) { notBuilt(w) }
