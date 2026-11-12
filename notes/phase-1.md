# Phase 1 — sign-in, projects, rooms (in progress)

Plan: `notes/phase-1-plan.md`. Each entry below is one slice, with the real output of its checks.

## 2026-11-12 12:55 IST: slice 1, tables and the migrate command (`slice/p1-schema`)
- **Built:** `server/migrations/001_init.sql` (the nine tables of `ARCHITECTURE.md` §3 with their rules as unique indexes), `server/migrations` (the files, embedded), `store.Migrate` (each new file in its own transaction; the file's name is recorded first, so two runs at once can't both apply it), `server/cmd/migrate` (uses `MIGRATE_DATABASE_URL`, else `DATABASE_URL`; prints file names, never the URL).
- **Tests first** (`server/store/migrate_test.go`, against a real Postgres, each test in its own schema): they failed to build before the code existed, and pass now:
  - a second run applies nothing · a broken file changes nothing, isn't recorded, and is applied once fixed
  - FR-SESS-2 one live session per project · FR-HAND-2 one open hand per user per session · FR-QUEUE-3 one hand answered at a time · FR-FAQ-1 a slug is unique inside its project · FR-PROJ-2 `PaperPlane/Stash` is `paperplane/stash`. Each case also inserts a row the rule must still allow.
  - a status outside FR-QUEUE-2's nine is refused · NFR-PRIV-1 no column anywhere stores an email address
- **Real run on the local dev database:** first run "applied 001_init.sql", second run "nothing to apply: the database is up to date", no URL "migrate: set MIGRATE_DATABASE_URL or DATABASE_URL". Tables: faq_entries, hand_plus, hands, maintainers, members, projects, schema_migrations, sessions, users.
- **CI:** the Go job now starts a Postgres 17 service and sets `TEST_DATABASE_URL`; without it in CI the store tests fail and are not skipped. Locally they run on Postgres 18.6.
- **Checks:** gofmt clean · vet ok · Go tests pass (main 86.8%, nexconn 97.1%, store 84.2%), 73 passing cases and none skipped with the database set.
- **Decisions:** project ids are random text (they name the Nexconn group, see the plan's decision 2); the other ids are plain numbers. Local databases `officehours_dev` and `officehours_test` exist on this machine's Postgres.
- **After the plan review:** the `nexconn_group_id` column is gone (the group id is `p` + the project id, computed in one place). CI's test step now fails when any test is skipped, and uses `pipefail` so a failing test can't hide behind the pipe to `tee`.
- **Open:** production needs `go -C server run ./cmd/migrate` with `MIGRATE_DATABASE_URL` once Neon exists (owner sets the variable).

## 2026-11-12 13:02 IST: local GitHub OAuth app created (owner)
- The owner created the OAuth app for local work ("office hours", homepage `http://localhost:5173`). `.env` now has `AUTH_GITHUB_ID` (public) and a local `DATABASE_URL` (`officehours_dev`, no password); I set those two, since neither is a secret.
- **Security:** the app's first client secret was visible in a screenshot pasted into the chat session. I did not use it or write it anywhere. Owner's decision (13:03): replace it later, before launch; it is on the P5 list in `docs/PHASES.md`. It belongs to an app whose only callback is `localhost`, so the exposure is small. Sign-in on this machine starts working when the owner puts a secret into `.env` as `AUTH_GITHUB_SECRET` by hand.
- To check in the app's settings: the authorization callback URL must be `http://localhost:5173/auth/github/callback`.
