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

**Open.** Flows through the real `queue.Step` are tested when the queue-rules slice is merged.
