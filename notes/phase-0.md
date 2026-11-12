# Phase 0 — spike and setup (in progress)

## 2026-11-12 07:45 IST — credentials and memory
- `.env` created (git-ignored, mode 600): Nexconn App Key + App Secret, generated `AUTH_SECRET`, supermemory + extraction-model keys copied from `~/ethglobal/.env`. Empty still: `AUTH_GITHUB_ID`, `AUTH_GITHUB_SECRET`, `DATABASE_URL`.
- Local supermemory server for this repo is up: `http://localhost:6768`, data in `.supermemory/` (encrypted local storage, local embeddings bge-base-en-v1.5). ETHGlobal's server on 6767 is separate and untouched.
- **A-2 (part) ✅ 2026-11-12:** the key pair works on the **Singapore** data center (`https://api.sg-light-api.com`; us and sau not needed). Signature = `sha1hex(AppSecret + Nonce + Timestamp)`, headers `App-Key`, `Nonce`, `Timestamp`, `Signature`.
  `POST /v4/auth/access-token/issue {userId, name}` → `{code: 0, result: {userId, accessToken}}` (token length 84). `oh_bot` ("Office Hours") is registered.
- `NEXT_PUBLIC_NEXCONN_AREA=sg` is a placeholder: confirm the client SDK's data-center option/enum name in the spike page.

## Still to check (ARCHITECTURE §8), updated 2026-11-12 11:50 IST
- A-1 plan in the Console (challenge plan vs default one-month Free) + app creation date — human
- A-8 on a real phone, and the two-user run on production (the P0 gate): both need the first deployment
- A-10 on a real deployment (routes, cookie, one pgx query, cold-start timings): needs Vercel + Neon linked — human
- A-6 translation (stretch); A-7 GitHub permission check (P1)
- Done: A-2, A-3, A-4, A-5, A-9 (answered ❌, mount workaround ✅), A-11, A-12

## Security note
The App Secret was pasted into a chat session on 2026-11-12. Rotate it in the Console before real users arrive (it invalidates existing tokens; re-issue `oh_bot` after), then update `.env` and Vercel.

## 2026-11-12 08:10 IST: UI demo (docs/UI.md §6, step 2)
- Clickable demo at `../design/office-hours-demo.html` (outside this repo). Fake data, no network. The centre chat pane stands in for Chat UI's channel detail.
- Covers landing, room (contributor + maintainer), raise hand with FAQ suggestions, "It's your turn", Done → FAQ card, end session, start session, sign-in gate, reconnecting, no session, FAQ list + answer page (+ 410 removed), phone layout (container queries, so phone mode = real phones), dark mode, reduced motion.
- Proof: `#selftest` drives the real actions; 20/20 PASS in headless Chrome, no console errors, user text rendered as text. Screenshot flags: `#room&maintainer&phone&dark&still` and moments `#raise`, `#turn`, `#done`, `#ended`, `#inline`.
- Deviations from UI.md for the owner to approve: maintainer **Next** sits at the bottom of the queue panel (same spot on desktop and phone) instead of the left column; status text colours darkened for 4.5:1 (green `#1A7A43`, amber `#8F5E12`); phone tab badge counts waiting hands only.
- Waiting on: owner approval before P1 UI work.

## 2026-11-12 08:32 IST: design research
- Three research passes (Chat UI's real theming surface, live-queue UX patterns, visual references) are written up in `docs/UI.md` §7 with sources.
- **A-9 ❌** (from docs + `@nexconn/chatui@26.2.5` source, not yet in the spike): no CSS tokens, no dark mode, a 70 px header that can't be hidden, `min-width: 600px`, cards sit inside bubbles at max 70% width. ARCHITECTURE §8 updated.
- Demo v2 applies the proposed changes (accent `#0047FF`, light only, Chat UI-shaped chat, new breakpoints, queue patterns). Self-test 23/23 PASS, no console errors. v1 kept in the session scratchpad only.
- Pending owner decisions: the accent change, dropping dark mode, and which "proposed, not built" queue features (if any) enter P2/P4.

## 2026-11-12 09:02 IST: owner decisions (design v2, Go server, queue extras)
- **Design v2** from the owner's reference board + a mentor's reference (openvenue.xyz): photo hero that shrinks into a stacked card, editorial labels, glass cards, torn paper, bento. One AI-generated hero photo (Figma AI, 1 generation used; owner limited it to the hero) at `../design/assets/hero-meadow.webp`; everything else SVG. Accent `#0047FF`. Dark mode on landing + FAQ only. See `docs/UI.md` §1, §7, §8.
- **Server = Go**, UI = Vite + React + TS (static). Facts checked: Nexconn Go SDK v0.1.2 covers our calls; Vercel Go runtime and Services are Beta; one origin via Services keeps the cookie first-party. ARCHITECTURE v1.2, SRS v1.2, PHASES, WORKFLOW, AGENTS, CLAUDE, README, .env.example updated. New assumptions A-10 (Services) and A-11 (Go SDK) are day-1 checks in P0.
- **Queue extras approved as Must:** Away + "I'm here", Last call, carry-over, +1 and merge (+ Should: wait estimate, tab title, maintainer timer). AT-17..19 added. Demo v3 has all of them; self-test 31/31 PASS, no console errors.
- **Human to do:** install Go 1.25+ (`go` is not on this Mac); add `NEXCONN_API_BACKUP_URL` to `.env`; GitHub OAuth callbacks are now `/auth/github/callback` (prod) and `http://localhost:5173/auth/github/callback` (local).
- A staff review of the Go switch (plan-reviewer) is running; its findings go here next.

## 2026-11-12 09:22 IST: staff review applied, design v4 (original), 3 new references
- **Staff review of the Go switch (plan-reviewer): 3 blockers, 5 high, all applied to the docs.** `go.mod` lives in `server/` (commands are `go -C server …`; migrations in `server/migrations/`; `/smoke` now detects `server/go.mod` and `web/package.json`); the `web` service needs an SPA fallback rewrite; A-10 lists exactly what day 1 must prove; queue order is `(sort_at, id)` so Away has somewhere to write; "posting counts as here" is client-side only; AT-16 is 20 queue changes of any kind; local dev uses a Neon `dev` branch + `tools/dev.sh`; FR-QUEUE-10..12 and the PWA moved to the P4 stretch list; public FAQ pages show a count of askers, names only in the room; SDK calls get a 3 s timeout. PHASES P2 has a cut order if it slips.
- **Owner: "do not do exact copy".** Design v4 drops everything lifted from the two references (pill nav, shrinking photo, ticker, bento, torn paper) and uses our own concept, "one session, hour by hour". See `docs/UI.md` §1 v3 for the list of what is and isn't taken.
- **3 new pins on the owner's board** (brush-stroke reveal poster, dot-matrix over a soft photo, RGB-fringed flower on black) became three treatments: painted reveal on the hero, dot-matrix clock + dot reveal in the story, RGB-fringed night photo in the closing.
- **Images:** 4 Figma AI generations in total (afternoon, golden hour, dusk, night), all first tries; `night-ca.webp` derived locally. Files in `../design/assets/`.
- Demo v4 verified: self-test 31/31 PASS, no console errors; screenshots checked for hero, story (incl. mid-transition), closing, phone, dark.
- **Still for the human:** install Go 1.25+; add `NEXCONN_API_BACKUP_URL` and a Neon `dev` branch URL to `.env`; line up 2 friends for the 3-person tests; approve the demo.

## 2026-11-12 09:39 IST: room redesigned to match the landing (demo v5)
- Owner: "hero ok but room is in still old design". The hero is approved; the room now uses the same language. Spec: `docs/UI.md` §2 S3.
- Room header: session headline with the status as an italic serif word; the desk scene as a painted strip whose light follows session progress (afternoon → golden hour → dusk → night; Last call and ended = night), computed client-side from `started_at` + `planned_minutes`; elapsed time as a dot-matrix clock.
- Panels float on paper without dividers; Chat UI sits in a white rounded card; "my place" is a selection box; empty states use a small painted strip. Breakpoints moved to 1240 / 980 px because of the 12 px gaps.
- Bug found and fixed while checking: `overflow: hidden` on the now-answering card let it shrink to a sliver in the scrolling queue on phones (`.q-body > * { flex-shrink: 0 }`).
- Verified: self-test 31/31 PASS, no console errors; screenshots of live (contributor, maintainer), your turn, last call, ended, no session, tablet (1024) and phone.
- Still waiting on the owner: approval of the room; Go install; `.env` values; 2 friends for the 3-person tests.

## 2026-11-12 10:24 IST: room rebuilt around a photo queue panel (demo v6)
- Owner on v5: "rooms not looking good". Cause: the layout was still three plain columns with a thin painted strip on top, so the scene was decoration. A second, unrelated problem: the strip's brush texture and dot clock were too small to read.
- v6 (spec: `docs/UI.md` §2 S3): two columns. Left: session headline + Chat UI in a white card. Right: a 400 px portrait photo of the scene, lit by session progress, with the queue floating over the sky as glass cards and a window kept free so the desk stays visible. Phones and tablets (< 1040 px) keep tabs; the photo fills the queue tab.
- 4 more Figma AI generations (portrait: day, golden hour, dusk, night), all first tries: `../design/assets/panel-*.webp`. Total so far: 8.
- End session moved to the header for maintainers, so it now opens a confirmation sheet with the carry-over checkbox.
- The left column (project info, recent answers) is gone on desktop; the FAQ button covers it. It remains as the FAQ tab on phones.
- Verified: self-test 31/31 PASS, no console errors; Retina screenshots of live (both roles), in line, your turn, last call (night), ended, no session, empty queue, signed out, 1280×720, and phone.
- Owner offered Blender (5.4 alpha is installed). Not used: the photos already line up and the problem was layout. It would be worth it for motion (a looping scene) or an exactly matched set; candidate for P4 or the P6 video.

## 2026-11-12 10:30 IST: design approved
- Owner approved the UI demo v6: "hero ok" (09:26) and, after the room rebuild, "ok good" (10:29). `docs/UI.md` is the spec for P1 UI work; the demo file and its `assets/` are the visual reference.
- P0 is now blocked only on setup: Go isn't installed, nothing is committed yet, and `.env` still needs `NEXCONN_API_BACKUP_URL` and a Neon dev branch URL.

## 2026-11-12 10:51 IST: first commit and app skeleton
- **First commit on `main`: `26626e5`** (docs, notes, tools, Claude config). Secret check before it was clean: no key-like strings, `.env.example` has no values, `.remember/` added to `.gitignore`. No remote yet; nothing pushed.
- Go was already installed through Homebrew (`brew install go` answered "go 1.27.4 is already installed"). Node 26.10, npm 11.19.
- **Skeleton on branch `slice/p0-skeleton`, not committed yet:**
  - `server/`: Go module `officehours` (`go 1.25.0`, stdlib only). `/api/health`, `/api/config` (public App Key + area), the two FAQ routes as placeholders with owner/repo/slug validation (NFR-SEC-3), and `/api/probe/cookie` for A-10 (delete when the session cookie lands in P1). Tests written first.
  - `web/`: Vite 8 + React 19 + TypeScript 6 (strict), ESLint 10, Prettier defaults, Vitest 5. One page that shows whether the Go server answers; dev proxy for `/api`, `/auth` and `^/p/[^/]+/[^/]+/faq` to :8080.
  - `vercel.json` (Services: `web` + `go`), `.github/workflows/ci.yml` (gofmt, vet, go test, web lint/typecheck/test/build, secret scan).
- **Checks (real output):** gofmt clean · `go vet` ok · `go test` ok, 18 cases, 87.2% coverage · web lint ok · typecheck ok · Vitest 6 passed · build ok (220 kB JS, 69 kB gzip).
- **Local run** (`tools/dev.sh` on :8080 + Vite on :5173): 10/10 route checks pass (proxy, FAQ routes reach Go, bad owner 404, SPA deep link, cookie round trip with HttpOnly + SameSite=Lax, config read from `.env`, wrong method 405).
- **A-10, locally** with `vercel dev -L` (CLI 64.3.1, no project link): 12/12 route checks pass on one port: `/api/*`, `/auth/*` and the FAQ routes reach Go, SPA deep links return `index.html`, a cookie set by Go comes back after a web route.
  - Finding: the Go service must set `"framework": "go"` in `vercel.json`. Without it the CLI detects only the Vite service and `/api/*` fails with "Cannot route to service `go`: it is not running".
  - Side effect: the CLI downloaded its own Go 1.27.1 for its builder (into its cache). `vercel dev` also reads `.env`.
- Corrected a documented command: types are checked with `npm --prefix web run typecheck` (`npx --prefix web tsc --noEmit` finds no tsconfig from the repo root).
- **Left for the next P0 steps:** push and link Vercel + Neon (human), then repeat the A-10 checks on a real deployment with one pgx query and cold-start timings; add pgx + `cmd/migrate`; the Nexconn Go SDK spike (A-2 rest, A-4, A-11); the Chat UI spike page (A-3, A-5, A-8).

## 2026-11-12 11:09 IST: GitHub repo created
- `github.com/Vishal4742/nexconn-office-hours`, **private**, added as `origin`. Name follows the owner's convention (lower case, hyphens, sponsor or platform first, like `dabao-tripwire` and `agentid-kya-solana`) and matches the memory namespace.
- Private until the publish checks in `CLAUDE.md` (`/security-scan`, agentshield) have run; it must be public for the submission. Branch protection needs the repo to be public on a free plan.
- Nothing pushed yet at the time of this note.

## 2026-11-12 11:20 IST: main pushed, Nexconn server calls proven, database check
- **`main` pushed** to the private repo (`26626e5`). The skeleton and everything below are still uncommitted on `slice/p0-skeleton` (the owner chose push + packages, not the commit/PR yet).
- **Packages added** (owner's OK): `github.com/NexconnAI-Dev/nexconn-server-sdk-go` v0.1.2 (MIT) and `github.com/jackc/pgx/v5` v5.13.0. `go.mod` stays at `go 1.25.0`.
- **`server/nexconn`** (the only Nexconn caller): `IssueToken`, `CreateGroup`, `JoinGroup`, `Send`, `Mute`, with its own 3 s timeout. Tests written first against a fake API; they pin the wire format: path, `App-Key`/`Nonce`/`Timestamp`/`Signature = sha1hex(secret+nonce+timestamp)`, `shouldPersist` sent as 0 or 1, `disablePush` + `disableUpdateLastMsg` on the ping, `hasMention`.
- **Confirmed from the SDK source** (`configuration.go`, `client.go`, the models): `SetNexconnCredentials`, `SetPrimaryBackupDomains` (both required; we pass the primary twice when no backup is set), `Configuration.HTTPClient` (defaults to `http.DefaultClient`, no timeout), `ShouldPersist *int32` (a pointer, so 0 is sent), HTTP ≥ 300 becomes `GenericOpenAPIError`, a 200 reply carries `code`. Never set `Debug`: it logs whole requests.
- **Confirmed from the docs** (`platform-chat-api` pages): text is `RC:TxtMsg` with `{"content":"…"}`; @all is `mentionedInfo: {type: 1}` inside `content` plus `hasMention: 1`; success is `code: 0`; custom types must be registered in the client, need `pushContent` for push, max 32 chars, content max 10 KB; send limit 20 group messages per second; channel ids are letters and digits, max 64; join takes up to 100 ids.
- **Real run** (`set -a; . ./.env; set +a; go -C server run ./cmd/spike`), 10/10 PASS, three runs: tokens for `oh_bot`, `ohspikea`, `ohspikeb` (84 chars each); group `ohspike1` created; both users joined; `RC:TxtMsg` with @all; stored `OH:Hand` with push text; `OH:Session` with a mention; transient `OH:Queue`; 1 minute mute.
  - Creating an existing group answers **HTTP 400, code 25409**. `CreateGroup` now treats that as done, so registration can be retried (FR-PROJ-2). Joining twice is fine.
  - Test data now in the Nexconn app: users `ohspikea` and `ohspikeb`, group `ohspike1` with 12 test messages. The browser spike will use them.
- **Results in ARCHITECTURE §8:** A-2 ✅ (server side), A-11 ✅, A-4 server half ✅ (two-browser check open), new **A-12** (does a mention on a custom `OH:Session` message badge the member? the docs only describe mentions for text messages). Group ids change from `p_<id>` to `p<id>`.
- **Database check:** `server/store.Open` (pgx pool, max 2 connections, idle 10 s, connects on first use) and `/api/health` now returns `db: up | down | off`. Checked for real: local Postgres 18 → `up`; dead port → `down` (logged without the password); no `DATABASE_URL` → `off`. `TEST_DATABASE_URL=… go -C server test ./store/` runs the real-database test; it is skipped otherwise.
- **Checks:** gofmt clean · vet ok · 40 Go cases pass, 1 skipped (real database) · coverage: main 85.7%, nexconn 96.6%, store 88.9% · web lint, typecheck, 6 tests, build ok · the CI secret scan finds nothing (test fixtures no longer look like real database URLs).
- **Next in P0:** the browser half (Chat UI spike page: A-3, A-4, A-5, A-8, A-12), which needs Nexconn's browser packages; then Vercel + Neon and the A-10 checks on a real deployment.

## 2026-11-12 11:50 IST: browser half of the Nexconn test (Chat UI spike)
- **Built** (uncommitted, branch `slice/p0-skeleton`):
  - `web/src/spike/`: `chat.ts` (the SDK and Chat UI as one module-level singleton), `bubbles.ts` (the 3 card elements), `SpikePage.tsx`, `spike.css`. Route `/spike?user=a|b`, lazy-loaded; `&mount=detail` uses Chat UI's full channel view, `&open=0` connects without opening the room.
  - `GET /api/spike/token?user=` on the Go server: answers only when `SPIKE=1`, only when `APP_URL` is a local address, and only for `ohspikea` / `ohspikeb`; 404 otherwise. Delete it with the spike.
  - `server/cmd/spike` gained an `OH:FAQ` card, a forged `OH:Hand` sent as `ohspikea`, and one optional argument (`text | hand | session | faq | ping | forged`) to send a single message.
  - Packages (owner's OK): `@nexconn/chat` 26.2.1, `@nexconn/chatui` 26.2.5, `@nexconn/engine` 26.2.1, `lit` 3.3.3.
- **Rerun:** `SPIKE=1 bash tools/dev.sh`, `npm --prefix web run dev`, open `http://localhost:5173/spike?user=a` and `http://[::1]:5173/spike?user=b` (two origins keep two separate SDK stores, like two browsers), then `set -a; . ./.env; set +a; go -C server run ./cmd/spike`.
- **Results** (real runs, Chrome, two users, dev server and `vite build` + `vite preview`):
  - A-3 ✅ `OH:Session`, `OH:Hand` and `OH:FAQ` render inside bubbles, live and from history. `OH:Queue` shows no bubble.
  - A-4 ✅ pings arrive live in both browsers: 237, 255, 259, 299, 333, 364, 395, 738, 853 ms after the server sent them (median 333). They don't count as unread and aren't in history. NFR-PERF-1 allows 1,000 ms for ping + fetch, so the fetch has about 150 to 650 ms; if AT-16 is tight, put the queue snapshot in the ping.
  - A-5 ✅ one init per page load with Strict Mode on; the production build works. Chat UI chunk: 1.44 MB, 321 kB gzip.
  - A-8 part ✅ a 390 px viewport fits with no sideways scroll. Not tried on a real phone yet.
  - A-9 follow-up ✅ `nc-toast-provider` + `nc-message-list-provider` + `nc-input-provider` mount fine without Chat UI's header and without its 600 px minimum width.
  - A-12 ✅ with the room closed: baseline "0 unread, no mention" → `OH:Hand` → "1 unread, no mention" → `OH:Session` with @all → "2 unread, mentions me" → `OH:Queue` → still 2.
  - FR-QUEUE-7 ✅ the forged card shows "Ignored: this card was not sent by Office Hours" and the listener counted it (1 per forged card), in the other user's browser.
  - FR-MOD-1 ✅ a muted user's send fails with code 22408 (`FORBIDDEN_IN_GROUP`); the bubble gets a red "!" with no reason. After the minute it sends again.
  - Two-way chat ✅ a typed message was in the other browser when checked 2 s later, with the right name.
  - One user on two pages of one origin ✅ and on two origins (separate storage, same machine) ✅: both stay connected and both receive. A second real device is untested.
- **Confirmed browser API** (from the installed type files and the bundle, then by running it):
  - `NCEngine.initialize({ appKey, areaCode: AreaCode.SG, logLevel })`. `AreaCode` is `BJ 1, SG 2, NA 3, SA 5, OM 6`; our `NEXT_PUBLIC_NEXCONN_AREA=sg` maps to it.
  - `app.registerCustomMessages([{ messageType, isPersisted, isCounted, isStatusMessage?, digest?, component? }])` before `ready()` registers with the engine too. The element receives `.message`; `message.senderUserId` and `message.content` are what the card reads.
  - `NCEngine.connect({ token })` → `{ isOk, code }`. `app.openChannel(new GroupChannelIdentifier(id), true)` answers 39005 (`CHANNEL_LIST_NOT_READY`) for a moment after connect: retry every 250 ms.
  - `MessageHandler({ onMessageReceived({ messages }) })`. The types also list `onOfflineMessageSyncCompleted()` (not used yet).
  - `BaseChannel.getUnreadChannels([ChannelType.GROUP])` → channels with `unreadCount`; `BaseChannel.createUnreadMentionMeChannelsQuery({ channelTypes }).loadNextPage()` → rooms with an unread mention. Both answer 35011 (`METHOD_NOT_AVAILABLE`) until the channel list has synced.
  - Profile fields are `{ userId, name, avatarUrl }` (the examples in the type file say `portraitUri`; the code reads `avatarUrl`).
- **Findings that change P1 and P2:**
  1. **Sender names.** The bot first showed as `oh_bot`. Cause, read in the bundle: on `CHANNEL_SELECTED` Chat UI builds its member list from `requestUserProfiles()`, which returns the cache's placeholder (`name = userId`) and only starts the real request; the list copies `name` and `avatarUrl` and nothing refreshes it when the hook answers. Fix that works: `app.updateUserProfile()` for every member before `openChannel()`. A later `app.updateGroupMembers()` rebuilds the copy. A sender missing from the copy shows as the raw user id with the group's avatar. Not run, read only: in the update path nicknames are matched by index after the current user is filtered out, so they shift; we leave `nickname` unset.
  2. **Replay.** A client receives what it missed when it reconnects, pings included. An origin the user had never used received 16 cards and 8 pings through `onMessageReceived` on its first connect: every OH message since the test group was created 30 minutes earlier. How far back this goes is unknown. Nothing may trigger a sound, a banner or a refetch from a message alone: compare `version`, keep one fetch in flight, take "your turn" from the fetched state.
  3. **Unread badge (AT-4).** The data exists (see A-12), but the design has no place for it yet: the room has no channel list. Decide in P2, most likely the dashboard's project rows. "A session is live" should come from our database, the unread count from Nexconn.
  4. **Mute.** The UI has to explain it (a line above the input), since Chat UI only shows the red mark.
  5. **Loading.** Connect takes 1.2 to 1.5 s (5.2 s once, the first connection from a new origin) and history a moment more: the chat card needs a loading state.
  6. A test tab reloaded by itself twice during the run. Cause unknown: Vite logged no reload, so it may be the browser automation. The page came back and received what it had missed, so nothing was lost.
  7. Not checked: the browser SDK also exposes `GroupChannel.createGroup`, `join`, `kickMembers` and `dismiss`. Our rooms are public, so a direct join only bypasses our `members` table (the queue API still checks it). A ban would need the group's join permission; mute is all FR-MOD-1 asks for.
- **Console noise, harmless:** `A-get_conversation_list_by_time-R {"code":35011}` once per connect (the channel list isn't synced yet); `A-sync_read_status-R {"code":22408}` while a user is muted.
- **Test data:** `ohspike1` now holds about 40 test messages. Nothing else was created.
- **Reviews from a fresh context** (code and security, after the run): nothing critical or high.
  - Fixed: the unread check's timer had no error handler and ran in open rooms too (now closed rooms only, one timer, errors caught); a non-JSON error page gave a parse error in place of the status; the sender check lived in two places (now one `isFromBot()` in `bubbles.ts`); a typo in the test sender's argument printed "all server steps passed" with nothing sent (now refused).
  - Fixed, test first: **`SPIKE=1` only counts when `APP_URL` is a local address** (`localhost`, `127.0.0.1`, `::1`). Anywhere else it is ignored with a log line, so a stray variable on a deployment can't hand out tokens. Checked with the built server: public address → 404 and the log line; local address → the endpoint opens; no address → 404.
  - Confirmed safe as written: no token, secret or signature reaches a log, an error string, a URL or a cacheable response (`Cache-Control: private, no-store`); HEAD returns no body; other methods get 405; the allow-list can't be passed with an encoded or repeated `user`.
  - Carried to P1, not built into the spike (marked with a `ponytail:` comment in `chat.ts`): split "initialize once" from "open room(id)", so a failed connect can be retried without a page reload and a second room can open; make the SDK module survive a hot edit (reload the page, or keep the promise on `globalThis`).
  - Left out on purpose: type checks on `/api/config` (they come with the real client config), extra tests and a rate limit for an endpoint that is about to be deleted.
  - Re-run after the fixes: token endpoint 200 for a test user and 404 for the bot on the local server; the room opens and shows history; with the room closed one `OH:Hand` gives "1 unread, no mention"; no console errors.
- **Checks (final):** gofmt clean · vet ok · 62 Go cases pass, 1 skipped (real database) · coverage: main 86.8%, nexconn 96.6%, store 88.9% (`cmd/spike` is a throwaway with no tests) · web lint, typecheck, 6 tests, build ok.
- **Next in P0:** link Vercel + Neon (human), deploy, repeat this run and the A-10 checks on production, try a real phone, then close P0 and delete the spike code, the token endpoint and the cookie probe.
- **The gate's "on production" half can't use the spike endpoint any more** (it only opens on a local address, see the reviews above). Options for the owner: run that half with real accounts as soon as P1's sign-in is deployed (my recommendation: the browser talks to Nexconn directly, so the local run with the production build already covers the Nexconn side, and A-10 covers the Vercel side), or relax the guard for a Vercel preview that asks for a Vercel sign-in.
- **Open question for the P5 security pass:** a member's token also works outside our UI, for direct messages to any user id and for the SDK's own group calls. Look in the Console for switches that turn off direct channels and client-side group creation.

## 2026-11-12 12:17 IST: the meadow scenes in motion (Higgsfield)
- The owner installed and signed in to the Higgsfield CLI and picked "Hero loop + day to night" (up to 60 credits). Made from the four approved landscape pictures; nothing is wired into the app or the demo yet.
- **Made** (outside the repo, in `../design/assets/video/`):
  - `hero-day-loop.mp4`: the day scene with clouds and grass moving, 1280×720, 10 s, 0.70 MB. It plays forward then backward, so it loops without a jump. For the landing hero, with the still as its poster.
  - `day-to-night.mp4`: day → golden hour → dusk → night in one 14.6 s shot, 1280×720, 3.58 MB. `day-to-night-1080.mp4` is the full-size copy (18.6 MB) for the demo video.
  - `raw/`: the four clips as generated (1920×1080, 24 fps, 5 s each, no sound).
  - `../design/video-preview.html` shows both, with buttons that jump to each of the four scenes.
- **How:** model Kling 3.0 Omni Frames, 5 s, pro mode, sound off, 6.25 credits a clip. 5 clips, **31.25 credits** (balance 3,825.86 → 3,794.61).
- **Findings:**
  - A start picture plus a different end picture gives a smooth time-lapse that ends almost exactly on the end picture, so the three transitions join with a 0.25 s crossfade and each stop is one of the approved stills.
  - The same picture as start and end gives a frozen clip (one wasted generation). A loop needs the start picture only, then forward-and-backward playback.
  - The framing drifts a little between scenes because the four stills were not shot from one camera. It reads as a slow pull-back.
- **Checked:** frames pulled from every clip and from both joins; the page was opened in Chrome through a local server: both videos load, the loop plays, the scene buttons land on dusk and night.
- **Not decided:** whether the landing uses the moving hero and the day-to-night shot (P4 polish), or only the demo video does (P6). The owner looks at the preview first.

## 2026-11-12 12:26 IST: landing cleaned up (demo v7)
- The owner, with three screenshots of the landing: "remove this … and others this types also". The three were the mock queue cards floating over the hero photo, the header strip "Nº 018 · Thursday office hours · live now" and the caption "fig. 01 · 17:00, the desk is open · paperplane/stash".
- Removed from the landing in `../design/office-hours-demo.html`, with their styles: those three, plus the same kinds elsewhere on the page: "fig. 05 · 21:00", the corner labels on the sky band ("[ why ]", "archive by @mia-k"), the section labels with counters ("[ 001 / 003 ] …"), the hero's "[ open source, answered live ]" and the "( 01 )" numbers on the benefits. The hero photo now has a text alternative, since the caption is gone.
- Kept, as my reading of "these types" (decoration, not information): the story's times and dot clock, the small example cards inside the story steps, and every label in the room and on the FAQ pages, which show real state.
- Checked in Chrome: dark, light and phone size; the self-test still passes 31 of 31; no console errors. Backup of the approved v6: `../design/.work/demo-v6-approved.html`. `docs/UI.md` §1, §2 S1 and §8 now describe v7.

## 2026-11-12 12:45 IST: Phase 0 code committed (commit routine)
- **Owner's git decision** (dialog): commit in small commits, push, open a pull request and merge when CI is green, for Phase 0 and for each Phase 1 piece.
- **Gate before the commit** (fresh run): gofmt clean · vet ok · Go tests pass (main 86.8%, nexconn 97.1%, store 88.9%) · web lint, typecheck, 6 tests and build ok.
- **Lean review of the diff:** nothing blocking. Applied: the Nexconn wrapper now reads the reply code through the SDK's nil-safe `GetCode()` (five copies of a nil check gone, 18 lines fewer).
- **Finding carried into Phase 1:** `CreateGroup` treats "this id already exists" as success, so room ids must be impossible to guess. If they were `p1`, `p2`, … any signed-in user might create that group first from the browser SDK and own a project's room. Phase 1 gives projects random ids. The code says so in a comment.
- **Grill:** the owner got the room-id and secrets questions right, and thought the test token endpoint was open everywhere. It isn't (`SPIKE=1` and a local `APP_URL`, both tested). After the correction the owner chose: keep the endpoint, the `/spike` page and the test sender until the real sign-in and room work, then delete them in that pull request.
- **Still open for Phase 0's gate:** Vercel + Neon linked (owner), the A-10 checks and a real phone on the deployment. Phase 1 starts in parallel.
