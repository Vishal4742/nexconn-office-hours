# Office Hours

Live office hours for open-source projects. Contributors raise a hand with their question, the maintainer takes them one at a time, and every answer becomes a public FAQ entry with a permanent link, so nobody has to answer the same question twice.

Built with the [Nexconn](https://www.nexconn.ai) Chat SDK and Chat UI for the Nexconn Build Challenge 2026. Go server, TypeScript UI.

Live at https://office-hours-oss.vercel.app

## Use it from an AI agent
An agent can do what its person can, with the same checks:
- **MCP server:** `https://office-hours-oss.vercel.app/mcp`. It speaks Streamable HTTP, the 2026-07-28 revision and the 2025 ones.
  - Add it in Claude, ChatGPT, Cursor or any app that speaks MCP. It asks you to sign in with GitHub and to allow the app.
  - The app can then read queues and answers and raise a hand. As a maintainer, it can also run a session.
  - With Claude Code: `claude mcp add --transport http office-hours https://office-hours-oss.vercel.app/mcp`.
  - You can disconnect an app from your dashboard.
- **In the browser:** an open room offers its actions to browser agents through WebMCP (`document.modelContext`).
- **To read:** the site has a [`/llms.txt`](https://office-hours-oss.vercel.app/llms.txt), each project has `/p/{owner}/{repo}/llms.txt`, and each answer has a Markdown version at `/p/{owner}/{repo}/faq/{slug}.md`. Answer pages carry schema.org `FAQPage` data.

## Docs
- `docs/UI.md`: UI direction and screens

## Run locally
```bash
cp .env.example .env   # fill in the values
go -C server mod download && npm --prefix web ci
bash tools/dev.sh              # API, auth and FAQ pages on :8080 (loads .env)
npm --prefix web run dev       # UI on :5173 (proxies to :8080)
```
