# Architecture — Office Hours

**Version:** 1.2 (2026-11-12, owner decision: **Go server + TypeScript UI**; queue extras from SRS v1.2; staff-review fixes: module root, SPA fallback, `sort_at`, timeouts, privacy of names). v1.1 (after the staff review: server calls via signed fetch, Chat UI hooks + members table, version-only queue ping, sender check on OH:* messages, interactive DB transactions, cuts). Requirement IDs refer to `SRS.md`. §8 lists what P0 must confirm. Where an assumption fails, apply its fallback and update this file.

## 1. Shape
```
 Browser: web/ (Vite + React + TS, static)        Server: server/ (Go 1.25 on Vercel)
 ┌──────────────────────────────┐   HTTPS/JSON    ┌───────────────────────────────────────┐
 │ Landing, dashboard, join     │ ───────────────▶│ /api/*   projects, sessions, hands,    │
 │ Room: Chat UI (group channel)│                 │          faq, profiles, stats, token   │
 │ Queue panel (React)          │◀── GET /queue ──│ /auth/*  GitHub OAuth + session cookie │
 │ @nexconn/chat + @nexconn/chatui                │ /p/{o}/{r}/faq*  FAQ pages (SSR, Go)   │
 └───────────────┬──────────────┘                 │ queue/   pure state machine            │
                 │ Nexconn realtime: chat,        │ nexconn/ ONLY package calling Nexconn  │
                 │ cards, OH:Queue version pings  │ github/  OAuth + permission check      │
                 ▼                                │ store/   Postgres (pgx pool)           │
          Nexconn Chat cloud ◀──── Server API ────┴───────┬───────────────────────────────┘
                                                          Neon Postgres
```
- **The server is the single writer of queue state** (FR-QUEUE-1). Clients ask the server to change it; it validates, writes Postgres in one transaction, then sends a small **version ping** (`OH:Queue {sessionId, version, sentAt}`) into the room through Nexconn. Clients that see a newer version call `GET /queue`. One code path, no message size limit, and a forged ping does no harm. If the A-10 timings show slow cold `GET /queue` calls (Go has no Fluid compute; N members = N fetches), put the queue snapshot in the ping instead (Nexconn allows 128 KB; the sender check still applies).
- **Chat UI renders the room** (group channel). The **queue panel** is our React component beside it. On phones the queue tab opens first (Chat UI doesn't target portrait mobile).
- **Public FAQ pages are rendered by Go** (`html/template`): readable signed out, link-preview tags, no JavaScript needed (FR-FAQ-3, NFR-PERF-2). Everything else is the static TypeScript UI.
- **Every signed-in page connects to Nexconn**, not only the room. Traction counts MAU, which only counts users who connect through the client SDK.

## 2. Stack
| Concern | Choice | Why |
|---|---|---|
| UI | Vite + React + TypeScript strict in `web/`, built to static files | No Node server at runtime; the room is client-only anyway (Chat UI) |
| Server | Go 1.25, `net/http`; **`go.mod` lives in `server/`** (the Vercel service root), so commands run as `go -C server …`; `main.go` listens on `$PORT` (default 8080) | Owner's choice (2026-11-12): compiled, small and fast; official Nexconn Go SDK |
| Public pages | Go `html/template` for `/p/{owner}/{repo}/faq` and `/p/{owner}/{repo}/faq/{slug}`, with an inline `<style>` (Vite's hashed CSS isn't reachable from Go) | Server-rendered, signed-out, link previews |
| Chat UI | `@nexconn/chatui` (Lit web components), client-only, **module-level singleton** | Supports direct/group/system channels (not community), so rooms are **group channels**. A singleton because React Strict Mode/Fast Refresh mount twice and custom elements can't be defined twice |
| Chat SDK | `@nexconn/chat` + `@nexconn/engine` | Required by the challenge |
| Server → Nexconn | `server/nexconn` wraps the official `github.com/NexconnAI-Dev/nexconn-server-sdk-go` **v0.1.2** (MIT, generated, no deps): `UserManagementAPI.IssueAccessToken`, `GroupChannelManagementAPI.CreateGroupChannel` / `JoinGroupChannel`, `MessageManagementAPI.SendGroupChannelMessage` (`shouldPersist`, `isEchoToSender`), `GroupChannelModerationAPI.AddGroupChannelUserMuteList`. Needs **both** primary and backup domains (`SetPrimaryBackupDomains`). Give it an `http.Client` with a **3 s timeout** (the SDK falls back to `http.DefaultClient`, which has none). Send `OH:Queue` with `DisablePush` + `DisableUpdateLastMsg`, and `OH:Session` with `PushContent` (needed for push on custom types) | Young generated SDK: if a call misbehaves, the fallback is ~30 lines of signed `net/http` (signature verified, A-2) |
| Auth | `golang.org/x/oauth2` + `/github` (state + PKCE), `scope: read:user`. Session = AES-GCM cookie from stdlib `crypto/cipher` (HttpOnly, Secure, SameSite=Lax); `redirect_uri` from config, never from the Host header. `Secure` follows the `APP_URL` scheme (Safari drops Secure cookies on `http://localhost`); other hosts redirect to `APP_URL`; preview deployments can't sign in | Email isn't needed. The GitHub token stays inside the encrypted session cookie |
| DB | Neon Postgres via `pgx/v5` `pgxpool`, created once per instance (`MaxConns` 2, short idle timeout) on the `-pooler` host; `BeginTx` + `SELECT … FOR UPDATE`. SQL files in `server/migrations/` (embedded with `go:embed`) applied by `go -C server run ./cmd/migrate` (< 40 lines: a `schema_migrations` table, one transaction per file, no down migrations) using `MIGRATE_DATABASE_URL`. Local dev uses a Neon **`dev` branch**, never production | Neon's serverless guidance; PgBouncer transaction mode supports this |
| Validation | Explicit checks at every handler boundary (lengths, types, `owner/repo` pattern) in one `server/validate.go` | NFR-SEC-3 |
| Tests | `go -C server test ./...`: table tests named after SRS IDs for queue, validation, slug, signature, permissions, session cookie. Vitest only for UI logic. Manual AT run on production | Playwright with stubbed sign-in stays cut: a test-only auth bypass could ship to production |
| Deploy | One Vercel project with **Services** (Beta): `web` (static) + `go`. Routes `/api/*`, `/auth/*`, `/p/*/faq*` → go (placed before the catch-all), everything else → web: **one origin**, so the session cookie stays first-party. Routing into a service is final, so the `web` service rewrites `/(.*)` → `/index.html` (deep links and reloads of `/p/:owner/:repo` work). From GitHub `main` only | NFR-OPS-1. Go runtime is Beta and not on Fluid compute (one request per instance). Fallback: two projects + an external rewrite (A-10) |

## 3. Data model (Postgres)
```
users        id TEXT PK ('gh'||github_id), github_id BIGINT UNIQUE, login, name, avatar_url, created_at, deleted_at
projects     id PK, owner, repo, UNIQUE(lower(owner), lower(repo)), description, nexconn_group_id, next_session_at, created_by, created_at
maintainers  project_id, user_id, PK(project_id, user_id)                  -- source of truth for roles
members      project_id, user_id, joined_at, PK(project_id, user_id)       -- written by /join; gives "unique participants"
sessions     id PK, project_id, title, status ('live'|'ended'), planned_minutes, started_at, ended_at, hands_closed BOOL DEFAULT false, queue_version INT DEFAULT 0
hands        id PK, session_id, author_id, title, details, status, raised_at, sort_at, taken_at, closed_at, acked_at, away_count INT DEFAULT 0, merged_into NULL
hand_plus    hand_id, user_id, PK(hand_id, user_id)                          -- "+1 me too" (FR-HAND-7); never changes the order
faq_entries  id PK, project_id, hand_id NULL, slug, title, question, answer, links JSONB, author_id, created_at, deleted_at
```
**Constraints the database enforces** (so racing requests can't break the rules):
1. `UNIQUE (project_id) WHERE status='live'` on sessions (one live session per project, FR-SESS-2)
2. `UNIQUE (session_id, author_id) WHERE status IN ('waiting','answering')` on hands (one open hand per user, FR-HAND-2)
3. `UNIQUE (session_id) WHERE status='answering'` on hands (one hand answered at a time, FR-QUEUE-3)
4. `UNIQUE (project_id, slug)` on faq_entries, plus a `tsvector` GIN index on title + question + answer (FR-FAQ-4)
Hand statuses: `waiting`, `answering`, `answered`, `skipped`, `withdrawn`, `removed`, `not_reached`, `carried`, `merged`. The queue orders by **`(sort_at, id)`**; `sort_at` starts as `raised_at`, which never changes (FR-HAND-6 and the wait estimate read it). Away sets `sort_at` between the 2nd and 3rd waiting hands, to the end on a second miss, and a third miss is `skipped`. A carried hand gets the next session's id when it starts and keeps its `sort_at`, so it sits at the front. Merge copies the merged hand's +1s into the target (`INSERT … SELECT … ON CONFLICT DO NOTHING`). "Asked by N people" on a public FAQ page is a count; names appear only inside the room.
Stats (FR-MET-1) are `COUNT` queries on these tables, with no events table. Rate limit FR-HAND-6 is one query on `hands.raised_at`.

## 4. Nexconn mapping
| Concept | Nexconn object |
|---|---|
| User | Nexconn user `gh<githubId>`; token issued by our server (FR-AUTH-2) |
| Bot | User `oh_bot`, name "Office Hours": token issued once at setup so push notifications show the right sender; sender of all OH:* messages |
| Project room | Group channel `p_<projectId>`, created by the server at registration; members added by `/join` |
| Session card | Custom `OH:Session` (persisted, counted): `{sessionId, title, state:'live'\|'ended'}`; sent with an @all mention so members get an unread badge (FR-SESS-3) |
| Hand card | Custom `OH:Hand` (persisted, counted): `{handId, sessionId, title, authorLogin}`. Cards can't change after sending, so the bubble reads the hand's live status from the client's queue store by `handId` |
| FAQ card | Custom `OH:FAQ` (persisted, counted): `{faqId, slug, title}`. The bubble builds the link from `slug` on our own origin and never renders a URL from the payload |
| Queue ping | Custom `OH:Queue` registered with `isStatusMessage: true`, sent unpersisted: `{sessionId, version, sentAt}` |

**Trust rule (NFR-SEC-6):** any room member's client could send a message typed `OH:*`. The queue listener and every OH bubble **ignore OH:\* messages whose sender isn't `oh_bot`**, and render all text with `textContent`.

**Chat UI init order (once, module-level):** `NCEngine.initialize({appKey, area})` → `NCEngine.registerCustomMessages([...4 types])` → `NCChatUIApplication.initialize({hooks})` → `app.registerCustomMessages(...)` (bubble web components) → `app.ready()` → `NCEngine.connect({token})` → mount.
**ServiceHooks** (Chat UI won't render without them): `reqUserProfiles(ids)` → `GET /api/profiles?ids=` (users + a fixed `oh_bot` row; every profile needs a non-empty `name`); `reqGroupProfiles` → project name + `memberCount`; `reqGroupMembers` → `GET /api/projects/:id/members`.

## 5. API (every mutation: Origin + JSON check → session → role → validation → DB transaction → Nexconn push)
| Method + path | Who | Does | SRS |
|---|---|---|---|
| `GET /auth/github/login` · `GET /auth/github/callback` · `POST /auth/logout` | anyone | OAuth with state + PKCE; sets / clears the session cookie | FR-AUTH-1 |
| `GET /api/config` | anyone | Public client config: App Key, area | — |
| `POST /api/nexconn/token` | signed in | Nexconn token for the caller | FR-AUTH-2 |
| `GET /api/profiles?ids=` | signed in | Profiles for Chat UI hooks | FR-PROJ-4 |
| `POST /api/projects` `{owner, repo}` | signed in | Permission check, project + group (idempotent) | FR-PROJ-1..2 |
| `POST /api/projects/:id/join` · `GET /api/projects/:id/members` | signed in · member | Join (members row + group join) · list | FR-PROJ-4 |
| `POST /api/projects/:id/sessions` · `POST /api/sessions/:id/end` `{carry}` | maintainer | Start (card + @all; carried hands first) · end (waiting → carried or not_reached) | FR-SESS-1..4, FR-SESS-7 |
| `POST /api/sessions/:id/lastcall` `{closed}` | maintainer | Close / reopen hands | FR-SESS-6 |
| `GET /api/sessions/:id/queue` | member | Queue + version | FR-QUEUE-4..6 |
| `POST /api/sessions/:id/hands` · `POST /api/hands/:id/withdraw` | member · author | Raise · withdraw | FR-HAND-1..6 |
| `POST /api/sessions/:id/next` · `POST /api/hands/:id/{skip,remove,done,away}` | maintainer | Queue actions; `done` takes `{answer, links}` and creates the FAQ entry | FR-QUEUE-3, FR-QUEUE-8, FR-FAQ-1..2 |
| `POST /api/hands/:id/here` · `POST /api/hands/:id/plus` · `POST /api/hands/:id/merge` `{into}` | author · member · maintainer | "I'm here" · toggle +1 · merge into the current hand | FR-QUEUE-9, FR-HAND-7..8 |
| `POST /api/projects/:id/mute` `{userId}` | maintainer | Mute in the group | FR-MOD-1 |
| `GET /api/projects/:id/faq/search?q=` · `DELETE /api/faq/:id` | anyone · maintainer | Suggestions · delete (410 page) | FR-FAQ-4, FR-FAQ-6 |
| `DELETE /api/me` (Should) | signed in | Anonymize my hands, delete my user row, expire my Nexconn tokens | FR-AUTH-5 |
Pages: static UI `/` · `/dashboard` · `/p/:owner/:repo` (join + room) · `/p/:owner/:repo/stats` · `/stats`; Go-rendered `/p/:owner/:repo/faq` · `/p/:owner/:repo/faq/:slug`.

**Nexconn calls in `server/nexconn`** (SDK methods in §2): issue token, create group, join group, send group message (`shouldPersist:0` for OH:Queue; @all mention for session cards), mute, expire token. `isPersisted` / `isCounted` are **client** flags set when the UI registers each custom type; the server only sends `shouldPersist`.

## 6. Key flows
**Queue action (raise, next, skip, done, withdraw, remove, end).** Handler → checks → pgx transaction: `SELECT … FROM sessions WHERE id=$1 FOR UPDATE` → load open hands → `server/queue` computes the transition (returns an error on an illegal one) → write hands + `queue_version += 1` (+ FAQ insert on done) → COMMIT → send the card if any (`OH:Hand`/`OH:FAQ`/`OH:Session`) and the `OH:Queue` ping. If the Nexconn send fails after commit, log it; clients still converge on their next `GET /queue` (on focus, reconnect, or the next ping).
**Sign-in.** GitHub OAuth (`read:user`, state + PKCE) → upsert user → the access token stays in the AES-GCM session cookie only → the client calls `/api/nexconn/token` → connect.
**Register project.** `GET https://api.github.com/repos/{owner}/{repo}` with the user's token → require `permissions.admin || permissions.maintain` → insert project + maintainer → create the group (bot + registrant). If an org restricts OAuth apps, show "ask an org owner to approve Office Hours".

## 7. Security
- Secrets only in server env (Vercel) and a git-ignored `.env`; `.env.example` lists names only (NFR-SEC-1). The App Key and data-center area are public client config.
- Roles come from the `maintainers` table, never from the client (NFR-SEC-2, AT-10).
- Every mutation rejects a request whose `Origin` ≠ `APP_URL` or whose body isn't JSON (CSRF, NFR-SEC-5). The OAuth callback checks `state` and the PKCE verifier.
- OH:* sender check, links from slugs, text via `textContent` / React escaping, external links `rel="noopener noreferrer nofollow"` (NFR-SEC-6).
- Nexconn tokens don't expire by default: expire them on account deletion (FR-AUTH-5).
- Moderation: Nexconn's profanity filter (FR-MOD-2). Automatic moderation needs a card on the account, so it isn't used.

## 8. Assumptions P0 must confirm (with fallbacks)
| # | Assumption | Check | Fallback |
|---|---|---|---|
| A-1 | The app is on the **challenge plan** (10,000 MAU, one year), not the default Free plan (one month, then blocked; 2,000 MAU; 300 members per group; PCC 5% of MAU) | Console plan page; write the app creation date in `notes/phase-0.md` | Ask Nexconn through the challenge's support issue before building further |
| A-2 | The REST paths in §5 and the signature formula work with our App Key and area | Spike script: token, group, join, text, custom card, unpersisted status message | Read the Go SDK's signing code for the exact formula; ask Nexconn support |
|  | **Part ✅ 2026-11-12:** Singapore data center (`api.sg-light-api.com`); signature `sha1hex(secret+nonce+timestamp)` works; token issue returns `{code:0, result:{userId, accessToken}}`; `oh_bot` registered. Group calls still to check | | |
| A-3 | Chat UI renders our 3 registered custom bubbles in a group channel, with the init order of §4 | Spike page | Show cards only in the queue panel; plain-text summary in chat |
| A-4 | Status messages reach online members live and aren't stored | Two browsers | Persisted `OH:Queue` with `isCounted:false` |
| A-5 | Chat UI works as a client-only singleton in the Vite + React app | Spike page with Strict Mode on | Mount in a plain `<div>` via `useEffect`, or an iframe route |
| A-6 | Translation (stretch) works on Web and fits the plan's allowance | Console + one message | Drop FR-I18N-1 |
| A-7 | GitHub permission check works for a user-owned **and** an org-owned repo | Try both in P1 | Org-approval message (§6) |
| A-8 | Chat UI is usable enough on a phone to type a message | Real phone in P0 | Phones get queue + hand form; chat is desktop-first |
| A-9 | Chat UI's colours can be set through CSS custom properties (to match `docs/UI.md` tokens) | Inspect the `nc-*` elements' styles in the spike page | Keep our shell neutral around Chat UI's default look | ❌ 2026-11-12 from docs + package source: no theme tokens, only `MessageBubbleConfig` (colours, radius, layout), font inherits, no dark mode, `min-width: 600px`. Design adapts (accent `#0047FF`, light only), see `docs/UI.md` §7. Spike also tries mounting `nc-message-list-provider` + `nc-input-provider` directly to drop Chat UI's header |
| A-10 | Vercel **Services** (Beta) routes `/api/*`, `/auth/*`, `/p/*/faq*` to Go and the rest to the static UI on one origin, and `Set-Cookie` from Go reaches the browser | Deploy a hello-world of both services on **day 1** of P0 and prove: a reload of `/p/a/b` (SPA fallback), `/p/a/b/faq/x` reaches Go, `Set-Cookie` from Go survives a request to a web route, one pgx query from Go, and 10 parallel cold `curl -w` timings | Two Vercel projects + an external rewrite from the UI project (`Cache-Control: private, no-store` on Go responses) |
| A-11 | The Nexconn Go SDK v0.1.2 does every call we need (create group, join, send with `shouldPersist:0`, mute) | Go spike in P0 | Signed `net/http` for the call that fails |
Record results in `notes/phase-0.md` and mark this table ✅/❌ with the date.

## 9. Repo layout (target)
```
server/                  Go module root (go.mod, go 1.25): main.go (mux, $PORT) · nexconn/ (ONLY Nexconn caller) · github/ · queue/ (pure, tested) ·
                         store/ (pgx) · guard/ (Origin/JSON/session/role) · session/ (AES-GCM cookie) · pages/ (FAQ templates) ·
                         migrations/ (001_init.sql …, go:embed) · cmd/migrate/
tools/dev.sh             loads .env (Go doesn't) and runs `go -C server run .` on :8080
web/                     Vite + React + TS: pages, QueuePanel, sheets, ChatRoom (Chat UI singleton), OH bubbles; Vitest for UI logic
vercel.json              services: web (static) + go, and the routes above
spike/                   P0 throwaway Go program (deleted after P0)
docs/  notes/  tools/
```
