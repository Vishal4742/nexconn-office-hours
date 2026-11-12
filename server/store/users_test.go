package store

import (
	"context"
	"testing"
)

func TestUpsertUser(t *testing.T) {
	pool := testDB(t)
	s := New(pool)
	ctx := context.Background()

	read := func() (login, name, avatar string, githubID int64, deleted bool, rows int) {
		t.Helper()
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM users`).Scan(&rows); err != nil {
			t.Fatalf("count: %v", err)
		}
		if err := pool.QueryRow(ctx, `SELECT login, name, avatar_url, github_id, deleted_at IS NOT NULL FROM users WHERE id = 'gh42'`).
			Scan(&login, &name, &avatar, &githubID, &deleted); err != nil {
			t.Fatalf("read gh42: %v", err)
		}
		return
	}

	// FR-AUTH-1: the first sign-in stores id, login, display name and avatar.
	first := User{ID: "gh42", GitHubID: 42, Login: "mia-k", Name: "Mia K", AvatarURL: "https://avatars.example/u/42?v=1"}
	if err := s.UpsertUser(ctx, first); err != nil {
		t.Fatalf("UpsertUser: %v", err)
	}
	if login, name, avatar, id, deleted, rows := read(); login != "mia-k" || name != "Mia K" || avatar != first.AvatarURL || id != 42 || deleted || rows != 1 {
		t.Fatalf("after the first sign-in: %q %q %q %d deleted=%v rows=%d", login, name, avatar, id, deleted, rows)
	}

	// A later sign-in after the user renamed themselves on GitHub updates the same row, and an
	// account that was deleted here comes back.
	mustExec(t, pool, `UPDATE users SET deleted_at = now() WHERE id = 'gh42'`)
	renamed := User{ID: "gh42", GitHubID: 42, Login: "mia-codes", Name: "Mia Kovač", AvatarURL: "https://avatars.example/u/42?v=2"}
	if err := s.UpsertUser(ctx, renamed); err != nil {
		t.Fatalf("second UpsertUser: %v", err)
	}
	if login, name, avatar, _, deleted, rows := read(); login != "mia-codes" || name != "Mia Kovač" || avatar != renamed.AvatarURL || deleted || rows != 1 {
		t.Fatalf("after the second sign-in: %q %q %q deleted=%v rows=%d", login, name, avatar, deleted, rows)
	}
}
