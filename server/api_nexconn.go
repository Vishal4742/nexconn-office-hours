package main

import (
	"log"
	"net/http"
)

// nexconnToken gives the signed-in user the Nexconn access token for their own user id
// (FR-AUTH-2). The id and the name come from the session, never from the request, and the App
// Secret stays in server/nexconn.
func (d deps) nexconnToken(w http.ResponseWriter, r *http.Request) {
	u, err := d.guard.Mutation(w, r, nil)
	if err != nil {
		writeError(w, err)
		return
	}
	if d.tokens == nil {
		writeJSON(w, http.StatusServiceUnavailable, envelope{Error: "The server has no Nexconn credentials yet."})
		return
	}
	token, err := d.tokens.IssueToken(r.Context(), u.ID, u.Name)
	if err != nil {
		log.Printf("nexconn token for %s: %v", u.ID, err)
		writeJSON(w, http.StatusBadGateway, envelope{Error: "The chat service did not answer. Try again in a moment."})
		return
	}
	writeJSON(w, http.StatusOK, envelope{OK: true, Data: struct {
		UserID string `json:"userId"`
		Token  string `json:"token"`
	}{u.ID, token}})
}
