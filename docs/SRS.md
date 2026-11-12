# SRS — Office Hours

**Version:** 1.2 (2026-11-12, owner decisions: Go server + TypeScript UI; queue extras FR-SESS-6..7, FR-HAND-7..8, FR-QUEUE-8..12; ending carries waiting hands over by default; staff-review fixes to FR-QUEUE-1, 8, 9, FR-HAND-8 and AT-6/8/16/18/19). v1.0 → v1.1: GitHub scope and token storage defined; queue ping is version-only; OH:* sender check; Take, FAQ edit, views counter and the events table cut; browser support matches Chat UI; announcement defined. Source: `PRD.md`. IDs are stable: specs, tests and commits cite them. **Must** = MVP, **Should** = stretch.

## 1. Definitions
- **Project:** one GitHub repository registered in Office Hours. Has one **room** (a Nexconn group channel).
- **Maintainer:** a user in the project's `maintainers` table. Added when they register the repo, after GitHub reports `admin` or `maintain` permission.
- **Member:** a signed-in user who joined the project's room.
- **Session:** a time-boxed live period in a room; at most one live session per project.
- **Hand:** one question raised by a member in a session. **Queue:** a session's hands in order, with status.
- **FAQ entry:** a public answer created when a maintainer marks a hand Done.

## 2. Roles and permissions
| Action | Visitor | Member | Maintainer |
|---|---|---|---|
| Read public FAQ pages | ✓ | ✓ | ✓ |
| Join a room, chat, raise/withdraw own hand | – | ✓ | ✓ |
| Register a project | – | ✓ (repos where GitHub says admin/maintain) | ✓ |
| Start/end session, Next/Skip/Done/Remove, delete FAQ, mute | – | – | ✓ |

## 3. Functional requirements

### Authentication (AUTH)
- **FR-AUTH-1 (Must)** Sign in with GitHub OAuth, scope `read:user` only. The database stores GitHub id, login, display name and avatar URL. The user's GitHub access token is kept only inside the encrypted session cookie, used for the repo permission check, and never stored in the database or exposed to client JavaScript.
- **FR-AUTH-2 (Must)** The server issues a Nexconn access token for user id `gh<githubId>`. The App Secret never reaches the client.
- **FR-AUTH-3 (Must)** Every signed-in page connects to Nexconn and reconnects automatically. On an invalid token it fetches a new one once and retries.
- **FR-AUTH-4 (Must)** Sign out disconnects from Nexconn and clears the session.
- **FR-AUTH-5 (Should)** A user can delete their account: their hands are anonymized, their user row is deleted, and their Nexconn tokens are expired.

### Projects and rooms (PROJ)
- **FR-PROJ-1 (Must)** A signed-in user can register `owner/repo` only if `GET /repos/{owner}/{repo}` with their token returns `permissions.admin` or `permissions.maintain` true. If GitHub refuses because an org restricts OAuth apps, show "Ask an owner of {org} to approve Office Hours".
- **FR-PROJ-2 (Must)** Registering creates the project, makes the registrant a maintainer and creates the room (group channel with the bot and the registrant). Registering an existing repo returns the existing project.
- **FR-PROJ-3 (Must)** Join page `/p/{owner}/{repo}`: repo name, description, the live session if any, the next session time if set, and the latest FAQ entries.
- **FR-PROJ-4 (Must)** Join adds the user to `members` and to the group. The room shows Chat UI (text, images, files, replies, @mentions, read receipts) with profiles served by the app.
- **FR-PROJ-5 (Should)** A maintainer can add co-maintainers who pass the FR-PROJ-1 check.

### Sessions (SESS)
- **FR-SESS-1 (Must)** A maintainer starts a session with a title (5–80 chars) and an optional planned length (15–180 min).
- **FR-SESS-2 (Must)** Starting fails if the project already has a live session (database-enforced).
- **FR-SESS-3 (Must)** Starting posts a session card mentioning @all, so members get an unread badge (and a push where supported).
- **FR-SESS-4 (Must)** Ending moves all `waiting` hands to `carried` (default, FR-SESS-7) or `not_reached` (if the maintainer unticks carry-over) and posts a session-ended card.
- **FR-SESS-5 (Should)** A maintainer sets the next session time, shown on the join page in the viewer's time zone.
- **FR-SESS-6 (Must)** Last call: a maintainer closes new hands while the session stays live (and can reopen them). The room sees "Hands closed, N left"; raising is refused while closed.
- **FR-SESS-7 (Must)** Carry-over: hands moved to `carried` at the end become `waiting` at the front of the project's next session, in their old order. Their authors see it, and the ended card counts them.

### Hands (HAND)
- **FR-HAND-1 (Must)** During a live session a member raises a hand: title 10–120 chars, optional details ≤ 2,000 chars (shown as plain text; links clickable).
- **FR-HAND-2 (Must)** At most one open hand (`waiting` or `answering`) per user per session (database-enforced).
- **FR-HAND-3 (Must)** Before submitting, up to 3 similar FAQ entries of the project are shown (FR-FAQ-4). The user can open one and cancel, or continue.
- **FR-HAND-4 (Must)** The author can withdraw a `waiting` hand.
- **FR-HAND-5 (Must)** A raised hand appears in the room as a hand card with title and author. The card shows the hand's current status, read from the live queue.
- **FR-HAND-6 (Must)** At most one hand per user per 60 s per project.
- **FR-HAND-7 (Must)** "+1 me too": a member can +1 another member's `waiting` hand once (toggle). The count is shown; the order stays first in, first out and never sorts by votes.
- **FR-HAND-8 (Must)** Merge: a maintainer can merge a `waiting` hand into the hand being answered (`merged`). Its author and +1s join that hand (the +1s are copied over). In the room the FAQ card names everyone who asked; the public FAQ page shows only a count ("asked by 3 people").

### Queue (QUEUE)
- **FR-QUEUE-1 (Must)** The server is the only writer of queue state. Order: first in, first out by `sort_at` (the raise time, except after Away or carry-over).
- **FR-QUEUE-2 (Must)** Allowed transitions only: `waiting → answering → answered`; `waiting → withdrawn`; `waiting|answering → skipped`; `waiting|answering → removed`; `answering → waiting` (Away, FR-QUEUE-8); `waiting → merged`; `waiting → not_reached | carried` (on end); `carried → waiting` (next session start).
- **FR-QUEUE-3 (Must)** Maintainer actions: **Next** (oldest `waiting` → `answering`; if one is already `answering`, it must be closed first), **Skip**, **Done** (needs FR-FAQ-1 input), **Remove**. At most one `answering` hand per session (database-enforced).
- **FR-QUEUE-4 (Must)** Every room member sees the live queue (position, title, author, status) next to the chat. After each change the server sends a version ping through Nexconn, and clients with an older version fetch the queue. No page reload.
- **FR-QUEUE-5 (Must)** A member with a hand sees "N ahead of you" (never counting the hand being answered) or "You're next", and a clear "It's your turn" state when their hand becomes `answering`.
- **FR-QUEUE-6 (Must)** Clients also fetch the queue on load, on reconnect and when the tab regains focus, so a missed ping is harmless.
- **FR-QUEUE-7 (Must)** Clients act only on OH:\* messages sent by the `oh_bot` user and ignore any others.
- **FR-QUEUE-8 (Must)** Away: while the current hand's author hasn't pressed "I'm here", the maintainer can mark them Away: the hand goes back to `waiting` two places behind (by `sort_at`), to the end on a second miss, and to `skipped` on a third; the room is told why.
- **FR-QUEUE-9 (Must)** "I'm here": the your-turn banner has an "I'm here" button, stored on the hand (`acked_at`); the maintainer sees "here" or "waiting for a reply". The maintainer's client also shows "here" when the author posted in the room since the hand was taken (client-side only: the server can't see chat messages).
- **FR-QUEUE-10 (Should)** Wait estimate: after 2 answers in the session, a member with a hand sees "about N min" from the session's median answer time.
- **FR-QUEUE-11 (Should)** The tab title carries the place: "(2 ahead) Office Hours", "(Your turn) Office Hours".
- **FR-QUEUE-12 (Should)** The maintainer sees a timer on the current question (only they see it); it turns amber after 8 min.

### FAQ (FAQ)
- **FR-FAQ-1 (Must)** Done requires an answer summary (20–2,000 chars) and optional links. It creates a FAQ entry linked to the hand, with a slug from the title (unique per project).
- **FR-FAQ-2 (Must)** The FAQ entry is posted to the room as a FAQ card. The card's link is built from the slug on the app's own origin.
- **FR-FAQ-3 (Must)** Public server-rendered pages, readable signed-out: `/p/{owner}/{repo}/faq` (list + search) and `/p/{owner}/{repo}/faq/{slug}` (question, answer, links, date, maintainer), with link-preview metadata.
- **FR-FAQ-4 (Must)** Similar-question search: Postgres full-text match of the draft title + details against the project's FAQ entries; up to 3 results.
- **FR-FAQ-5 (Must)** "Open GitHub issue": a prefilled `https://github.com/{owner}/{repo}/issues/new?title=…&body=…` link on hands and FAQ entries. The app writes nothing to GitHub.
- **FR-FAQ-6 (Must)** A maintainer can delete a FAQ entry; its URL then returns 410 "removed". (Editing is Should.)

### Translation, notifications, moderation, metrics
- **FR-I18N-1 (Should)** A user sets a preferred language; questions and answers can be shown translated, with the original one tap away (Nexconn built-in translation).
- **FR-NOTIF-1 (Must)** Session start is announced as in FR-SESS-3.
- **FR-NOTIF-2 (Should)** "It's your turn" triggers a browser notification if the user allowed it.
- **FR-MOD-1 (Must)** A maintainer can remove a hand and mute a user in the room.
- **FR-MOD-2 (Must)** Nexconn's profanity filter is on for the app.
- **FR-MET-1 (Must)** `/p/{owner}/{repo}/stats` and `/stats` show sessions held, hands raised, hands answered, FAQ entries and unique participants (members), computed from the tables above.

## 4. Non-functional requirements
- **NFR-PERF-1** A queue change reaches other online members' panels in ≤ 1 s p95. Measured with `sentAt` in the ping: the client logs `now - sentAt` plus the fetch time; p95 over 20 Next clicks ≤ 1,000 ms.
- **NFR-PERF-2** Join and FAQ pages: LCP ≤ 2.5 s on a mid-range phone on 4G (Lighthouse mobile).
- **NFR-SEC-1** Secrets (Nexconn App Secret, GitHub OAuth secret, `AUTH_SECRET`, `DATABASE_URL`) only in server env and a git-ignored `.env`. Never logged, never in the client bundle.
- **NFR-SEC-2** Every mutation checks auth and role on the server. Roles come from the `maintainers` table.
- **NFR-SEC-3** All inputs are validated at the Go handler boundary (lengths, types, `owner/repo` pattern `^[A-Za-z0-9-]{1,39}/[A-Za-z0-9._-]{1,100}$`).
- **NFR-SEC-4** Rate limit FR-HAND-6. `ponytail:` add a general per-user limiter only if abuse appears.
- **NFR-SEC-5** The session cookie is AES-GCM encrypted, HttpOnly, Secure, SameSite=Lax; OAuth uses state + PKCE. Every mutation rejects `Origin` ≠ `APP_URL` or a non-JSON body.
- **NFR-SEC-6** User text is rendered as text (`textContent` / React escaping, never HTML). Links only from slugs on our origin or user links with `rel="noopener noreferrer nofollow"`. OH:\* messages are trusted only from `oh_bot` (FR-QUEUE-7).
- **NFR-PRIV-1** No email addresses and no private repo data stored. Account deletion as in FR-AUTH-5.
- **NFR-A11Y-1** WCAG 2.2 AA basics on our own UI: keyboard use, visible focus, 4.5:1 contrast, queue changes announced with `aria-live="polite"`, "It's your turn" with `aria-live="assertive"`.
- **NFR-COMP-1** The room supports the latest desktop Chrome, Edge and Safari (Chat UI's supported browsers); Firefox gets a "best in Chrome, Edge or Safari" note. At 375 px the join page, FAQ pages, queue panel and hand form must fully work; the chat pane is best effort on phones, and the queue tab opens first.
- **NFR-REL-1** Reloading or reconnecting never loses queue state (it lives in Postgres).
- **NFR-OPS-1** Production deploys only from GitHub `main` through CI; a failing check blocks the merge.
- **NFR-OPS-2** Server errors are logged with a request id and without secrets or tokens.

## 5. External interfaces
- **GitHub:** OAuth (`read:user`); REST `GET /repos/{owner}/{repo}` with the user's token (FR-PROJ-1). Read-only.
- **Nexconn Server API** (official Go SDK, from `server/nexconn` only): issue/expire access tokens, create group, join group, send group message (custom types, unpersisted status messages, @all mention), mute member.
- **Nexconn client:** `@nexconn/chat` (initialize with App Key + area, register custom messages, connect, listeners, translation), `@nexconn/chatui` (room UI, custom bubbles, ServiceHooks).

## 6. Acceptance tests (manual, on the production URL)
| ID | Steps | Expected | Covers |
|---|---|---|---|
| AT-1 | Sign in with GitHub | Signed in; Nexconn connected; consent screen asks only for profile; no secret or GitHub token in the network tab or JS | FR-AUTH-1..3, NFR-SEC-1 |
| AT-2 | Register a repo you maintain (user-owned and org-owned), then one you don't | Room created; the third refused with a clear message | FR-PROJ-1..2 |
| AT-3 | A second user opens `/p/owner/repo` and joins | Room visible with names and avatars; both can chat | FR-PROJ-3..4 |
| AT-4 | Maintainer starts a session, then tries to start another | Session card arrives and the second user sees an unread badge; second start refused | FR-SESS-1..3 |
| AT-5 | Member types a question similar to an existing FAQ entry | ≤ 3 suggestions shown before submit | FR-HAND-3, FR-FAQ-4 |
| AT-6 | Two members raise hands; one tries a second hand; one withdraws and raises again within 60 s | Cards appear; positions shown; second hand refused; re-raise refused with the wait time; ping latency logged | FR-HAND-1..2,4..6, FR-QUEUE-1,4 |
| AT-7 | Maintainer clicks Next | First hand → answering; its author sees "It's your turn"; its card shows "answering" | FR-QUEUE-3,5, FR-HAND-5 |
| AT-8 | Maintainer clicks Done with a summary; opens the entry's GitHub issue link; deletes the entry | FAQ card in the room; its link opens signed-out; the issue link is prefilled; after delete the URL shows 410 | FR-FAQ-1..3, FR-FAQ-5..6 |
| AT-9 | Member reloads mid-session | Same queue after reload | FR-QUEUE-6, NFR-REL-1 |
| AT-10 | A member calls the Next API directly | 403; queue unchanged | NFR-SEC-2 |
| AT-11 | Maintainer ends the session, once with carry-over and once without | Waiting hands → next session (front of the line) or not_reached; ended card counts them | FR-SESS-4, FR-SESS-7 |
| AT-12 | Open stats pages | Counts match the session just run | FR-MET-1 |
| AT-13 | Keyboard only: join → raise hand → withdraw | Possible; focus visible; queue changes announced | NFR-A11Y-1 |
| AT-14 | Phone at 375 px: join page, FAQ, queue panel, hand form | No horizontal scroll; all actions reachable | NFR-COMP-1 |
| AT-15 | A member sends a custom message typed `OH:Queue`/`OH:FAQ` from the browser console | Ignored by every other client | FR-QUEUE-7, NFR-SEC-6 |
| AT-16 | 20 queue changes of any kind (raise, withdraw, Next, Done) from two browsers while a third watches | p95 ≤ 1,000 ms in the logged deltas | NFR-PERF-1 |
| AT-17 | Next on a member who doesn't reply; Away; later Away again | Back 2 places, then to the end; the room is told; "I'm here" prevents it | FR-QUEUE-8..9 |
| AT-18 | Maintainer presses Last call; a member tries to raise a hand; the maintainer reopens hands, removes one hand and mutes a member | Refused with "Hands are closed", allowed after reopening; everyone in line still answered; the removed hand is gone; the muted member can't post | FR-SESS-6, FR-MOD-1 |
| AT-19 | Two members +1 a hand (a second +1 removes it; your own hand has no +1); the maintainer merges a duplicate into the current one and presses Done | Counts shown, order unchanged; the room card names both askers; the public page says "asked by 2 people" | FR-HAND-7..8 |

## 7. Traceability
Each phase plan (`notes/phase-N-plan.md`) lists the FR/NFR IDs it implements and the AT IDs that prove them. Test names start with the ID they cover (Go: `t.Run("FR-QUEUE-2: rejects answered → waiting", …)`; web: `it('FR-…', …)`). Commit bodies cite IDs.
