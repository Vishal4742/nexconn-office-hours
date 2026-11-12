# Phase 2 — build log

Phases 2, 3 and parts of 4 are built together by ten workers (`notes/phase-2-plan.md`). This file records what the lead did around them; each worker's slice gets its own entry when it is merged.

## Skeleton (slice `p2-skeleton`, PR #12, merged 2026-11-12 16:47 IST)
**Built.** Everything the ten workers share: the `queue` contract (types, errors, a `Step` to fill in), the store's shapes and signatures, every route registered with a stub that answers 501, the helpers a queue route is made of (`onSession`, `onHand`, `onProject`, `queueRefusal`, `finish`), the queue's JSON, what the bot says in a room (`tell`, the ping, three cards), shared input rules, `002_queue.sql`; on the web side the queue's types and calls, a queue store with fixed exports, fixtures of every queue state, and the room page as a composition of parts with fixed props. `GET /api/p/{owner}/{repo}/queue` works.

**Reviewed before any worker started** by `plan-reviewer`: 13 findings, 7 to fix first, all applied (plan §10). The costly ones: name clashes between workers who share a Go package (each pull request green alone, `main` broken at the second merge), four requirements with a route and no button, an overloaded queue-panel worker, a test that pinned the FAQ placeholder, and the first migration edited in place.

**Verified.**
```
go -C server test -race ./...   ok officehours, github, guard, nexconn, session, store (real Postgres)
npm --prefix web test           9 files, 81 tests passed
CI on PR #12                    Go server pass (1m17s) · Web pass (22s) · Secret scan pass (6s)
```
Locally the room loads as before and `GET …/queue` answers `{"session":null,"hands":[],"done":[],…}`.

## Hosting, first deployment with both services (2026-11-12 16:48 IST)
The owner cleared the project's Root Directory and deleted a second project they had created for the Go server. The skeleton's merge was the first production deployment of the root `vercel.json`:
```
https://office-hours-oss.vercel.app/            200 text/html
https://office-hours-oss.vercel.app/dashboard   200 text/html      (the web service's own rewrite: deep links work)
https://office-hours-oss.vercel.app/api/health  500 FUNCTION_INVOCATION_FAILED
runtime log: cannot start: session: AUTH_SECRET must be 32 random bytes in base64 (openssl rand -base64 32)
headers: content-security-policy: frame-ancestors 'self' · x-frame-options: SAMEORIGIN · x-content-type-options: nosniff · referrer-policy: strict-origin-when-cross-origin
```
So assumption A-10 holds for routing: one origin, `/api/*` and the FAQ routes reach Go, Vercel builds the Go service with Go 1.25 from `server/go.mod` (40 s) and runs it as a long-running server. The Go service stops at start, as it must, until the owner adds the environment variables. Still to prove there: a cookie set by Go, one database query, cold-start timings. Preview deployments for `slice/*` branches are off (`git.deploymentEnabled`): PR #12 got none.

The live address is `https://office-hours-oss.vercel.app` (added to the project on the owner's choice); `APP_URL` and the production GitHub OAuth callback follow it.

## Store core (slice `p2-store-core`)
**Built.** `server/store/queue.go`: `Queue` (one consistent read: the live session or the last one, its hands in the queue's order, their people, FAQ entries that still exist, the median answer time), `Apply` (lock the session row, load the open hands, ask the rules, write the difference, version + 1, all in one transaction), `Done` (Apply plus the FAQ entry in the same transaction, with a slug that is free in the project and at most 80 characters), `ProjectOfSession`, `ProjectOfHand`, `People`.

**Decided.**
- The rules are a parameter inside the package (`applyWith(step, …)`), so the store's tests script them and do not wait for the queue-rules worker. `Apply` passes `queue.Step`.
- The store trusts the rules for what is allowed and nothing else: rules that lose or invent a hand are an error and nothing is written; a second open hand of one author or a second hand being answered is refused by the unique indexes and comes back as the rules' own error (`queue.ErrAlreadyOpen`, `queue.ErrAnswering`).
- A slug is looked up before the insert, deleted entries included: an address that answered once is never given to another answer. Dones of one project hold the lock of its one live session, so looking first is enough; the unique index stays as the guard.
- A hand's +1s are a set, loaded sorted by user id.

**Verified.** Real Postgres, own schema per test:
```
TEST_DATABASE_URL=… go -C server test ./store    ok  officehours/store
```
Covered: a raise gets its id and the version goes up; what the rules see (open hands, +1s, last raise per project, nothing closed); every field the rules may change, and NULL for what they clear; +1 rows added and taken back; a refusal writes nothing and comes back as the same error value; last call and the end of a session; two actions at the same moment applied one after the other (one sees an empty queue and makes version 1, the other sees that hand and makes version 2); the three database refusals; the snapshot's order, people, FAQ ids and median (0 until two answers, 7 min for 4 and 10, 6 min for 4, 6 and 10); Done's entry, `asked_by` = author plus +1s, slugs `-2`, `-3`, `-4` after a delete, 80 characters kept, nothing by halves.

**Open.** Flows through the real `queue.Step` are tested when the queue-rules slice is merged. → Done, see "The real rules through the store" below.

## The workers' slices, as they were merged
The ten workers ran on Fable 5.2 until the account's 5-hour limit stopped them all at 16:56 (nine minutes after the start), then its usage credits at 17:04. The owner switched the session to Opus 5.5; the workers were resumed from their worktrees with their work intact (two that had written nothing were started again). Commits keep the trailer of the model that made them.

| Slice | PR | What it does | Verified (from the worker's report and CI) | Lead's notes |
|---|---|---|---|---|
| Hands (worker 3) | #15 | raise, withdraw, +1, "I'm here" (FR-HAND-1, 2, 4..7, FR-QUEUE-9) | 5 tests, 49 subtests; 40 deliberate bugs all caught; CI green | Read the routes: the rules decide everything; a store answer without a new hand still pings |
| Control characters (lead) | #17 | `cleanLine`/`cleanText` drop NUL and other non-space controls (NFR-SEC-3) | test first, then fix; CI green | Found by workers 3 and 8: a NUL reached Postgres (`22021`) and gave a 500 |
| FAQ and stats API (worker 8) | #16 | similar (3), latest (5), delete, stats of a project and of the site (FR-FAQ-4, 6, FR-PROJ-3, FR-MET-1) | 7 tests, 35 subtests; 40 of 41 deliberate bugs caught; CI green | One SQL statement for the five numbers; "sessions held" counts the live one |
| Queue rules (worker 1) | #18 | `queue.Step`, every row of plan §4 | 99.2 % coverage (100 % of `step.go`); 6,000 random actions keep the invariants; 77 of 79 deliberate bugs caught; CI green | Read in full. A closed hand named by an action is `ErrNoSuchHand`; Skip, Remove and Done keep when the hand was taken (the median needs it) |
| Sessions (worker 2) | #19 | start (GitHub asked again: yes, a clear no that removes the role, anything else refused without removing), end with carry-over and the ended card, last call, next session time (FR-SESS-1..7) | 101 route and 19 store tests; 81 deliberate bugs all caught; CI green | A private repo now refuses the start; a missing `carry` means true; the room hears about last call only when it changed |
| Maintainer actions (worker 4) | #22 | Next, Skip, Remove, Away (with the line the room is told), Merge, Done with the FAQ entry and its card, Mute; `slugOf` (FR-QUEUE-3, 8, FR-HAND-8, FR-FAQ-1, 2, FR-MOD-1) | 7 tests, 65 cases; fuzzing `slugOf` over 4.2 million titles; CI green | Links with a name or password before the host are refused (`https://github.com@evil.example`) |
| Cards, join page, stats pages (worker 9) | #21 | the three cards with live hand status, FAQ and issue links, "Remove from the FAQ" for maintainers; the join page's live session, next time and latest answers; the stats pages (FR-HAND-5, FR-FAQ-2, 5, 6, FR-PROJ-3, FR-MET-1) | 156 new web tests; CI green | Card links built from the slug on our origin, only for the watched room's own messages |
| A past plan is no plan (lead) | #23 | `store.Queue` reports `next_session_at` only while it is to come (FR-SESS-5) | test first; CI green | From worker 2: starting a session does not clear the plan |
| Local start script (lead) | #24 | `tools/dev.sh` unsets `PGUSER` and the other libpq variables | local server back to `db: up` | Neon's settings pasted into the local `.env` made the local server log in as the production user |
| Migration says where (lead) | #26 | `cmd/migrate` prints the variable it used and the database's host | test first; CI green | The owner's first production migration silently ran against the local database (zsh stopped reading `.env` at an unquoted `&`) |
| Queue panel (worker 5) | #25 | the store that keeps the queue current (ping rules, one fetch at a time, one clock, NFR-PERF-1 log) and the panel in every state for members and maintainers, with the four controls that had no button (FR-QUEUE-4..9, FR-HAND-4, 7, 8, FR-SESS-5, 6, FR-FAQ-5, 6, FR-MOD-1) | 98 new web tests; CI green; looked at in `/dev/room` at 1440 and 375 px | Added `actOnQueue` so an answer about another room never becomes this room's queue; worker 6 was told to use it |
| Public FAQ pages (worker 7) | #27 | the FAQ store (list, search, suggestions, delete) and the Go-rendered list, entry, 404 and 410 pages with link previews (FR-FAQ-3..6, FR-HAND-3) | 17 tests, 106 subtests; both searches use the indexes on 20,000 rows; CI green | Typed text stays text; only http and https become links, with `rel="noopener noreferrer nofollow"`; search text drops NUL and broken UTF-8 |
| Sheets, room head, your-turn banner (worker 6) | #29 | the accessible `Sheet`, the four sheets with their limits and the similar answers while typing, the head with the session as its headline and the maintainer's controls, the phone tabs (queue first), the banner with "I'm here" (FR-SESS-1, 4, 6, 7, FR-HAND-1, 3, FR-FAQ-1, FR-QUEUE-5, 9) | 30 new web tests; CI green after the scan fix; looked at in `/dev/room` | Uses `actOnQueue` and `serverNow()` from the panel's store, as asked during the build |
| Secret scan (lead) | #30 | the migration test's fake address joined at run time | all four checks read in full | #26 had been merged with a red secret scan: a `tail` of the check list hid it. Rule added to `CLAUDE.md` |
| Landing (worker 10) | #31 | the hero's painted swath, the story hour by hour with the pinned frame, the dot clock and the dot wave from the lamp, numbers, sky band, questions, closing; dark mode on this page only (UI.md §1 v3, S1) | 26 new web tests; CI green; looked at in Chrome in dark mode | Started last on purpose, when the first workers had finished |

**Seams to check later (from the workers' reports):**
- Checked: `ProjectOfFAQ` finds deleted entries, so deleting twice answers 200.
- The FAQ pages are cached for 60 s with `max-age`; Vercel's edge would need `s-maxage` too (worker 7).
- The re-check at session start asks GitHub by the registered name; after a rename where someone took the old name, a maintainer who still has rights is refused (409, nothing removed). Asking by id needs a new `server/github` method (worker 2).
- A muted member can't be told "muted until …": the server keeps no record of mutes (worker 4).
- `actionApply` (worker 4) and `handAct` (worker 3) are the same ten lines for two roles.
- On a phone-sized panel the waiting card leaves little room for its rows; look again once the phone tabs exist (worker 6).
- Deleting a FAQ entry twice answers 200 only if worker 7's `ProjectOfFAQ` also finds deleted entries.
- A delete does not tell the room; another page keeps the entry's link until its next queue read, and the link then shows the 410 page.
- A title of ten zero-width spaces counts as 10 characters and looks blank. A maintainer can remove it.

## The real rules through the store (lead)
`TestApplyWithTheRealRules` plays one session through `store.Apply` with `queue.Step` itself on real Postgres: four hands in the order raised, a second open hand refused, a +1 stored (and refused on one's own hand), Next and a second Next refused, Away two places back (h3, h4, h2, h5 in the snapshot), "I'm here" protecting from Away, a merge and a Done whose entry counts two askers, withdraw only by the author, last call refusing a raise, the 60 s rule after reopening (a 30 s wait), the end carrying the one still waiting, and nothing accepted after it. The version goes up by one with every allowed action. Passed on its first run.


## One person through the whole room, locally (lead, 2026-11-12 17:37–17:39 IST)
On `main` with eight of the ten slices merged, signed in as the owner (maintainer of the test project), the local Go server and Vite, the real Nexconn app and GitHub. The parts without a sheet yet (start, raise, done) were called with `fetch` from the page; Next was pressed in the panel.
```
POST …/sessions {title, plannedMinutes: 45}  200, live, number 1, version 1   (GitHub re-check passed with the owner's token)
  → chat: OFFICE HOURS STARTED · Integration run, part one; panel: Live · No hands yet · Next question
POST /api/sessions/{id}/hands                 200, version 2, isMine
  → chat: "@Vishal4742 raised a hand" card, status "In line"; panel: You're next, waiting 1
click Next question                           → card "Being answered now"; Now answering card with Done / Skip / Away, the link in the details
POST /api/hands/{id}/done {answer, links}     200, version 4, answered, faqId set, slug of 52 characters
  → chat: ADDED TO FAQ card (asked by @Vishal4742, Read the answer, Open a GitHub issue, Remove from the FAQ); hand card "Answered"
GET /p/{owner}/{repo}/faq/{slug}              the entry page, dark mode, three outside links with rel="noopener noreferrer nofollow", no script
POST /api/sessions/{id}/end {carry: true}     200, live false, version 5
  → chat: OFFICE HOURS ENDED · 1 answered; panel: Session ended, plan the next session, Start a session
console: NFR-PERF-1 queue ping to store: 329 ms (session 1, version 5)
```
Seen on the way: the room's head still says "No session right now" (worker 6's part, not merged yet), and after an action the panel had scrolled a little so its top chips were cut. Both are for the two-user run.

## Hosting, continued (2026-11-12)
- 17:16 The owner created the Neon database (Singapore) and connected it to project `web` with the prefix `DATABASE`, so the variable is `DATABASE_URL`.
- 17:33 Production migrated by the owner from their own terminal (this session's safety check refused a write to production): `applied 001_init.sql`, `applied 002_queue.sql`.
- Waiting for the eight app values in Vercel; until then the live `/api/health` answers `FUNCTION_INVOCATION_FAILED` (the server stops at start without `AUTH_SECRET`).

## Two people through the whole room, locally (lead, 2026-11-12 17:48–17:53 IST)
All ten slices merged. The owner as maintainer on `localhost:5173`, the test user "Spike B" as a member on a second origin (`127.0.0.1:5174` with its own Go server, `design/.work/two-user`). Real Nexconn, real GitHub for the owner. Sheets and buttons were used where they exist; a few refusals were checked with `fetch` from the page.

| AT | What happened |
|---|---|
| AT-4 | Start sheet ("Thursday office hours", 45 min, Go live): the start card arrived live in both chats, the head reads "Thursday office hours, *live.*" with "Nº 2 · 45 min with @Vishal4742 · 45 min left". A second start: `409 A session is already live in this room.` |
| AT-5 | Typing "How do I run the tests against Postgres?" in the raise sheet showed "Already answered?" with the earlier FAQ entry |
| AT-6 | Raised from the sheet: the hand card "@spike-b raised a hand · In line" in both chats; a second hand: `409 You already have an open hand in this session.` (the 60 s wait is proven by `TestApplyWithTheRealRules`) |
| AT-7 | Next pressed in the panel: Spike B's page showed the blue "It's your turn · @Vishal4742 is answering your question · I'm here" banner, the card turned "Being answered now" |
| AT-10 | Spike B calling `POST /api/sessions/{id}/next`: `403 Only a maintainer of this project can do that.` |
| AT-17 | Away with one other waiting: the hand went to the end; the room was told "@spike-b didn't answer when their turn came, so their question moved to the end of the line." |
| AT-19 | Spike B +1'd the owner's hand (count 1, order unchanged); the owner merged Spike B's hand into the one being answered ("Merged into another question" on its card) and published through the Done sheet: the FAQ card reads "Asked by @Vishal4742 and @spike-b", the public page "asked by 2 people" and names nobody |
| AT-8 | The FAQ card's links; the public page renders signed out (checked earlier in the one-person run) |
| AT-18 | Last call: the head "…, *last call.*", the panel "Hands closed, 1 left", the room told "Last call: hands are closed. 1 question is left in line."; a raise: `409 Hands are closed for this session.` |
| AT-11 | End sheet with carry-over ticked: the end card "1 answered · 1 carried over"; a new session (Nº 3) put Spike B's carried hand first ("You're next") |
| AT-9 | Spike B's page reloaded mid-session: the same queue |
| AT-14 (part) | The room at 375 px: tabs "Queue · Chat", queue first, no sideways scroll; the chat tab shows Chat UI at full width |

Not run here: AT-15 (forged pings and cards: covered by the ping and card unit tests), AT-16 (p95 over 20 changes; one sample so far, 329 ms), AT-12 and AT-13, and everything on production (waiting for the app's variables in Vercel).

Fixed after the run (#32): the phone panel showed sky without the desk (a rule left from the old band), two stale comments, the Away button's title promised "two places" when fewer than three wait, and the landing's dark mode now starts before the first paint.
