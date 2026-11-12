# CLAUDE.md — Office Hours

@AGENTS.md

## What this is
Office Hours: live rooms where open-source maintainers take contributor questions one at a time (raise hand → live queue → Next/Done), and every answer becomes a public FAQ entry with a permanent link. Go server (`server/`) + TypeScript UI (`web/`, Vite + React) + Nexconn Chat SDK/Chat UI + Neon Postgres, on Vercel (Services). Built solo for the Nexconn Build Challenge: freeze Nov 19 10:00 IST, submit Nov 22 by 20:00 IST.

- Product / requirements / design / plan: `docs/PRD.md`, `docs/SRS.md`, `docs/ARCHITECTURE.md`, `docs/PHASES.md`
- How to work: `docs/WORKFLOW.md` · Build log: `notes/` (one file per phase, updated after every phase commit)

## Commands
- Session: `bash tools/claude.sh` (loads only the supermemory key; memory server on `localhost:6768`, namespace `nexconn-office-hours`)
- Environment: Go 1.25+ and Node 20+; `go -C server mod download` · `npm --prefix web ci` (`server/` is the Go module root)
- Run: `bash tools/dev.sh` (loads `.env`, Go on :8080) + `npm --prefix web run dev` (:5173)
- Tests: `go -C server test ./...` · `npm --prefix web test` (Vitest, UI logic only) · acceptance: AT-1…AT-19 by hand on production · lint/format: `gofmt -l server` + `go -C server vet ./...` · `npm --prefix web run lint` · types: `npx --prefix web tsc --noEmit` · DB: `go -C server run ./cmd/migrate`
- Smoke: `/smoke` (detects the stack and runs everything above)

## Conventions
- Go: `gofmt`, small packages, errors wrapped with context, no globals except the pgx pool and SDK client. Web: TypeScript strict, Prettier defaults, small modules, `readonly` types, no mutation of shared state.
- Secrets only in `.env` (gitignored); `.env.example` lists every key; never echo secrets; redact them from error text.
- Every Nexconn server call goes through `server/nexconn`; every GitHub call through `server/github`; queue rules live only in `server/queue` (pure, table-tested).
- Commits: `<type>: <description>` with type in feat, fix, refactor, docs, test, chore; SRS IDs in the body.

## Workflow (Claude Code team tips + ECC loop + Ponytail)
- **Ponytail ladder before code:** does it need to exist → already in this repo → stdlib → platform → installed dependency → one line → only then the minimum that works. Read the code the change touches first. Never cut validation, error handling, security or anything requested. Deliberate shortcuts get a `ponytail:` comment naming the ceiling and upgrade trigger.
- **Plan:** complex task → plan mode; `plan-reviewer` agent reviews the plan as a staff engineer; re-plan when something goes sideways.
- **Build:** research/reuse first (`search-first`); tests first for pure logic (RED → GREEN commits); fixtures + `/smoke` for network code; then `/code-review` from a fresh context, `verification-loop`, and the `verify-app` agent.
- **Commit:** `/ponytail-review` on the diff; "Grill me on these changes and don't commit until I pass your test"; "Prove to me this works" with real output; `/commit-phase`.
- **Phase close:** `/phase-close` (review, verification loop, `/ponytail-audit`, notes, prune this file). Independent phases run in git worktrees under `.claude/worktrees/`.
- **Memory:** supermemory recalls past sessions; `notes/` is the shared record for every agent. Never print secrets: session text goes to the memory extractor.
- **Publish:** `/security-scan` and `npx -y ecc-agentshield scan --path .` first.

## Mistakes and rules
Append one line after every correction ("Update your CLAUDE.md so you don't make that mistake again"). Prune at the end of each day.

- (none yet)
