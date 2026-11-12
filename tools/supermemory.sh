#!/usr/bin/env bash
# Local supermemory server for this repo: data in .supermemory/, keys in .env (both git-ignored). http://localhost:6768
# Port 6768 (not 6767) so it never mixes with ~/ethglobal's memory server.
# Loads ONLY its own keys: .env also holds app secrets this process must never see.
cd "$(dirname "$0")/.." || exit 1
# Parse KEY=value lines without eval (a value containing $( or a backtick must never run)
while IFS='=' read -r k v; do v="${v%\"}"; v="${v#\"}"; v="${v%\'}"; v="${v#\'}"; export "$k=$v"; done \
  < <(grep -E '^(SUPERMEMORY_API_KEY|OPENAI_[A-Z_]*)=' .env)
SUPERMEMORY_PORT=6768 SUPERMEMORY_DATA_DIR="$PWD/.supermemory" SUPERMEMORY_DISABLE_TELEMETRY=1 SUPERMEMORY_NO_OPEN=1 \
  SUPERMEMORY_HIDE_API_KEY_IN_LOGS=1 exec "$HOME/.local/bin/supermemory-server" "$@"
