# Office Hours

Live office hours for open-source projects. Contributors raise a hand with their question, the maintainer takes them one at a time, and every answer becomes a public FAQ entry with a permanent link, so nobody has to answer the same question twice.

Built with the [Nexconn](https://www.nexconn.ai) Chat SDK and Chat UI for the Nexconn Build Challenge 2026. Go server, TypeScript UI.

> Status: in development. See `docs/PHASES.md`.

## Docs
- `docs/PRD.md`: problem, users, goals, scope
- `docs/SRS.md`: requirements (IDs used by specs, tests and commits)
- `docs/ARCHITECTURE.md`: design, data model, Nexconn mapping
- `docs/PHASES.md`: build plan and gates
- `docs/WORKFLOW.md`: how we build (agents included)

## Run locally
```bash
cp .env.example .env   # fill in the values
go -C server mod download && npm --prefix web ci
bash tools/dev.sh              # API, auth and FAQ pages on :8080 (loads .env)
npm --prefix web run dev       # UI on :5173 (proxies to :8080)
```
