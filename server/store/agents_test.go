package store

import (
	"context"
	"errors"
	"testing"
)

func TestAgentGrants(t *testing.T) {
	pool := testDB(t)
	s := New(pool)
	ctx := context.Background()
	mustExec(t, pool, `INSERT INTO users (id, github_id, login) VALUES ('gh1', 1, 'mia'), ('gh2', 2, 'raj')`)
	grant := func(id, user, name string) Grant {
		t.Helper()
		g := Grant{ID: id, UserID: user, ClientName: name, RedirectHost: "localhost"}
		if err := s.CreateGrant(ctx, g); err != nil {
			t.Fatalf("CreateGrant %s: %v", id, err)
		}
		return g
	}
	active := func(id string) bool {
		t.Helper()
		ok, err := s.UseGrant(ctx, id)
		if err != nil {
			t.Fatalf("UseGrant %s: %v", id, err)
		}
		return ok
	}

	t.Run("FR-AGENT-3: a person's agents, the newest first, and only theirs", func(t *testing.T) {
		grant("g-old", "gh1", "Cursor")
		mustExec(t, pool, `UPDATE agent_grants SET created_at = now() - interval '1 day' WHERE id = 'g-old'`)
		grant("g-new", "gh1", "Claude Code")
		grant("g-raj", "gh2", "ChatGPT")
		got, err := s.ListGrants(ctx, "gh1")
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 2 || got[0].ID != "g-new" || got[1].ID != "g-old" || got[0].ClientName != "Claude Code" || got[0].RedirectHost != "localhost" {
			t.Fatalf("ListGrants gh1 = %+v", got)
		}
		if !got[0].LastUsedAt.IsZero() || got[0].CreatedAt.IsZero() {
			t.Errorf("a grant not used yet: created %v, last used %v", got[0].CreatedAt, got[0].LastUsedAt)
		}
	})

	t.Run("FR-AGENT-2: a code turned in twice finds its grant made already", func(t *testing.T) {
		grant("g-twice", "gh1", "Claude")
		err := s.CreateGrant(ctx, Grant{ID: "g-twice", UserID: "gh1", ClientName: "Claude", RedirectHost: "claude.ai"})
		if !errors.Is(err, ErrGrantExists) {
			t.Fatalf("second CreateGrant = %v, want ErrGrantExists", err)
		}
		if err := s.RevokeGrantByID(ctx, "g-twice"); err != nil {
			t.Fatal(err)
		}
		if active("g-twice") {
			t.Error("a replayed code's grant still lets its agent in")
		}
	})

	t.Run("a grant for nobody we know is refused as such", func(t *testing.T) {
		err := s.CreateGrant(ctx, Grant{ID: "g-ghost", UserID: "gh404", ClientName: "x", RedirectHost: "x"})
		if !errors.Is(err, ErrUnknownUser) {
			t.Fatalf("CreateGrant for gh404 = %v, want ErrUnknownUser", err)
		}
	})

	t.Run("FR-AGENT-3: Disconnect ends a person's own grant at once, and nobody else's", func(t *testing.T) {
		grant("g-cut", "gh1", "Cursor")
		if !active("g-cut") {
			t.Fatal("a new grant does not let its agent in")
		}
		if ended, err := s.RevokeGrant(ctx, "gh2", "g-cut"); err != nil || ended {
			t.Fatalf("raj disconnecting mia's agent = %v, %v; want nothing ended", ended, err)
		}
		if !active("g-cut") {
			t.Fatal("someone else's Disconnect ended the grant")
		}
		if ended, err := s.RevokeGrant(ctx, "gh1", "g-cut"); err != nil || !ended {
			t.Fatalf("mia disconnecting her agent = %v, %v", ended, err)
		}
		if active("g-cut") {
			t.Error("a disconnected agent is still let in")
		}
		if ended, _ := s.RevokeGrant(ctx, "gh1", "g-cut"); ended {
			t.Error("a second Disconnect ended something")
		}
		list, _ := s.ListGrants(ctx, "gh1")
		for _, g := range list {
			if g.ID == "g-cut" {
				t.Error("a disconnected agent is still listed")
			}
		}
	})

	t.Run("FR-AUTH-5: a deleted account's agents are let in no more", func(t *testing.T) {
		grant("g-gone", "gh2", "ChatGPT")
		mustExec(t, pool, `UPDATE users SET deleted_at = now() WHERE id = 'gh2'`)
		t.Cleanup(func() { mustExec(t, pool, `UPDATE users SET deleted_at = NULL WHERE id = 'gh2'`) })
		if active("g-gone") {
			t.Error("a deleted account's agent is still let in")
		}
	})

	t.Run("a use is noted, at most once a minute", func(t *testing.T) {
		grant("g-used", "gh1", "Claude")
		lastUsed := func() (seconds float64) {
			t.Helper()
			if err := pool.QueryRow(ctx, `SELECT extract(epoch FROM now() - last_used_at) FROM agent_grants WHERE id = 'g-used'`).Scan(&seconds); err != nil {
				t.Fatalf("read last_used_at: %v", err)
			}
			return seconds
		}
		active("g-used")
		if s := lastUsed(); s > 5 {
			t.Fatalf("first use noted %v s ago", s)
		}
		mustExec(t, pool, `UPDATE agent_grants SET last_used_at = now() - interval '30 seconds' WHERE id = 'g-used'`)
		active("g-used")
		if s := lastUsed(); s < 25 {
			t.Errorf("a use within the minute was noted again (%v s ago)", s)
		}
		mustExec(t, pool, `UPDATE agent_grants SET last_used_at = now() - interval '2 minutes' WHERE id = 'g-used'`)
		active("g-used")
		if s := lastUsed(); s > 5 {
			t.Errorf("a use after the minute was not noted (%v s ago)", s)
		}
	})

	t.Run("an id nobody issued lets nobody in", func(t *testing.T) {
		if active("g-never") {
			t.Error("an unknown grant id is active")
		}
	})
}
