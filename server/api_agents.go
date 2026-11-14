package main

import (
	"net/http"
	"time"
)

// A person's connected agents, for the dashboard (FR-AGENT-3).

// agentJSON is one connected agent as the dashboard shows it.
type agentJSON struct {
	ID          string     `json:"id"`
	App         string     `json:"app"`       // what the app called itself
	ReturnsTo   string     `json:"returnsTo"` // where its consent page sent the person back
	ConnectedAt time.Time  `json:"connectedAt"`
	LastUsedAt  *time.Time `json:"lastUsedAt"` // null until its first request
}

// agentsNotSetUp is what a server without a database answers.
const agentsNotSetUp = "Apps can't connect to this server yet."

// myAgents is GET /api/me/agents: the agents the signed-in person connected, the newest first.
func (d deps) myAgents(w http.ResponseWriter, r *http.Request) {
	u, err := d.guard.User(r)
	if err != nil {
		writeError(w, err)
		return
	}
	if d.grants == nil {
		writeJSON(w, http.StatusServiceUnavailable, envelope{Error: agentsNotSetUp})
		return
	}
	grants, err := d.grants.ListGrants(r.Context(), u.ID)
	if err != nil {
		writeError(w, err)
		return
	}
	agents := make([]agentJSON, 0, len(grants))
	for _, g := range grants {
		a := agentJSON{ID: g.ID, App: g.ClientName, ReturnsTo: g.RedirectHost, ConnectedAt: g.CreatedAt}
		if !g.LastUsedAt.IsZero() {
			a.LastUsedAt = &g.LastUsedAt
		}
		agents = append(agents, a)
	}
	writeJSON(w, http.StatusOK, envelope{OK: true, Data: agents})
}

// disconnectAgent is POST /api/me/agents/{id}/disconnect: the agent is refused from its next
// request on. Someone else's agent is a 404, the same as one that never was.
func (d deps) disconnectAgent(w http.ResponseWriter, r *http.Request) {
	u, err := d.guard.Mutation(w, r, nil)
	if err != nil {
		writeError(w, err)
		return
	}
	if d.grants == nil {
		writeJSON(w, http.StatusServiceUnavailable, envelope{Error: agentsNotSetUp})
		return
	}
	ended, err := d.grants.RevokeGrant(r.Context(), u.ID, r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	if !ended {
		writeJSON(w, http.StatusNotFound, envelope{Error: "There is no such connected app."})
		return
	}
	writeJSON(w, http.StatusOK, envelope{OK: true})
}
