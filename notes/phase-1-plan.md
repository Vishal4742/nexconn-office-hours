# Phase 1 plan — sign-in, projects, rooms

Written 2026-11-12, a day ahead of the dates in `docs/PHASES.md`. **v2, same day, after the staff review** (verdict: go after four must-fix points; all four and the should-fix points are folded in below, and "Review" at the end lists what changed). Phase 0's code is on `main` (PR #1, CI green); its gate still waits for the first deployment (owner: Vercel + Neon). Phase 1 runs in parallel.

## Scope
| SRS | What |
|---|---|
| FR-AUTH-1..4 | GitHub sign-in (`read:user`), Nexconn token for `gh<id>`, every signed-in page connects and reconnects, sign-out |
| FR-PROJ-1..4 | Register a repo you maintain, room created, join page, join + room with Chat UI and our profiles |
| NFR-SEC-1..3, NFR-SEC-5 | Secrets on the server only; role checks from the `maintainers` table; input validation; encrypted session cookie; state + PKCE; Origin and JSON checks |
| NFR-PRIV-1 | No email, no private repo data |

**Proved by** AT-1 (sign in), AT-2 (register: user-owned, org-owned, one you don't maintain), AT-3 (a second user joins, both chat, names and avatars show). The gate runs them on production.

Not in Phase 1: sessions, hands, queue (P2); FAQ pages (P3); landing art and its questions section, dark mode, stats (P4).

## Decisions
1. **OAuth with the standard library, no `golang.org/x/oauth2`.** Two requests. GitHub's docs (read 2026-11-12): authorize with `client_id`, `redirect_uri`, `scope`, `state`, `code_challenge`, `code_challenge_method=S256` (plain is refused); exchange at `POST https://github.com/login/oauth/access_token` with `client_id`, `client_secret` (still required with PKCE), `code`, `redirect_uri`, `code_verifier` and `Accept: application/json`. The page does not document the error shape, so "no `access_token` in the reply" is a failure whatever the status. Conditions from the review: our own `http.Client` with a 5 s timeout, every parameter in the POST body (Go prints URLs into errors), and the RFC 7636 appendix B vector as the test of the hand-written challenge.
2. **Project ids are random**: `strings.ToLower(rand.Text())` from `crypto/rand` (26 base32 characters). The room's group id is `p` + that id, computed in one function; there is no `nexconn_group_id` column.
3. **Provision first, record second.** Registering a repo creates the Nexconn group and joins the registrant under a fresh id, and only then inserts the project row (`ON CONFLICT (lower(owner), lower(repo)) DO NOTHING`). If someone else won the race, their project is used and its group joined. So a project row always means a room we created: an id is never public before its group exists, and there is nothing to heal. A failed attempt leaves an orphan group, which is harmless. Join works the same way: `JoinGroup`, then the members row. (This replaces v1's "row first, heal on retry", which left the id public while the group did not exist.)
4. **A second maintainer registers the same repo:** if GitHub says they have admin or maintain rights, they become a maintainer and member of the existing project. That covers co-maintainers (FR-PROJ-5) without an invite flow.
5. **No router or data-fetching package.** Five routes: a small path matcher with `history.pushState`. A typed `fetch` wrapper for the API envelope.
6. **Icons are inline SVG.** Fonts from Google Fonts as in the demo; self-hosting is a P4 item.
7. **Org repos:** the check reads `permissions.admin || permissions.maintain`. On an organization's repo a failure says both things it can mean ("GitHub says you don't have admin or maintain rights on X. If you do, ask an owner of ORG to approve Office Hours"). A reply without a `permissions` key is assumption A-7 failing and is logged as such.
8. **Postgres in tests:** store tests run against a real database, each test in its own schema, locally and in CI (Postgres service). CI fails if any test is skipped. Handler tests in `package main` use fakes, so packages never share tables.
9. **Chat UI's hooks answer from memory.** The room loads its members from our server first; `reqUserProfiles` and `reqGroupMembers` answer from that map and never fetch. Reason (read in the Chat UI bundle by the reviewer): Chat UI clears its member list and awaits the members hook every time a room is selected, and it does not repaint when the list changes later. There is no `GET /api/profiles` endpoint: nothing would call it, and it would let any signed-in user look up any user.
10. **The Phase 0 test code:** the token endpoint, `SPIKE`, the `/spike` page and the cookie probe are deleted in the slice that finishes the real room (owner's choice). `server/cmd/spike` stays until P2 sends real cards: it is a local command, and the only sender of cards and pings until then.

## Slices
Each is a branch `slice/p1-<name>` from an up-to-date `main`, tests first, PR, CI green, merge (owner approved this flow for Phase 1).

### 1. `p1-schema` — tables and the migrate command ✅ built 2026-11-12
- **Files:** `server/migrations/001_init.sql`, `server/migrations/migrations.go`, `server/store/migrate.go`, `server/cmd/migrate/main.go`, `.github/workflows/ci.yml`.
- **Tests first:** applying twice changes nothing · a broken file changes nothing and is applied once fixed · FR-SESS-2, FR-HAND-2, FR-QUEUE-3, FR-FAQ-1, FR-PROJ-2 each refused by Postgres, with a row each rule must still allow · a status outside FR-QUEUE-2's nine · NFR-PRIV-1 no email column.
- All nine tables are created now, as `docs/PHASES.md` asks. Until the first production migration `001_init.sql` may still be edited; P2 will shape the queue tables' details.

### 2. `p1-session-guard` — session cookie and request guard ✅ built 2026-11-12
- **Files:** `server/session/session.go`, `server/guard/guard.go`, tests for both.
- **Builds:** AES-256-GCM seal/open of a small JSON payload with an expiry, keyed by `AUTH_SECRET` (32 bytes, base64); the cookie's name is bound in as additional data, so a login-state cookie can't be replayed as a session. Session lifetime 30 days, `Max-Age` equal to the sealed expiry. `guard`: `Origin` must equal `APP_URL`'s origin, the body must be JSON and at most 16 KB, a session with a user id is required; member and maintainer checks ask the store (a few lines; their real test is AT-10 in P2). Stateless by design: logout deletes the browser's copy only, and rotating `AUTH_SECRET` signs everyone out (`ponytail:` comment).
- **Tests first (NFR-SEC-5, NFR-SEC-2):** round trip · one changed character, another key, another cookie's name, cut short, plain JSON: all refused · expired refused · a payload without a user id is no session · `Secure` follows `APP_URL`'s scheme, `HttpOnly`, `SameSite=Lax`, `Path=/` always · wrong or missing `Origin` → 403 · non-JSON → 415 · oversized body → 413 · no session → 401 · a login-state cookie sent as the session → 401 · a non-member → 403, a member on a maintainer action → 403 · a missing or wrong-length `AUTH_SECRET` is an error at startup.

### 3. `p1-signin` — GitHub sign-in ✅ built 2026-11-12
- **Files:** `server/github/github.go` (+ test against a fake GitHub), `server/auth.go` (+ test), `server/store/users.go` (+ test), `server/main.go` (config validation, routes), `docs/ARCHITECTURE.md`.
- **Builds:** `GET /auth/github/login?next=` (a login started on another host is first redirected to `APP_URL`; state, PKCE verifier and `next` go into a 10-minute encrypted cookie), `GET /auth/github/callback` (checks state, exchanges the code, reads `GET /user`, upserts the user as `gh<id>`, sets the session cookie with the GitHub token inside, redirects to `next`), `POST /auth/logout`, `GET /api/me`. Every failure redirects to `/?signin=<reason>`, where the page shows one sentence and a retry; no JSON in a top-level navigation. `Cache-Control: no-store` on all of `/auth/*`. `APP_URL` is validated at startup (scheme and host, no path).
- **`next` is only ever** `/dashboard` or a path matching `/p/{owner}/{repo}` (the existing patterns). Coming back lands on the join gate; joining stays a click.
- **Display name:** `name`, or `login` when GitHub's `name` is null or empty. One function, used by the upsert, by `IssueToken` and by the member list.
- **Tests first (FR-AUTH-1, FR-AUTH-4):** the login redirect carries `scope=read:user`, `state`, `code_challenge`, `S256` and a `redirect_uri` built from `APP_URL`, never from the Host header · RFC 7636's vector (`dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk` → `E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM`) · `next` as `//evil.com`, `/\evil.com`, `https://evil.com` or anything else unknown falls back to `/dashboard` · callback with a wrong, missing or expired state: no session cookie · `?error=access_denied` (the user pressed Cancel) · the exchange sends the verifier in the body · a reply without `access_token` · the exchange works but `GET /user` fails: no session cookie · a GitHub that never answers gives up after the timeout · `"name": null` becomes the login · the user row has no email · a whole fake sign-in with the log captured: the secret, code, verifier and token never appear in it · `/api/me` is 401 signed out and never returns the GitHub token · logout clears the cookie and needs the right `Origin`.

### 4a. `p1-web-frame` — the web app's frame, no SDK yet ✅ built 2026-11-12
- **Files:** `web/src/styles/` (tokens and base styles from the approved demo), `src/router.ts`, `src/api.ts`, `src/auth.ts`, `src/pages/Landing.tsx` (headline, paragraph, the one action; no art, no questions section), `src/pages/Dashboard.tsx` (empty list), `src/components/` (header with avatar and sign out, sign-in button, icons), the sign-in failure sentence for `?signin=`.
- **Tests first:** route matching for `/`, `/dashboard`, `/p/:owner/:repo` and unknown paths · the API wrapper turns `{ok:false,error}`, non-JSON replies and network failures into errors with a message a person can read · a bundle check in CI: after the build, nothing in `web/dist` matches `gho_|client_secret|APP_SECRET|AUTH_SECRET`.

### Before 4b: three facts from the Phase 0 test page (1 to 2 hours, no merge) ✅ found 2026-11-12
With a third test user on the existing `/spike` page, find out and write down: (a) does a late joiner's first message get the right name after `updateGroupMembers` + `requestUpdate()` on the message list, without a mouse move; (b) does leaving a room and opening the same one again work (`openChannel` answers OK without re-selecting); (c) can a second room be opened after the first (Chat UI's types have no "close channel"). These decide how `openRoom` is written; guessing them would surface on the last slice.

### 4b. `p1-nexconn` — one Nexconn connection for every signed-in page ✅ built 2026-11-12
- **Files:** server: `server/api_nexconn.go` (`POST /api/nexconn/token`) + test. Web: `src/nexconn/client.ts` (a port of the test page's `boot()` without opening a channel: engine, Chat UI with the hooks of decision 9, the three OH cards and `isFromBot`, bubble and menu config, `ready()`, handlers, connect), `src/nexconn/cards.ts`, `src/components/ConnectionBanner.tsx`.
- **Why the cards are here and not in the room slice:** Chat UI only learns it is connected through a handler its constructor registers, and cards, bubble config and menu can't be added after `ready()`. So the whole start has to exist before the first connect. The SDK is loaded lazily after sign-in is known, so the landing stays light.
- **Builds:** connect once per page load, reconnect on its own, and on an invalid token fetch a new one once and retry (FR-AUTH-3). Starting the SDK and connecting are separate steps, so a failed connect can be retried without a reload. Sign-out disconnects (FR-AUTH-4).
- **Tests first:** token endpoint: signed out 401 · bad `Origin` 403 · a `userId` in the body or query is ignored, the token is always the caller's · Nexconn failing gives 502 and no detail · the retry rule (pure function): one fresh token, then give up · cards refuse a sender that isn't the bot.

### 5. `p1-projects` — register a repo ✅ built 2026-11-12
- **Files:** `server/github/github.go` (`RepoPermission`), `server/api_projects.go`, `server/store/projects.go`, `server/validate.go`, tests; web: `src/pages/Dashboard.tsx` (list + form).
- **Builds:** `POST /api/projects {owner, repo}` (guard → validation → GitHub permission → decision 3's order), `GET /api/projects/{owner}/{repo}` (public join-page data, plus `isMember` and `isMaintainer` for a signed-in caller), `GET /api/me/projects`. The project stores GitHub's own spelling of owner and repo and its description.
- **Tests first:** NFR-SEC-3 patterns, with `.` and `..` refused as repo names · FR-PROJ-1 table: admin, maintain, push only, 404, private repo refused (NFR-PRIV-1), 403, organization owner (decision 7's message), 401 from GitHub ("sign in again": GitHub keeps ten tokens per user and app and revokes the oldest), no `permissions` key, timeout or 5xx ("GitHub didn't answer") · FR-PROJ-2: a second registration returns the same project · two registrations of one repo at the same moment give one project and no error · decision 4 · signed out 401, bad `Origin` 403.
- **Changed while building** (details in `notes/phase-1.md`): the page's data is `GET /api/p/{owner}/{repo}`, not `/api/projects/{owner}/{repo}`, where a repo called `members` would collide with slice 6a's routes · a project records GitHub's id of its repo, and the same name with another id is refused (a name can change hands) · the store's `IsMember` and `IsMaintainer` and the guard's roles move to slice 6a, whose member list is the first route that needs a role.

### 6a. `p1-room` — join, and a room that passes AT-3 ✅ built 2026-11-12
- **Files:** `server/api_projects.go` (`POST /api/projects/:id/join`, `GET /api/projects/:id/members`), `server/store` (`IsMember`, `IsMaintainer`, given to the guard as its roles), tests; web: `src/pages/Room.tsx`, `src/room/ChatRoom.tsx`, `src/room/JoinGate.tsx`, `src/room/members.ts`.
- **Builds:** the join gate for visitors and non-members (with "Sign in with GitHub to join" carrying `next`); for members a plain room: our header and the chat. The room loads its members, hands them to Chat UI (`updateUserProfile`), then opens the channel. A message from a sender it doesn't know triggers one member refresh, `updateGroupMembers`, and a repaint of the message list on the next tick.
- **Tests first:** join twice is one member row · `/members` refuses a non-member (403) and includes the bot · the "unknown sender" rule (pure function).
- **Exit check, in two browser profiles:** A has the room open, B joins and sends one message, and A sees B's name and avatar within 2 s without moving the mouse or reloading. If the repaint doesn't do it, the server sends a transient bot hint on join so clients refresh their members before the first message.
- **Also tried once between two users** (FR-PROJ-4 names them): an image, a file, a reply, an @mention, a read receipt. Whatever fails comes out of the input menu and into the notes.
- **Changed while building** (details in `notes/phase-1.md`): the two routes are `POST /api/p/{owner}/{repo}/join` and `GET /api/p/{owner}/{repo}/members`, found through the project's name like its page · the project id no longer leaves the server; members get the room's `groupId` instead · the member list does not carry the bot (the page's roster always has it) · the role check uses the answer of the project lookup, so the store's `IsMember` and `IsMaintainer` and the guard's roles wait for the first route that has only an id (P2).

### 6b. `p1-room-layout` — the room as designed ✅ built 2026-11-12
- **Files:** `src/room/QueuePanel.tsx` (the photo panel with "No session right now"; P2 fills it), layout and header per `docs/UI.md` S3 (two columns from 1040 px, stacked below until P2 has a queue to put in a tab), the connection banner in place. Deletes the Phase 0 test endpoint, `SPIKE`, the `/spike` page and the cookie probe (decision 10).

## Order
1 and 2 don't depend on each other. 3 needs both. 4a needs 3. 4b needs 4a and the three facts. 5 needs 3. 6a needs 4b and 5. 6b needs 6a.

## What only the owner can do
| Needed | By when | How |
|---|---|---|
| GitHub OAuth app for local work | ✅ done 2026-11-12 | Callback must be `http://localhost:5173/auth/github/callback` |
| **Vercel project, Neon database, and a second OAuth app for production** | **Before slice 3 merges**, or the sign-in code lands without ever having run on the real platform (Vercel's Go runtime is Beta; the fallback costs half a day) | `docs/PHASES.md`, P0 setup order 3 and 4. **Choose Singapore for both**: Neon's region can't be changed later, and Nexconn's API is in Singapore, so a server anywhere else adds an ocean crossing to every queue update. On Vercel the function region is `sin1` |
| Migrate production | When Neon exists, before the next merge | `MIGRATE_DATABASE_URL` set to Neon's **unpooled** URL, then `go -C server run ./cmd/migrate` |
| A second GitHub account, and an org repo you maintain | AT-3 and the org half of AT-2 | A friend, or a second account of your own |
| **Nexconn Console: turn on "Enable pre-join history access by new member"** | Before real people join rooms | Console → Chat → Chat settings → Group Channels (`https://console.nexconn.ai/agile/im/service/config`). Without it, someone who joins a room sees none of the earlier messages (seen on 2026-11-12 with two users) |

Once there is a deployment, every merged slice gets a smoke check on it: `/api/health` answers, and `/auth/github/login` redirects with the production `redirect_uri` and a `Secure; HttpOnly; SameSite=Lax` state cookie.

## Risks
1. **The first deployment has no date.** See the table. If it still doesn't exist when slice 3 is ready, I stop and ask.
2. **GitHub permissions for org repos (A-7).** Tried with a real token as soon as slice 3 works locally.
3. **Chat UI in a navigable app.** The three facts above are found out before the client is written.
4. **Names in the room** (decision 9 and 6a's exit check).
5. **Vercel and Neon details that bite at the gate:** no goroutine may outlive its response (the instance freezes); database timeouts of 5 s or more, since Neon wakes from idle; if the first query fails on prepared statements behind the pooler, set pgx's `default_query_exec_mode=exec`. CI's Postgres is 17; move it to Neon's major version once known.

## Done means (per slice, from `docs/WORKFLOW.md` §6)
SRS ids implemented with passing tests named after them · gofmt, vet, Go tests with none skipped in CI, web lint, typecheck, tests and build pass · every mutation goes through `guard` · no secret in code, logs or the bundle · merged with CI green · its AT steps noted in `notes/phase-1.md`, on production once there is one.

## Phase-close reviews (2026-11-12)
Security, Go, web and ponytail reviews of the whole phase; results and fixes in `notes/phase-1.md`. Carried forward: **P2's plan must re-check a maintainer's GitHub rights** before a maintainer action counts (at session start) · P5: the `__Host-` cookie prefix, a way to release a name held by a deleted repo's project, following a repo's rename · on the first deployment: the response headers of `vercel.json`.

## Review (plan-reviewer agent, 2026-11-12)
Must-fix, all applied: a way back to the room after sign-in (`next`, slice 3) · the whole SDK start moves to 4b, because Chat UI can't take cards after `ready()` · GitHub's `name` can be null (display-name rule) · "names and avatars show" gets a testable exit check and hooks that answer from memory.
Should-fix, applied: provision first (decision 3) · the sign-in paths users really hit (Cancel, expired state, GitHub down, wrong host) · cookie purpose binding and lifetime · no skipped tests in CI · tests for the token endpoint, more permission cases, a bundle check · slices 4 and 6 split, with the fact-finding before 4b · deployment date and region.
Cut: `GET /api/profiles` · the landing's questions section (P4) · tabs (until P2).
Not taken: deferring the queue and FAQ tables out of `001_init.sql`. They were already built and tested, `docs/PHASES.md` asks for all tables here, and the file stays editable until production.
