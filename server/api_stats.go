package main

import "net/http"

// The numbers of FR-MET-1 as JSON. Owned by the FAQ-and-stats API part of the parallel build
// (notes/phase-2-plan.md, worker 8). The shape (statsJSON) and the store interface (statsStore)
// are in shapes.go. Both routes are public: the numbers are counts, with no names in them.

// projectStats is GET /api/p/{owner}/{repo}/stats: one project's numbers.
func (d deps) projectStats(w http.ResponseWriter, r *http.Request) { notBuilt(w) }

// siteStats is GET /api/stats: the numbers of every project together.
func (d deps) siteStats(w http.ResponseWriter, r *http.Request) { notBuilt(w) }
