# PRD — Office Hours

**Status:** v1.2, 2026-11-12 (owner decisions: Go server, queue extras, carry-over by default) · **Owner:** Vishal Kumar · **Deadline:** Nexconn Build Challenge, Nov 22 23:59 AoE (Nov 23 17:29 IST)
**Related:** `SRS.md` (requirements with IDs), `ARCHITECTURE.md` (design), `PHASES.md` (build plan), `WORKFLOW.md` (how agents build it)

## 1. Summary
Office Hours gives open-source maintainers a live room where contributors raise a hand with a question, the maintainer answers them one at a time, and every answer becomes a public FAQ entry with a permanent link, so the same question is never answered twice. Built on the Nexconn Chat SDK and Chat UI.

## 2. Problem
- About 60% of open-source maintainers are unpaid, and nearly 60% have quit or considered quitting a project (Tidelift 2024 maintainer survey).
- Contributor questions arrive as "how do I…" issues, duplicate questions and DMs. Each one is answered in private or buried in a thread, so the next contributor asks again.
- Real-time help today means Discord or Slack servers: noisy, unordered, nobody knows who is next, and the answers can't be found later.
- Many contributors don't write English comfortably, so they don't ask at all.
- Queue tools exist for university office hours (Illinois Queue, Khoury Office Hours, HelpMe), but not for open-source projects, and none turn answers into a public FAQ.

## 3. Users
| Persona | Who | Needs |
|---|---|---|
| **Maintainer** (primary) | Maintains a library with ~1–10k stars and dozens of open "question" issues; volunteers evenings | Answer each question once, in a bounded time slot, without a 24/7 chat server |
| **Contributor** | First or second PR, may write English poorly, works in another time zone | Ask without being judged, know when it's their turn, get an answer they can link to |
| **Visitor** | Finds a FAQ entry from search or an issue link | Read the answer without signing in |

## 4. Goals and success metrics
| Goal | Metric | Target by Nov 21 |
|---|---|---|
| G1 Maintainers answer each question once | FAQ entries created per session | ≥ 3 per session |
| G2 Contributors get answers live | Questions answered / raised in a session | ≥ 70% |
| G3 Real use (challenge criterion, 20% + bonus) | Active users on the Nexconn app | ≥ 50 users, ≥ 3 maintainers, ≥ 5 sessions |
| G4 Works end to end (challenge criterion, 50%) | AT-1…AT-19 pass on the production URL | 100% |

## 5. User stories
- **US-1** As a maintainer, I sign in with GitHub and register a repo I maintain, so that it gets an Office Hours room.
- **US-2** As a maintainer, I start a session ("Office hours, Thu 17:00 UTC, 45 min") and members are told it's live.
- **US-3** As a contributor, I open the project's link, sign in with GitHub and join the room.
- **US-4** As a contributor, before I raise my hand I see FAQ answers to similar questions.
- **US-5** As a contributor, I raise a hand with my question and see my place in the queue update live.
- **US-6** As a maintainer, I click Next to take the next question, chat about it in the room, then click Done with a short answer summary.
- **US-7** As anyone, I open the permanent FAQ link of an answer without signing in.
- **US-8** As a contributor who wrote in Spanish, the maintainer reads my question in English, and I read the answer in Spanish (stretch).
- **US-9** As a maintainer, I turn a question into a prefilled GitHub issue when it's a real bug.
- **US-10** As a maintainer, I end the session, and unanswered hands move to the front of the next session (or are marked "not reached" if I choose), so their authors don't have to ask again.
- **US-11** As the team, we see per-project numbers (sessions, questions, answers, participants) for traction proof.

## 6. Scope
**MVP (phases P0–P4):** GitHub sign-in · project registration · project room (Nexconn group channel via Chat UI) · sessions · raise hand · live queue · Next/Skip/Done/Remove · FAQ entries with permalinks and a public FAQ page · FAQ suggestions before raising a hand · prefilled GitHub issue link · stats page.
**Stretch (only if P4 finishes early, in this order):** built-in translation · "your turn" browser notification · next-session time · account deletion · co-maintainers · FAQ editing.
**Later (not in the challenge):** voice/screen-share calls (Call SDK) · GitHub App that links FAQ entries on new issues · Discord/Slack bridge · recurring schedules and calendar invites · analytics.
**Non-goals:** replacing GitHub Discussions or Discord for general chat · private support for paying customers · mobile apps (the web app is mobile-friendly).

## 7. Alternatives users have today
| Tool | Why it isn't enough |
|---|---|
| GitHub Discussions / issues | Async only; duplicates pile up; no live session |
| Discord or Slack server | No queue or order; answers are hard to find later; one more server to watch |
| Slido / Mentimeter | Built for talks, not for helping one person at a time; no FAQ |
| University queue tools | Built for courses (TAs, enrolment), not for open-source projects; no public FAQ |

## 8. Risks
| Risk | Mitigation |
|---|---|
| No maintainers hold sessions (chicken and egg) | Outreach starts Nov 12 as its own human track; sessions booked for Nov 16–21 |
| Completeness (50% of score) suffers from scope | Feature freeze Nov 19 10:00 IST; stretch items only after the AT list passes |
| The default Nexconn Free plan lasts one month (the challenge promises one year) | P0 checks the plan in the Console; ask Nexconn early if it shows the default plan |
| Nexconn SDK surprises | P0 spike verifies every assumption in `ARCHITECTURE.md` §8 before building |
| Abuse in public rooms | GitHub sign-in, rate limits, maintainer remove/mute, Nexconn moderation |

## 9. Release plan
P0 spike + skeleton → P1 projects and rooms → P2 live queue (first real session) → P3 FAQ → P4 polish + stretch → freeze → P5 traction and hardening → P6 submission. Dates in `PHASES.md`.
