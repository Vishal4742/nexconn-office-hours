# Office Hours — rules for every agent (Claude Code, Codex, opencode, subagents)

Live rooms where open-source maintainers answer contributor questions one at a time, and every answer becomes a public FAQ entry. Built on the Nexconn Chat SDK + Chat UI for the **Nexconn Build Challenge** (deadline Nov 22 23:59 AoE; we submit Nov 22 by 20:00 IST). This repo will be **public**.

**Read first:** `docs/WORKFLOW.md` (how to work) → `docs/PHASES.md` (current phase) → latest `notes/phase-N*.md`. Requirements: `docs/SRS.md` (cite its IDs). Design: `docs/ARCHITECTURE.md`. UI: `docs/UI.md`. Product: `docs/PRD.md`.

## Commands
- Stack: **Go server** in `server/` (Go 1.25, `net/http`, pgx, official Nexconn Go SDK) + **TypeScript UI** in `web/` (Vite + React, static). One Vercel project with Services.
- Install: `go -C server mod download` · `npm --prefix web ci` · Dev: `bash tools/dev.sh` (loads `.env`, Go on :8080) + `npm --prefix web run dev` (:5173, proxies `/api`, `/auth`, `/p/*/faq` to Go). `server/` is the Go module root.
- Checks: `gofmt -l server` · `go -C server vet ./...` · `go -C server test ./...` · `npm --prefix web run lint` · `npm --prefix web run typecheck` · `npm --prefix web test` · acceptance tests AT-1…AT-19 by hand on production
- DB: `go -C server run ./cmd/migrate` applies `server/migrations/*.sql` (local `.env` points at a Neon `dev` branch or a local Postgres; production only with `MIGRATE_DATABASE_URL`, before merging code that needs the schema). The store tests need a real Postgres: `TEST_DATABASE_URL='postgres://localhost:5432/officehours_test?sslmode=disable' go -C server test ./...` (skipped without it locally, required in CI)
- Claude Code: start with `bash tools/claude.sh` (supermemory on); `/smoke` runs all checks
- One port, production routing: `vercel dev -L` runs both services behind `vercel.json` (local only, no project link; it reads `.env`).

## Hard rules
- **Judging is 50% "does it work and feel good".** Finish and polish one flow before adding features. Respect the freeze (Nov 19 10:00 IST).
- **Secrets** (Nexconn App Secret, GitHub OAuth secret, `AUTH_SECRET`, `DATABASE_URL`, supermemory keys) live only in a git-ignored `.env` and Vercel env. Never print, log, commit or paste them. Never `cat`/`grep` a `.env`; recursive searches use `--exclude='.env*'` (grep) or `-g '!.env*'` (rg). If a secret is printed anyway, tell the human to rotate it.
- **The App Key may be public** (client + write-up). The App Secret never leaves the server. All Nexconn server calls go through `server/nexconn`.
- **The server is the only writer of queue state.** Every mutation goes through `server/guard` (Origin + JSON + session + role) → input validation → DB transaction → Nexconn push. Roles come from the `maintainers` table, never from the client.
- **Trust OH:\* messages only from the `oh_bot` sender**; build FAQ links from slugs on our own origin; render user text as text.
- **The user's GitHub token** lives only in the encrypted session cookie (scope `read:user`): never in the DB, logs or client JS.
- **Don't guess SDK APIs.** Read the Markdown docs (`https://docs.nexconn.ai/llms.txt` is the index; every page has a `.md` version), or the installed package types. Write what you confirmed in `notes/`.
- **Users are real people:** store only GitHub id, login, name and avatar; no emails; text rendered as text.
- Git: short `slice/*` branches merged to `main` as soon as CI is green (gates run on production); small commits with explicit paths, SRS IDs in commit bodies; never commit `.env`, never `--no-verify`, never force-push `main`. Production deploys only from `main` through GitHub.
- Downloads: ask the human before installing anything new that isn't in `package.json` already (models, global tools, new heavy packages, `npx -y ecc-agentshield`).
- Stuck: same error 3 times → stop, write it in `notes/`, ask the human.
