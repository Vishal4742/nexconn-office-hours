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

## 2026-11-12 13:20 IST: slice 2, session cookie and request guard (`slice/p1-session-guard`)
- **Built:** `server/session` (AES-256-GCM seal and open with an expiry inside; the cookie's name is bound in as additional data; `Write`, `Read`, `Clear`; `User` is what the session holds; lifetime 30 days) and `server/guard` (`New` checks `APP_URL` is a plain origin; `SameOrigin`, `User`, `Body`, the combined `Mutation`; `Member` and `Maintainer` ask the store through a two-method interface). Refusals are a `guard.Error{Status, Msg}` the handler can show; anything else is an internal error.
- **Tests first:** committed before the code (they did not build). 56 cases pass now:
  - session: round trip and the value hides its contents · one changed character, another key, another cookie's name, cut short, empty, not base64, plain JSON: refused · one minute past the end: refused · `AUTH_SECRET` must be 32 bytes of base64 · HttpOnly, SameSite=Lax, Path=/, Secure only on https, Max-Age 30 days · `Clear` expires the cookie (FR-AUTH-4)
  - guard: wrong origin, our host on http, our origin as a prefix, no Origin, `null` → 403 · signed out, a login-state cookie sent as the session, a session without a user id → 401 · form post, text/plain, no content type → 415 · not JSON, unknown field, two values, empty body → 400 · over 16 KB → 413 · a member on a maintainer action and a non-member → 403 · a failing store lets nobody through
- **Checks:** gofmt clean · vet ok · session 83.7%, guard 95.7% (what is left are branches that can't fail in practice: AES setup, the random source, JSON encoding of our own types).
- **Decisions:** every mutation sends JSON, `{}` when it has nothing to say, and unknown fields are refused. Sessions are stateless (a `ponytail:` comment names the ceiling: logout deletes the browser's copy only; a new `AUTH_SECRET` signs everyone out).
- **Not yet wired:** `main.go` builds the codec and the guard in slice 3, where the first routes use them; that is also where a bad `AUTH_SECRET` or `APP_URL` stops the server at startup.

## 2026-11-12 13:25 IST: slice 3, GitHub sign-in (`slice/p1-signin`)
- **Built:**
  - `server/github`: the OAuth authorize URL, the code exchange and `GET /user`, with the standard library only and its own 5 s timeout. `User.DisplayName()` is the name, or the login when GitHub has no name.
  - `server/auth.go`: `GET /auth/github/login?next=`, `GET /auth/github/callback`, `POST /auth/logout`, `GET /api/me`. State, PKCE verifier and `next` travel in a sealed 10-minute cookie that is cleared on use. A failed sign-in lands on `/?signin=cancelled|expired|github|server|setup`.
  - `store.Store` with `UpsertUser` (by GitHub id; a rename updates the row) and `Ping`.
  - `main.go`: `newDeps` builds the session codec, the guard, the GitHub client and the store; the server does not start without a valid `APP_URL` and `AUTH_SECRET`; sign-in says "not set up" when the GitHub app or the database is missing. `.` and `..` are no longer valid repo names.
- **Tests first:** the test commit did not build. Now 83 cases pass in the server package, plus the GitHub client's and the store's:
  - RFC 7636's challenge vector · the authorize URL (read:user, state, challenge, S256, `redirect_uri` from `APP_URL`) · the exchange sends everything in the body, nothing in the URL
  - a reply without a token, not JSON, a 500, and a GitHub that never answers: failures whose text holds no secret
  - a login on another host goes to `APP_URL` first · `next`: 16 cases, only `/dashboard` and `/p/owner/repo` survive
  - the verifier GitHub receives hashes to the challenge that went out · the user is saved as `gh42` without an email · the session cookie hides the token · the state cookie is cleared
  - ten ways a callback can be wrong (foreign or missing state, no cookie, an expired one, a session cookie sent as the state cookie, Cancel, no code, GitHub refusing, the user read failing, our database down): no session, the right reason
  - a null name becomes the login · a whole sign-in with the log captured: no secret, code, verifier or token in it · `/api/me` 401 signed out, never the token · logout needs our Origin and works twice
- **Real run** (`bash tools/dev.sh`, local database): `/api/health` says `db: up` · login answers 302 to `https://github.com/login/oauth/authorize` with `scope=read:user`, `code_challenge_method=S256`, a 43-character challenge, a 26-character state and `redirect_uri=http://localhost:5173/auth/github/callback`; the state cookie is `Path=/; Max-Age=600; HttpOnly; SameSite=Lax` (no Secure on http) with `Cache-Control: no-store` · login on port 8080 redirects to `APP_URL` first · logout 200 with our Origin, 403 from another site, 415 for a form post · a callback nobody started lands on `/?signin=expired`.
- **Found by the real run:** `.env` still had `APP_URL=http://localhost:3000` from the first plan (Next.js). Sign-in redirected to port 3000. Corrected to `http://localhost:5173` (not a secret). Unit tests could not have caught this; it is why each slice gets a real run.
- **Checks:** gofmt clean · vet ok · coverage: main 86.4%, github 91.2%, guard 95.7%, session 83.7%, store 81.8%, nexconn 97.1%.
- **Open:** the click through GitHub's consent page with a real account (next), and assumption A-7 (what `permissions` says for a `read:user` token).
