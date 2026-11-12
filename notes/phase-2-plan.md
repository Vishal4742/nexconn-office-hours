# Plan — Phases 2, 3 and parts of 4, built by ten workers at once

**2026-11-12.** The owner asked for ten sessions working in parallel and chose the split "Phases 2, 3 and parts of 4" with the same git flow as Phase 1 (each piece on its own branch, a pull request, merged by the lead when CI is green and the pieces fit).

Ten people writing at once collide unless three things are fixed first: the shapes they share, who owns which file, and the order the pieces go in. This plan fixes them. A skeleton with every shared shape is merged to `main` before anyone starts (slice `p2-skeleton`), so each worker fills in files that already exist and compile.

## 1. Scope
| Phase | SRS | In this build |
|---|---|---|
| P2 | FR-SESS-1..4, 6..7 · FR-HAND-1..2, 4..6 · FR-QUEUE-1..9 · FR-MOD-1 · NFR-PERF-1, NFR-REL-1, NFR-SEC-2, 4, 6 | all |
| P3 | FR-FAQ-1..6 · FR-HAND-3, 7..8 · FR-PROJ-3 (session and FAQ part) | all |
| P4 | FR-MET-1 · FR-SESS-5 · FR-QUEUE-10..12 · landing story and its dark mode · phone tabs | these; the rest of P4 (state audit, contrast check, Firefox note) stays for the phase close |

**Not in this build:** FR-I18N-1, FR-NOTIF-2, FR-AUTH-5, FR-PROJ-5, the PWA manifest, FAQ editing. **A second slice, after the two-user run** (the plan's review found the queue panel's worker overloaded, and these are Should items or polish that need a browser to get right): the maintainer's keys `N D S A`, the timer that turns amber (FR-QUEUE-12), the place in the tab title (FR-QUEUE-11), the wait estimate (FR-QUEUE-10), the light that follows the session's clock with its three other photos, and the maintainer's page counting a post in the room as "here" (the client-side half of FR-QUEUE-9). **Cut on purpose:** the 5-second undo after Next, Done and Skip that the demo showed. The SRS has no reverse transitions, so an undo would be a delay before every action, and a queue that must feel instant (NFR-PERF-1) should not wait. Done already asks through its sheet; Remove asks once inline.

**Gates** (on production, once there is a deployment): P2 AT-4, 6, 7, 9, 10, 11, 15, 16, 17, 18 · P3 AT-5, 8, 19 · P4 AT-12, 13, 14. Until then the lead runs them locally with two users (see §7).

## 2. Decisions
1. **One rule function.** `queue.Step(State, Action) (State, error)` is pure and holds every rule (§4). The store loads a session's open hands inside a transaction, calls Step, and writes back what changed. Handlers never decide a transition.
2. **One writer.** `store.Apply` is the only code that changes hands of a live session: `SELECT … FOR UPDATE` on the session row, Step, write the difference, `queue_version + 1`, commit. `store.Done` is Apply plus the FAQ insert in the same transaction. `store.StartSession` is its own transaction (it has no state to step from).
3. **A queue route is three lines of decisions.** `onSession` / `onHand` / `onProject` (guard, the project behind the id, the role), one `queue.Action` through Apply, `queueRefusal` for what the rules refuse, and `finish` (tell the room, answer with the queue). Every mutation answers with the whole queue as the caller sees it, so the acting page is current before the ping arrives.
4. **The ping is a hint.** `OH:Queue {sessionId, version, sentAt}`. A page refetches only when the ping is newer than what it holds: a greater session id, or the same session with a greater version. Session ids only grow. One fetch at a time; a ping that arrives during a fetch asks for one more afterwards.
5. **Ids are strings on the wire**, in the JSON and in the cards, so a card's `handId` can be compared with a hand's `id`.
6. **The room is told after the commit, before the answer**, with a context that survives a caller who went away, and a failure to tell never fails the action (`tell`).
7. **A maintainer's GitHub rights are checked again when they start a session** (security review of Phase 1). If GitHub says no, the start is refused and their maintainer row is removed; if GitHub does not answer, the start is refused. `ponytail:` a co-maintainer who lost their rights can still act in a session someone else started until they try to start one; re-check per action on a timer if that ever matters.
8. **End is refused while a hand is being answered** (FR-QUEUE-2 has no way from `answering` to `carried` or `not_reached`). The maintainer finishes or skips it first.
9. **Someone outside the room gets the session and no hands** from `GET queue`: the join page can say "live now", and questions and names stay with members.
10. **A draft question never travels in an address.** The similar-questions search is a POST with the text in its body. The public FAQ page's own search is a normal `?q=` form.
11. **Removed and withdrawn hands stay in the queue's `done` list with their status**, so a hand card in the chat can say what became of it. A removed hand is listed without its title and details (`describeQueue` blanks them), and its card hides the title while the queue says "removed". The card's own text was posted to the room when the hand was raised; taking that message out of the chat is not in this build (see §8).
12. **The page counts time from the server's clock** (`serverTime` in every queue answer), not its own.

## 3. The contract (frozen until integration)
Merged in the skeleton. **Nobody but the lead edits these files.** If something a worker needs is missing or wrong here, they stop and say so in their final message; they do not work around it in a frozen file.

| File | What it fixes |
|---|---|
| `server/queue/types.go` | `Status`, `Hand`, `State`, `Kind`, `Action`, `RaiseEvery`, the errors Step may return |
| `server/store/types.go`, `server/store/queue.go` | `Person`, `Session`, `NewSession`, `QueueHand`, `Snapshot`, `Applied`, `FAQDraft`, `FAQEntry`, `Stats`, `ErrNotBuilt`, `ErrSessionLive`; the signatures of `Queue`, `Apply`, `Done`, `ProjectOfSession`, `ProjectOfHand`, `People`; and the two shared readers `personCols` and `sessionSelect` + `scanSession`, so no file of the package writes its own |
| `server/migrations/` | the schema: `001_init.sql` as it was, and `002_queue.sql` with three new columns (`sessions.started_by`, `hands.taken_by`, `faq_entries.asked_by`). No worker adds a migration |
| `server/validate.go` | `cleanLine`, `cleanText`, `lengthBetween` (characters, not bytes), `validOwnerRepo`, `slugRe`, `clip`: every route uses these and writes no trimming or counting of its own |
| `server/main.go` | every route, `deps` |
| `server/api_queue.go` | `queueStore`, `onSession`, `onHand`, `onProject`, `queueRefusal`, `finish`, `answerQueue`, `raisedHand`, `handIn`, `now`, `notBuilt`, the queue's JSON (`queueJSON`, `describeQueue`), `GET queue` |
| `server/notify.go` | `chatService`, `tell`, `queuePing`, `sessionStarted`, `sessionEnded`, `handRaised`, `faqAdded`, `botSays` and their payloads |
| `server/shapes.go` | `sessionStore`, `faqStore`, `statsStore`, `faqJSON`, `describeFAQ(s)`, `statsJSON` |
| `server/api_queue_test.go` | the shared test helpers: `newQueueApp`, `fakeQueue`, `fakeFAQ`, `fakeChat`, `fakeRoles`, `person`, the ids |
| `web/src/queue/types.ts`, `web/src/queue/api.ts` | the queue's types and every call |
| `web/src/queue/store.ts` | **exports only** are frozen (`watchQueue`, `useQueue`, `getQueueState`, `subscribeQueue`, `refreshQueue`, `acceptQueue`, `findHand`, `watchedRoom`, `QueueState`, `WatchedRoom`); the body is worker 5's |
| `web/src/queue/fixtures.ts` | a `Queue` for every state the room can show, for tests and for looking at a part without a session |
| `web/src/faq.ts`, `web/src/stats.ts`, `web/src/issueLink.ts` | FAQ and stats types and calls, `faqPath`, `issueUrl` |
| in `web/src/nexconn/cards.ts` and `cardElements.ts` (worker 9's files) | the names other files import stay as they are: `BOT_ID`, `isFromBot`, `CardMessage`, and the classes `OhSessionCard`, `OhHandCard`, `OhFaqCard` |
| `web/src/pages/Room.tsx`, `web/src/App.tsx`, `web/src/router.ts`, `web/src/main.tsx`, `web/src/styles/app.css`, `web/src/styles/tokens.css` | how the room's parts are put together, the routes, the order of the style files |
| the `Props` type of `RoomHead`, `QueuePanel`, `YourTurnBanner`, the four sheets and `Stats` | what the room page passes to each part |
| `docs/`, `notes/`, `CLAUDE.md`, `AGENTS.md`, `vercel.json`, `.github/`, `web/package.json`, `server/go.mod` | the lead writes these after integration; no new dependency without the owner |

**Routes** (all registered; a stub answers 501 until its worker lands):
```
GET  /api/p/{owner}/{repo}/queue               anyone; hands for members only            lead
POST /api/p/{owner}/{repo}/sessions            maintainer  {title, plannedMinutes}       W2
POST /api/p/{owner}/{repo}/next-session        maintainer  {at}                          W2
POST /api/sessions/{id}/end                    maintainer  {carry}                       W2
POST /api/sessions/{id}/lastcall               maintainer  {closed}                      W2
POST /api/sessions/{id}/hands                  member      {title, details}              W3
POST /api/hands/{id}/withdraw | plus | here    member (the rules check whose hand it is) W3
POST /api/sessions/{id}/next                   maintainer                                W4
POST /api/hands/{id}/skip | remove | away | merge   maintainer                           W4
POST /api/hands/{id}/done                      maintainer  {answer, links}               W4
POST /api/p/{owner}/{repo}/mute                maintainer  {userId, minutes}             W4
POST /api/p/{owner}/{repo}/faq/similar         signed in   {text}                        W8
GET  /api/p/{owner}/{repo}/faq/latest          anyone                                    W8
POST /api/faq/{id}/delete                      maintainer                                W8
GET  /api/p/{owner}/{repo}/stats · /api/stats  anyone                                    W8
GET  /p/{owner}/{repo}/faq · /faq/{slug}       anyone, HTML from Go                      W7
```
Web routes added: `/stats` and `/p/{owner}/{repo}/stats`.

## 4. The rules of the queue (what `queue.Step` must do)
Step never changes its input: it copies the hand list, and any `Plus` list it changes. The state it returns holds the same hands as the one it got. A hand the action closed stays in the list with its new status and `ClosedAt = a.At`; a raised hand is added with `ID 0`. The order of the list means nothing: the queue's order is `SortAt`, then `ID`, and a hand with `ID 0` goes last among equals. Times are whole microseconds. `Plus` is a set: the database keeps no order for it, so Step keeps it sorted by user id, and that is how the store loads it.

A session that is not `Live` refuses every action with `ErrNotLive`. "Open" means `Waiting` or `Answering`; an action on a hand id that is not in the state is `ErrNoSuchHand`. Whether the actor is a member or a maintainer is the guard's question, not Step's.

| Kind | Refused when (in this order) | Effect |
|---|---|---|
| `Raise` | `HandsClosed` → `ErrHandsClosed` · the actor has an open hand → `ErrAlreadyOpen` · `LastRaisedAt` is set and less than `RaiseEvery` before `At` → `*TooSoonError` with the time left | a new hand: `ID 0`, the actor as author, `Title`, `Details`, `Waiting`, `RaisedAt = SortAt = At` |
| `Withdraw` | not the author → `ErrNotYours` · not `Waiting` → `ErrBadStatus` | `Withdrawn` |
| `Plus` | the actor is the author → `ErrOwnHand` · not `Waiting` → `ErrBadStatus` | the actor's id is added to `Plus` (kept sorted), or taken out if it is there. `SortAt` never changes |
| `Here` | not the author → `ErrNotYours` · not `Answering` → `ErrNotAnswering` | `AckedAt = At` if it was zero (a second press keeps the first time) |
| `Next` | a hand is `Answering` → `ErrAnswering` · nobody waits → `ErrNothingWaiting` | the first waiting hand becomes `Answering`: `TakenAt = At`, `TakenBy` = the actor, `AckedAt` cleared |
| `Skip` | — | `Skipped` (from `Waiting` or `Answering`) |
| `Remove` | — | `Removed` (from `Waiting` or `Answering`) |
| `Away` | not `Answering` → `ErrNotAnswering` · `AckedAt` is set → `ErrIsHere` | `AwayCount + 1`. On the third miss: `Skipped`. Otherwise back to `Waiting` with `TakenAt`, `TakenBy` and `AckedAt` cleared, and a new `SortAt`: on the first miss two places behind (with three or more others waiting, halfway between the second and the third of them; with fewer, the end), on the second miss the end. If no whole microsecond lies strictly between the second and the third, the hand goes to the end instead. "The end" is later than every other waiting hand and not earlier than `At` |
| `Merge` | not `Waiting` → `ErrBadStatus` · no hand is `Answering` → `ErrNotAnswering` | the hand becomes `Merged` with `MergedInto` = the id of the hand being answered. That hand's `Plus` gains the merged hand's author and its `Plus`, without repeats and without its own author (still a sorted set) |
| `Done` | not `Answering` → `ErrNotAnswering` | `Answered` |
| `LastCall` | — | `HandsClosed = a.Closed` |
| `End` | a hand is `Answering` → `ErrAnswering` | every `Waiting` hand becomes `Carried` (when `a.Carry`) or `NotReached`; `Live = false` |
| anything else | `ErrNotImplemented` | |

What the store adds around Step: it fills `LastRaisedAt` for a Raise (the actor's latest `raised_at` in the project, in any session), gives a new hand its id, writes the hands whose fields changed and the `hand_plus` rows that were added or taken away, marks the session ended when `Live` turned false, and raises the version by one on every action that Step allowed. Starting a session brings the project's `carried` hands into it as `waiting` hands with `closed_at`, `taken_at`, `taken_by`, `acked_at` cleared and `away_count` 0; they keep `sort_at`, so they stand first, in their old order (FR-SESS-7). A new session starts at version 1.

## 5. The ten workers
Each has one area and owns its files alone. "Reads" lists what to read before writing; everyone also reads this plan, `AGENTS.md`, `docs/WORKFLOW.md` and the SRS items named.

| # | Area | Owns | Depends on at run time |
|---|---|---|---|
| 1 | Queue rules | `server/queue/step.go`, `server/queue/*_test.go` | — |
| 2 | Sessions API | `server/api_sessions.go` + test, `server/store/sessions.go` + test | lead's `Apply` (End, LastCall) |
| 3 | Hands API | `server/api_hands.go` + test | lead's `Apply`, W1 |
| 4 | Maintainer actions | `server/api_actions.go` + test, `server/slug.go` + test (a slug is at most 72 characters; the store's "-2" keeps it within the 80 of `slugRe`) | lead's `Apply` and `Done`, W1 |
| 5 | Queue panel | `web/src/queue/store.ts` (body) + tests, new files under `web/src/queue/`, `web/src/room/QueuePanel.tsx`, `web/src/room/panel/`, `web/src/styles/queue.css` | W2–W4, W8 (delete) |
| 6 | Sheets, room head, banner | `web/src/components/Sheet.tsx`, `web/src/room/sheets/`, `web/src/room/RoomHead.tsx`, `web/src/room/YourTurnBanner.tsx`, new files under `web/src/room/head/`, `web/src/styles/room.css` | W2–W4, W8 |
| 7 | Public FAQ pages | `server/pages/`, `server/faq_pages.go` + test, `server/store/faq.go` + test | W4 (entries exist) |
| 8 | FAQ and stats API | `server/api_faq.go` + test, `server/api_stats.go` + test, `server/store/stats.go` + test | W7's store functions |
| 9 | Cards, join page, stats pages | `web/src/nexconn/cards.ts` + test, `web/src/nexconn/cardElements.ts`, `web/src/room/JoinGate.tsx`, `web/src/pages/Stats.tsx`, `web/src/styles/gate.css`, `web/src/styles/stats.css` | W5's store, W8 |
| 10 | Landing | `web/src/pages/Landing.tsx`, `web/src/landing/`, `web/src/styles/landing.css`, the landing photos in `web/public/` | — |
| lead | Store core, integration | `server/store/queue.go` + test, everything frozen | W1 |

**What each must cover** (the tests name the SRS id they prove):
1. **Queue rules.** Every row of §4 as a table test: each allowed transition, each refusal in its stated order, first in first out, one open hand, one answering hand, Away on the first, second and third miss with 0, 1, 2, 3 and 5 others waiting, Merge with overlapping +1s, End with and without carry, the input never changed (compare a deep copy), a property-style loop of random actions that never yields two answering hands or two open hands of one author.
2. **Sessions.** Start (validation, the GitHub re-check with its three outcomes, the live-session conflict, the card with @all, carried hands first), end (the card's counts), last call (closed, reopened, the room is told how many are left), next session time (future only, clear). Store tests on real Postgres: two starts at the same moment, carry-over order, `RemoveMaintainer`.
3. **Hands.** Raise (lengths counted in characters after trimming, the card, the wait sentence from `TooSoonError`), withdraw, +1, "I'm here"; each with the refusal sentences and "nothing is told to the room when refused".
4. **Maintainer actions.** Next, skip, remove, away (the three sentences the room is told), merge, done (answer and link validation, the draft, the slug with unicode, long and empty titles, the FAQ card with every asker's login), mute (who may be muted, the minutes), and AT-10: a member calling any of them gets 403 and nothing is applied.
5. **Queue panel.** The store (load, focus, reconnect, a newer ping of this room, an older or forged ping or one from another room ignored, an answer older than what is held dropped, one fetch at a time, the latency log of NFR-PERF-1 on one clock), the panel of `docs/UI.md` S3 in every state (loading, failed with retry, no session, live for a member with and without a hand, live for a maintainer, last call, ended with its summary), "N ahead of you" and "You're next" (never counting the hand being answered), +1, withdraw, the maintainer's Next / Done / Skip / Away / Remove / Merge / Mute by click, and the four controls that otherwise have a route and no button: **"Open a GitHub issue" on the hand being answered (FR-FAQ-5), the next session's time set and cleared by a maintainer (FR-SESS-5), "Remove from the FAQ" on an answered hand of the done list for a maintainer (FR-FAQ-6, AT-8), and "here / waiting for a reply" from `ackedAt` (FR-QUEUE-9)**; `aria-live` for queue changes. Not this worker's: the second slice named in §1.
6. **Sheets, head, banner.** One accessible `Sheet` (native `<dialog>`, focus kept inside, Escape closes), the four sheets with their limits, counters and the server's refusal shown inline, similar answers while typing (300 ms debounce, stale answers dropped), the head (live label, the session as the headline, the meta line, Last call, End, Start, FAQ link, the phone tabs with the queue first), the your-turn banner with "I'm here" and an assertive `aria-live`.
7. **FAQ pages.** List with search, entry, 404, 410 for a deleted entry, user text always escaped, links only `http`/`https` with `rel="noopener noreferrer nofollow"`, link-preview tags, "asked by N people", the prefilled issue link, light and dark from the system setting, no JavaScript needed to read, 375 px wide without sideways scroll. Store tests on real Postgres: ranking on fixtures, deleted entries left out of lists and found by slug.
8. **FAQ and stats API.** Similar (limit 3, short text asks nothing), latest (5), delete (only a maintainer of that entry's project, twice is fine), the two stats routes; `Stats` on real Postgres with a fixture where every number differs.
9. **Cards, join page, stats.** The hand card with the hand's live status from the queue store (and no title while the queue says it was removed), the FAQ card with a link built from the slug on our own origin, the names of who asked, the issue link, and for a maintainer "Remove from the FAQ" (so an entry of an earlier session can be deleted too), the session card with the end's counts; a card not sent by the bot shows nothing of its content (FR-QUEUE-7, kept); the join page's live session, next session in the viewer's time zone and latest answers; the stats pages with loading, empty and error states.
10. **Landing.** The approved story of `docs/UI.md` §1 v3 and S1 (hero, the session hour by hour with the pinned photo and the dot clock, the three numbers, the questions, the closing), its dark mode, reduced motion, phones, and no decorative labels or mock cards (`CLAUDE.md`, mistakes and rules).

## 6. Rules for every worker
- Work only in your own worktree and on your own branch `slice/p2-<area>`. Edit only the files you own; create new files only inside your area. Never edit a frozen file (§3).
- **Names carry your area.** Five workers write into Go's `package main` and three into `package store` at the same time, and each pull request is green alone: two helpers with the same name break `main` at the second merge. So every new package-level name in Go (functions, types, variables, constants, test helpers and fakes) starts with your area's word (`sessionTitle…`, `handsFake…`, `actionsApp…`, `faqPage…`, `statsFake…`), and you use the shared helpers of §3 instead of writing a second one. In CSS every new class starts with your area's prefix (`q-` panel, `sheet-`, `rh-` and `turn-` for worker 6, `gate-` and `stats-` for worker 9, `land-` for the landing).
- Read first: this plan, `AGENTS.md`, `docs/WORKFLOW.md`, the code your change touches. Don't guess an SDK call: read the installed package's types or the docs, and say in your pull request what you confirmed.
- Tests first for logic: a `test:` commit that fails, then the code. Go tests are table tests named after SRS ids; web tests are Vitest for logic only (no DOM).
- Before the pull request, everything green: `gofmt -l server` (prints nothing), `go -C server vet ./...`, `TEST_DATABASE_URL='postgres://localhost:5432/officehours_test?sslmode=disable' go -C server test ./...`, and for web work `npm --prefix web ci` once, then `npm --prefix web run lint`, `typecheck`, `test`, `build`.
- Never read or print `.env`. Don't start servers on ports 8080 or 5173 and don't use the browser: the lead runs the app. No new dependency, no download.
- Commits: `<type>: <description>` in lower case (feat, fix, refactor, docs, test, chore), SRS ids in the body, explicit paths in `git add`, the co-author trailer. Never `--no-verify`, never a force push, never a push to `main`.
- Finish with `git push -u origin <branch>` and `gh pr create` against `main`. The body says what was built, what was verified with the real output of the checks, what was decided, and what is open. Don't merge. Your last message repeats that summary and the pull request's number.
- Stuck on the same error three times: stop and say what you tried.

## 7. Integration (lead)
1. While the ten run: build `store.Queue`, `Apply`, `Done`, `ProjectOfSession`, `ProjectOfHand`, `People` with tests on real Postgres, against a small test double of Step until W1 lands.
2. Merge order: W1 (rules) → lead's store core → W2, W3, W4 (routes) → W7 → W8 → W5, W6, W9 → W10 (independent, any time). Each: read the diff, CI green, `gh pr merge --rebase --delete-branch`, then pull and run every check on `main`.
3. After the routes are in: the two-user run on this machine (a second origin, `design/.work/two-user/README.md`): AT-4, 6, 7, 9, 10, 11, 15, 17, 18, then AT-5, 8, 19, then AT-12..14, with the ping latency of AT-16 read from the console log.
4. Fix the seams in small `fix:` slices. Then the phase-close reviews for P2 and P3 (code review, security review, ponytail audit) on a tree nobody edits, `notes/phase-2.md` and `notes/phase-3.md` with real output, and the docs (`ARCHITECTURE.md` §3 to §6, `UI.md`, `PHASES.md`).

## 8. Risks and open points
| Risk | What we do |
|---|---|
| A worker needs something the contract lacks | They stop and report; the lead changes the contract once and tells the others at integration |
| `Step` and the store disagree about a detail | §4 is the single source; the store's tests run the real Step as soon as W1 is merged |
| Two workers style the same element | One style file per worker, loaded after `app.css` in a fixed order; rules in `app.css` that became dead are listed in the pull request and removed by the lead |
| Ten sessions spend a lot of tokens for work that then needs rework | Small, closed areas with tests; a worker that drifts is stopped rather than fixed in place |
| A removed hand's card stays in the chat | The queue no longer shows the words, and the card hides its title while the queue says "removed". The message itself stays, and once the next session starts the card can no longer learn that it was removed. Try "Delete for all" as a maintainer in the two-user run; if only the sender can, a later slice stores the card's message id with the hand and lets the bot recall it |
| `main` is edited by the schema while hosting is set up | New columns are a new file (`002_queue.sql`, idempotent), never an edit of `001` |
| Cold `GET /queue` on Vercel is slow (ARCHITECTURE §1) | Measured on the first deployment; the fallback (snapshot inside the ping) is already described there |
| The production deployment still serves only `web/` | Owner action (hosting walkthrough, step 2); the gates wait for it, the build does not |

## 9. Status
| Step | State |
|---|---|
| Skeleton and this plan | 🟨 reviewed by `plan-reviewer` (13 findings: 7 to fix first, all applied; see below), merging |
| Ten workers | ⬜ |
| Store core | ⬜ |
| Integration and the two-user run | ⬜ |

## 10. The plan's review (2026-11-12, `plan-reviewer`, before any worker started)
Verdict: fix seven things first. What it found and what changed:
1. **Go names would collide** between workers who share a package, with every pull request green alone. → shared helpers in the skeleton (`cleanLine`, `cleanText`, `lengthBetween`, `personCols`, `sessionSelect`, `scanSession`, `fakeFAQ`) and the naming rule in §6.
2. **Four requirements had a route and no control** (delete a FAQ entry, the issue link on a hand, the next session's time, "here"). → named in worker 5's list; `faqId` added beside `faqSlug` so the delete has its id; the FAQ card also offers the delete to a maintainer.
3. **The queue panel's worker carried too much** for a screen built without a browser. → keys, timer, tab title, wait estimate, light and the client-side "here" moved to a second slice (§1); a fixtures file of queue states so each part can be looked at on arrival.
4. **The FAQ pages' worker could not go green** inside its own files: two rows of `main_test.go` pinned the placeholder. → rows deleted.
5. **Three routes needed facts the contract did not hand over.** → the stubs' comments now say where each comes from: Done reads the hand from `Queue(...).Open`, End counts from `Queue` after `Apply` (so `Closed` is never limited), mute asks `ProjectByRepo` as the target (`IsMember` left `queueStore`).
6. **A long duplicate question would get a dead link** (80 characters plus "-2" fails `slugRe`). → slugs are at most 72; the store keeps the total within 80.
7. **The first migration was edited in place** while hosting is being set up. → `001` restored, `002_queue.sql` added.
8. Smaller ones, applied: `Plus` is a set (the table has no order); Away falls back to the end when no microsecond lies between two hands; a removed hand loses its words in the queue; the GitHub re-check removes a maintainer only on a clear "no"; pings count only in their own room and an older answer never replaces a newer one; suggestions match any word; the latency of NFR-PERF-1 is measured on one clock.
Not taken: reserving a column and a return value for recalling a removed hand's card now. It can be added later in one small slice without changing what the workers build against (§8).
