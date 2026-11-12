---
description: Detect the stack and run lint, types, tests and the smoke script; report PASS/FAIL with real output
allowed-tools: Bash(gofmt:*), Bash(go -C server vet:*), Bash(go -C server test:*), Bash(npm --prefix web run lint:*), Bash(npm --prefix web test:*), Bash(npm --prefix web run typecheck:*), Bash(pytest:*), Bash(python -m pytest:*), Bash(ruff:*), Bash(pyright:*), Bash(mypy:*), Bash(npm test:*), Bash(npm run lint:*), Bash(npm run test:*), Bash(npx tsc:*), Bash(cargo test:*), Bash(cargo clippy:*), Bash(go test:*), Bash(go vet:*), Bash(scripts/smoke.sh:*), Read, Grep
---

## Context
- Branch: !`git branch --show-current`
- Uncommitted: !`git status --porcelain | wc -l`
- Stack markers: !`ls pyproject.toml package.json Cargo.toml go.mod server/go.mod web/package.json 2>/dev/null | tr '\n' ' '`

## Task
Pick the checks for the stack markers present and run them in order (activate the project environment first if `CLAUDE.md` names one):
- `pyproject.toml`: `ruff check . && ruff format --check .`, `pyright` (or `mypy`), `pytest -q --cov`
- `package.json`: `npm run lint`, `npx tsc --noEmit`, `npm test`
- `Cargo.toml`: `cargo clippy -- -D warnings`, `cargo test`
- `go.mod`: `go vet ./...`, `go test ./...`
- `server/go.mod` (this repo): `gofmt -l server` (must print nothing), `go -C server vet ./...`, `go -C server test ./...`
- `web/package.json` (this repo): `npm --prefix web run lint`, `npm --prefix web run typecheck`, `npm --prefix web test`
- `scripts/smoke.sh` if it exists

Report a table: step | PASS/FAIL | one-line evidence (counts, coverage, first failing assertion). Never claim PASS without the command output. If anything fails, propose the fix; apply it only if asked.
