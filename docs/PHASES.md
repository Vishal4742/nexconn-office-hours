# Phases — Office Hours

**v1.1 (2026-11-12, after the staff review):** slices merge to `main` as soon as CI is green, so every gate runs on production; outreach is a separate human track and no longer blocks a phase; scope cuts applied.
Deadline: **Nov 22, 23:59 AoE = Mon Nov 23, 17:29 IST**. We submit **Nov 22 by 20:00 IST**. Feature freeze: **Nov 19, 10:00 IST**.
Each phase runs the loop in `WORKFLOW.md`. A phase closes only when its gate passes **on the production URL**, with real output recorded in `notes/phase-N.md`.
Status: ⬜ not started · 🟨 in progress · ✅ closed (date). Bug severity: **S1** blocks the core flow · **S2** wrong but has a workaround · **S3** cosmetic.

| Phase | Dates | Goal | Status |
|---|---|---|---|
| P0 | Nov 12–13 | Accounts in the right order, spike confirms §8 assumptions, deployed hello-world room | 🟨 |
| P1 | Nov 13–14 | Sign-in, projects, rooms (Chat UI + profiles) | ⬜ |
| P2 | Nov 14–16 | Sessions, hands, live queue | ⬜ |
| P3 | Nov 16–17 | FAQ entries, public pages, suggestions, issue links | ⬜ |
| P4 | Nov 17–19 10:00 | Polish, stats, stretch if time → **freeze** | ⬜ |
| P5 | Nov 19–21 | Real sessions, S1/S2 fixes only, security checks, traction evidence | ⬜ |
| P6 | Nov 22 | Video, write-up PDF, submission | ⬜ |

## Human track (runs in parallel; never blocks a gate)
| When | What |
|---|---|
| Nov 12 | Create the accounts in the P0 order below. Write the Nexconn app creation date and plan in `notes/phase-0.md` |
| Nov 12–13 | Pick UI references and approve the clickable demo (`docs/UI.md` §6) before P1 UI work |
| Nov 12–13 | Message 8–10 maintainers (your repos, friends' repos, small libraries with many "how do I" issues, dev Discords). Book sessions for **Nov 16–21** |
| Nov 13 | Ask Nexconn (challenge support issue) whether challenge apps stay active through judging (Nov 23–Dec 1), if A-1 shows the default one-month Free plan |
| Nov 16–21 | Hold the sessions; screenshot stats after each one; collect 2–3 quotes |
| Nov 22 | Record the voice-over; submit |

## P0 — Accounts, spike, skeleton (Nov 12–13)
**Setup order (human, values only into `.env` / Vercel):**
0. **Install Go 1.25+ first** (`brew install go`): every P0 agent task needs it.
1. Nexconn account and app (in Console: plan check A-1, profanity filter on, translation on only for testing) → App Key, App Secret, area/base URL.
2. First commit pushed to GitHub (this repo).
3. Vercel project linked to the repo (Services: `web` + `go`); Neon added through the Vercel Marketplace (it sets `DATABASE_URL` in Vercel). Local `.env` uses a separate Neon **`dev` branch**, so tests never land in the traction stats; `MIGRATE_DATABASE_URL` is set only when migrating production.
4. **Two** GitHub OAuth apps: one for production (callback `https://<app>/auth/github/callback`) and one for local dev (callback `http://localhost:5173/auth/github/callback`; Vite proxies `/api`, `/auth` and the regex `^/p/[^/]+/[^/]+/faq` to Go on `:8080`).
5. Branch protection on `main` **after the first CI run** (the check names only exist then): required status checks only (no review approval: there is no second reviewer).
6. Line up 2 friends with GitHub accounts for the 3-person tests (AT-6, AT-16, AT-17, AT-19).

**Agent tasks:**
1. `spike/` Go program using the official Nexconn Go SDK v0.1.2: issue tokens for 2 users + `oh_bot`, create a group, join both, send text, an `OH:Hand` card and an unpersisted (`shouldPersist:0`) `OH:Queue` ping → A-1, A-2, A-4, A-11. **Server half ✅ 2026-11-12** (`server/cmd/spike`, results in `notes/phase-0.md`); the browser half is task 3.
2. Skeleton: `server/` is the Go module (`go.mod`, Go 1.25, `net/http`, pgx; `server/migrations/` + `cmd/migrate`) + `web/` (Vite + React + strict TS, ESLint, Prettier, Vitest) + `vercel.json` services (Go rules before the web catch-all; `web` rewrites `/(.*)` → `/index.html`) + `tools/dev.sh`. **Deploy a hello-world of both services on day 1** and run every A-10 check (ARCHITECTURE §8). CI: `setup-go` with `go-version-file: server/go.mod`, `gofmt -l server`, `go -C server vet ./...`, `go -C server test ./...`, web lint/typecheck/test/build, secret scan.
3. Spike page: Chat UI singleton with ServiceHooks, the §4 init order, 3 custom bubbles, Strict Mode on → A-3, A-5; try it on a real phone → A-8. **Browser half ✅ 2026-11-12** (`web/src/spike`, `/spike?user=a|b`, needs `SPIKE=1` on the Go server): A-3, A-4, A-5 and A-12 pass with two users, A-8 passes at 390 px. Still open: a real phone, and the gate's "on production" half. The test token endpoint only opens on a local address (a stray `SPIKE=1` on a deployment would hand out tokens), so that half needs real sign-in (P1's first slice) or an owner decision to relax the guard for a protected preview.
4. Translation check → A-6 (stretch; skip if slow).
5. Chat UI theming check → A-9: answered ❌ from docs + source (see `docs/UI.md` §7). The spike instead tries mounting `nc-message-list-provider` + `nc-input-provider` without `nc-channel-detail-provider` (drops its 70 px header) and sets `MessageBubbleConfig` to our values. **Both work ✅ 2026-11-12.**

**Gate:** two browsers chat in the same group on production; the custom card renders; the ping arrives live; CI green; `ARCHITECTURE.md` §8 marked ✅/❌ with dates.

## P1 — Sign-in, projects, rooms (Nov 13–14)
**SRS:** FR-AUTH-1..4, FR-PROJ-1..4, NFR-SEC-1..3, NFR-SEC-5, NFR-PRIV-1
**Tasks:** `001_init.sql` (all tables + the 4 constraints) · GitHub OAuth in Go (`x/oauth2`, state + PKCE, `read:user`; token only in the AES-GCM session cookie) · `server/github` · `server/guard` (Origin + JSON + session + role) · `POST /api/projects` · join page · `/join` + `/members` + `/profiles` · room with Chat UI (members into `app.updateUserProfile` before the channel opens, `updateGroupMembers` when someone joins: ARCHITECTURE §4) · dashboard · Nexconn connection on every signed-in page.
**Tests first:** `owner/repo` validation, permission mapping, idempotent registration, guard rejects a bad Origin, session cookie round-trip and tamper rejection, OAuth state mismatch rejected.
**Gate:** AT-1, AT-2 (user-owned + org-owned repo), AT-3 on production.

## P2 — Sessions, hands, live queue (Nov 14–16)
**SRS:** FR-SESS-1..4, FR-SESS-6..7, FR-HAND-1..2,4..6, FR-QUEUE-1..9, FR-MOD-1, NFR-PERF-1, NFR-REL-1, NFR-SEC-4, NFR-SEC-6
**Tasks:** `server/queue` (pure) · pgx transactions with `SELECT … FOR UPDATE` · Away + "I'm here" (`sort_at`) · Last call · carry-over on end and next start · start/end (+ @all card) · raise/withdraw · Next/Skip/Remove/Done (Done stubbed until P3) · OH:Queue ping + `GET /queue` + refetch on focus/reconnect (old pings are replayed after a reconnect: refetch only for a newer `version`, one fetch at a time) · queue panel ("N ahead of you", "It's your turn") · hand bubble reading status from the queue store · sender check · mute · latency logging.
**Tests first:** every allowed and forbidden transition, FIFO, one open hand, one answering hand, version increments, not_reached / carried on end, carried first on next start, Away back 2 then to the end, closed hands refuse a raise, sender check.
**Gate:** AT-4, AT-6, AT-7, AT-9, AT-10, AT-11, AT-15, AT-16, AT-17, AT-18 on production.
**If P2 slips, cut in this order:** (1) merge (FR-HAND-8) → Should; (2) Away always sends the hand to the end, and posts no longer count as "here"; (3) +1 (FR-HAND-7) → Should. Keep Last call and carry-over (one boolean, one UPDATE).

## P3 — FAQ (Nov 16–17)
**SRS:** FR-FAQ-1..6, FR-HAND-3, FR-HAND-7..8, FR-PROJ-3 (FAQ part)
**Tasks:** Done → FAQ entry in the same transaction + `OH:FAQ` card · slug (unique per project) · Go `html/template` FAQ list and entry pages with preview metadata · +1 and merge (the room card names everyone who asked; the public page shows a count) · full-text search + suggestions in the hand form · prefilled issue link · delete → 410.
**Tests first:** slug (unicode, collisions), search ranking on fixtures, issue-URL encoding, +1 toggles and never reorders, merge carries askers into the FAQ entry.
**Gate:** AT-5, AT-8, AT-19 on production; FAQ pages load signed-out; Lighthouse mobile LCP ≤ 2.5 s (NFR-PERF-2).

## P4 — Polish, stats, stretch (Nov 17–19 10:00)
**SRS:** FR-MET-1, NFR-A11Y-1, NFR-COMP-1, NFR-OPS-2; then stretch in order: FR-QUEUE-10..12 (wait estimate, tab title, maintainer timer) → FR-I18N-1 → FR-NOTIF-2 → FR-SESS-5 → FR-AUTH-5 → FR-PROJ-5 → PWA manifest
**Tasks:** empty, loading and error states for every view · phone layout (queue tab first) · keyboard + `aria-live` · stats pages · landing page that explains it in 20 seconds · Firefox note.
**Gate:** AT-12, AT-13, AT-14 pass, and a full run of AT-1…AT-19 is green on production → **feature freeze**.

## P5 — Real sessions and hardening (Nov 19–21)
**Tasks:** support the booked sessions; fix S1/S2 only · ECC `/security-scan` (agentshield needs the human's yes: it downloads a package) · `npm audit` (web) and `govulncheck` (server; needs the human's yes, it downloads a tool) · a full AT run after the last fix · stats screenshots and quotes · a clean backup demo recording · **before the repo goes public or real users arrive, the owner replaces every secret that was shown in a chat session** (the Nexconn App Secret and the local GitHub OAuth client secret, both on 2026-11-12) and checks the production OAuth secret was never shown anywhere.
**Gate:** no open S1/S2; security findings fixed or written up; traction evidence in `notes/phase-5.md`.

## P6 — Submission (Nov 22)
**Tasks:** 2–3 min video (core flow in the first 30 s, no speed-up) · write-up PDF (why, problem, Nexconn features used, App Key, "built during the challenge") · README with screenshots · form: app URL, public repo, video, write-up, App Key, packages (Chat SDK + Chat UI), traction link + active users.
**Gate:** submitted by Nov 22 20:00 IST; confirmation saved in `notes/phase-6.md`.
