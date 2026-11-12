# Phase 0 — spike and setup (in progress)

## 2026-11-12 07:45 IST — credentials and memory
- `.env` created (git-ignored, mode 600): Nexconn App Key + App Secret, generated `AUTH_SECRET`, supermemory + extraction-model keys copied from `~/ethglobal/.env`. Empty still: `AUTH_GITHUB_ID`, `AUTH_GITHUB_SECRET`, `DATABASE_URL`.
- Local supermemory server for this repo is up: `http://localhost:6768`, data in `.supermemory/` (encrypted local storage, local embeddings bge-base-en-v1.5). ETHGlobal's server on 6767 is separate and untouched.
- **A-2 (part) ✅ 2026-11-12:** the key pair works on the **Singapore** data center (`https://api.sg-light-api.com`; us and sau not needed). Signature = `sha1hex(AppSecret + Nonce + Timestamp)`, headers `App-Key`, `Nonce`, `Timestamp`, `Signature`.
  `POST /v4/auth/access-token/issue {userId, name}` → `{code: 0, result: {userId, accessToken}}` (token length 84). `oh_bot` ("Office Hours") is registered.
- `NEXT_PUBLIC_NEXCONN_AREA=sg` is a placeholder: confirm the client SDK's data-center option/enum name in the spike page.

## Still to check (ARCHITECTURE §8)
- A-1 plan in the Console (challenge plan vs default one-month Free) + app creation date — human
- A-2 rest: group create/join, group message send (custom type, unpersisted status message, @all), mute
- A-3, A-4, A-5, A-8 in the spike page; A-6 translation (stretch); A-7 GitHub permission check (P1)

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
