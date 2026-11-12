package main

import "net/http"

// The numbers of FR-MET-1 as JSON. The shape (statsJSON) and the store interface (statsStore)
// are in shapes.go. Both routes are public: the numbers are counts, with no names in them.

const statsNotSetUp = "The stats aren't set up on this server yet."

// projectStats is GET /api/p/{owner}/{repo}/stats: one project's numbers.
func (d deps) projectStats(w http.ResponseWriter, r *http.Request) {
	if project, ok := d.projectOfPath(w, r, ""); ok {
		d.statsAnswer(w, r, project.ID)
	}
}

// siteStats is GET /api/stats: the numbers of every project together.
func (d deps) siteStats(w http.ResponseWriter, r *http.Request) {
	d.statsAnswer(w, r, "")
}

// statsAnswer answers with the five numbers of one project, or of the whole site when projectID
// is empty.
func (d deps) statsAnswer(w http.ResponseWriter, r *http.Request, projectID string) {
	if d.stats == nil {
		writeJSON(w, http.StatusServiceUnavailable, envelope{Error: statsNotSetUp})
		return
	}
	numbers, err := d.stats.Stats(r.Context(), projectID)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, envelope{OK: true, Data: statsJSON(numbers)})
}
