package main

import (
	"net/http"

	"officehours/store"
)

// The FAQ as JSON, for the room's pages. The public FAQ pages themselves are faq_pages.go. The
// shapes (faqJSON, describeFAQs) and the store interface (faqStore) are in shapes.go.

const (
	faqAPINotSetUp = "The FAQ isn't set up on this server yet."

	// What the similar-questions search reads: a hand's title (120 characters) and its details
	// (2,000) fit with room for what joins them, and one or two characters are not a question yet.
	faqAPIMinText = 3
	faqAPIMaxText = 2200
	faqAPITooLong = "That is too long to look for similar answers. Keep it to 2,200 characters."

	faqAPISimilarLimit = 3 // FR-FAQ-4
	faqAPILatestLimit  = 5 // FR-PROJ-3
)

// faqAPIAnswer answers with the entries the store found, or with the store's failure.
func faqAPIAnswer(w http.ResponseWriter, entries []store.FAQEntry, err error) {
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, envelope{OK: true, Data: describeFAQs(entries)})
}

// faqSimilar is POST /api/p/{owner}/{repo}/faq/similar {text}: up to 3 entries that look like a
// question someone is typing, the best match first (FR-HAND-3, FR-FAQ-4). A POST with the text
// in its body, so a draft question is never part of an address. For anyone signed in: the FAQ is
// public, so the answer does not depend on who asks. The text is read as one line of at most
// 2,200 characters; fewer than 3 answer an empty list without asking the store.
func (d deps) faqSimilar(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Text string `json:"text"`
	}
	if _, err := d.guard.Mutation(w, r, &in); err != nil {
		writeError(w, err)
		return
	}
	if d.faq == nil {
		writeJSON(w, http.StatusServiceUnavailable, envelope{Error: faqAPINotSetUp})
		return
	}
	project, ok := d.projectOfPath(w, r, "")
	if !ok {
		return
	}
	text := cleanLine(in.Text)
	if !lengthBetween(text, 0, faqAPIMaxText) {
		writeJSON(w, http.StatusBadRequest, envelope{Error: faqAPITooLong})
		return
	}
	if !lengthBetween(text, faqAPIMinText, faqAPIMaxText) {
		faqAPIAnswer(w, nil, nil) // not a question yet: nothing is alike
		return
	}
	entries, err := d.faq.FAQSuggest(r.Context(), project.ID, text, faqAPISimilarLimit)
	faqAPIAnswer(w, entries, err)
}

// faqLatest is GET /api/p/{owner}/{repo}/faq/latest: the project's 5 newest entries, for the
// join page (FR-PROJ-3). Public.
func (d deps) faqLatest(w http.ResponseWriter, r *http.Request) {
	if d.faq == nil {
		writeJSON(w, http.StatusServiceUnavailable, envelope{Error: faqAPINotSetUp})
		return
	}
	project, ok := d.projectOfPath(w, r, "")
	if !ok {
		return
	}
	entries, err := d.faq.FAQList(r.Context(), project.ID, "", faqAPILatestLimit)
	faqAPIAnswer(w, entries, err)
}

// deleteFAQ is POST /api/faq/{id}/delete: a maintainer of the entry's project deletes it, and
// its page answers 410 from then on (FR-FAQ-6). The entry says which project it belongs to, and
// the maintainers table says whether the caller maintains that one (NFR-SEC-2). Deleting an
// entry that is already deleted is fine. The answer is {"id"}.
func (d deps) deleteFAQ(w http.ResponseWriter, r *http.Request) {
	u, err := d.guard.Mutation(w, r, nil)
	if err != nil {
		writeError(w, err)
		return
	}
	if d.faq == nil {
		writeJSON(w, http.StatusServiceUnavailable, envelope{Error: faqAPINotSetUp})
		return
	}
	const noSuchEntry = "There is no such FAQ entry."
	id, ok := pathID(r)
	if !ok {
		writeJSON(w, http.StatusNotFound, envelope{Error: noSuchEntry})
		return
	}
	projectID, found, err := d.faq.ProjectOfFAQ(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	if !found {
		writeJSON(w, http.StatusNotFound, envelope{Error: noSuchEntry})
		return
	}
	if err := d.guard.Maintainer(r.Context(), u, projectID); err != nil {
		writeError(w, err)
		return
	}
	if err := d.faq.DeleteFAQ(r.Context(), id); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, envelope{OK: true, Data: struct {
		ID string `json:"id"`
	}{idText(id)}})
}
