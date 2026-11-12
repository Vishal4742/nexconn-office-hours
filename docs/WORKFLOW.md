# Workflow — how every agent builds Office Hours

**v1.1 (2026-11-12):** a light loop for each small slice, the heavy reviews once per phase, and slices merged to `main` as soon as CI is green, so gates run on production.
Applies to Claude Code, Codex, opencode and every subagent. Phases and gates: `PHASES.md`.

## 0. Start of every session
1. **Claude Code:** start with `bash tools/claude.sh` from the repo root. It loads only the supermemory key from `.env` and starts this repo's memory server (`localhost:6768`, namespace `nexconn-office-hours`). The startup line should say "supermemory · N memories loaded". If it says "Authentication failed" or nothing, restart with the script.
   **Codex / opencode:** start normally. They have no supermemory, so `notes/` is their memory.
2. Read `AGENTS.md` → `docs/PHASES.md` (which phase is 🟨) → the latest `notes/phase-N-plan.md` and `notes/phase-N.md`.
3. Claude only: ask supermemory about the area you'll touch ("what did we decide about the queue ping?") before reading code.

## 1. Phase start (once per phase)
- Write `notes/phase-N-plan.md`: the SRS IDs in scope, the AT IDs that prove them, the slices (each ≤ half a day), the files each slice touches, the tests to write first, and risks.
- P0–P3: the `plan-reviewer` agent reviews the plan as a staff engineer; fix what it finds. P4–P6: skip the review.

## 2. Each slice (repeat)
| Step | What |
|---|---|
| 1 Branch | `git switch -c slice/<short-name>` from an up-to-date `main` |
| 2 Tests first | Pure logic (`server/queue`, validation, slugs, cookie) gets failing `go -C server test` table tests named after the IDs (`FR-QUEUE-2: …`) → commit `test:` (RED) |
| 3 Build | Ponytail ladder before each piece; the smallest change that passes → commit each working step (GREEN) |
| 4 Check | `/smoke` (lint, types, unit, build). UI changes: look at them in a browser at 375 and 1440 px |
| 5 Merge | PR to `main`; CI green → merge. Vercel deploys `main` |
| 6 Prove | Run the slice's AT steps on production; paste the result into `notes/phase-N.md` |
Something goes sideways → stop and re-plan. Same error 3 times → stop, write it in notes, ask the human.

## 3. Phase close (once per phase)
1. `/code-review` from a fresh context on the phase's diff (`git diff <phase-start>..main`).
2. ECC `security-review` if the phase touched auth, API routes, tokens or user input (P1, P2, P3).
3. `/ponytail-review` on the diff; `verify-app` agent runs every check and reports PASS/FAIL with real output.
4. Fix the findings in slices (§2). Run the phase gate from `PHASES.md` on production.
5. `/phase-close`: writes `notes/phase-N.md` (built, verified with output, decisions, open issues, what's next), prunes `CLAUDE.md`, and marks the phase ✅ in `PHASES.md`.

## 4. Git and deploy
- Trunk-based: short `slice/*` branches, merged to `main` as soon as CI is green. `main` is protected by **required status checks only** (lint, typecheck, unit, build, secret scan); a solo builder has no second reviewer.
- Production deploys only from `main` through Vercel's GitHub integration. Never `vercel deploy` from a laptop.
- Schema changes: run `go -C server run ./cmd/migrate` (with `MIGRATE_DATABASE_URL`) against production **before** merging the code that needs it, and write each migration so the old code still works with the new schema.
- Small commits, explicit paths (`git add <paths>`; never `git add -A` or `commit -a`), `<type>: <description>` with SRS IDs in the body. Never commit `.env`; never `--no-verify`; never force-push `main`.
- Parallel slices run in git worktrees under `.claude/worktrees/<name>`, one agent each, and never touch the same file.

## 5. Memory (supermemory + notes)
- **Supermemory** (Claude sessions started with `tools/claude.sh`) captures each turn and recalls it later. Session text goes to the extraction model, so **never print a key, token or `.env` value**.
- **`notes/` is the durable, shared record** for every agent: what was built, what was verified (real output), decisions, open issues.
- Design changes go into `docs/ARCHITECTURE.md`; requirement changes into `docs/SRS.md` with a new version line.
- After every correction from the human: one line in `CLAUDE.md` → "Mistakes and rules".

## 6. Definition of done (per slice)
- [ ] Its SRS IDs are implemented, and their tests pass
- [ ] `gofmt -l server` is empty; `go -C server vet ./...` and `go -C server test ./...` pass; in `web/`: `npm run lint`, `npx tsc --noEmit`, `npm test`, `npm run build` pass (output in notes)
- [ ] Every mutation goes through `server/guard` (Origin + JSON + session + role) and validates its input
- [ ] OH:\* messages are trusted only from `oh_bot`; user text is rendered as text
- [ ] No secret in code, logs, client bundle or chat (`git grep -nE 'sk-|ghp_|NEXCONN_APP_SECRET='`)
- [ ] Merged, deployed, and its AT steps pass on production

## 7. What only the human does
Creating accounts and apps (Nexconn, two GitHub OAuth apps, Vercel, Neon), putting secrets into `.env` and Vercel, branch protection, switching the challenge track, maintainer outreach and booking sessions, asking Nexconn about the plan, approving any new download (for example `ecc-agentshield`), recording the voice-over, and submitting the form.
