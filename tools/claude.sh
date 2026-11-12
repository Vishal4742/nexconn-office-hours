#!/usr/bin/env bash
# Start Claude Code in this repo with supermemory on: passes ONLY the supermemory key from .env (no app secrets).
cd "$(dirname "$0")/.." || exit 1
SUPERMEMORY_CC_API_KEY="$(grep -m1 '^SUPERMEMORY_CC_API_KEY=' .env 2>/dev/null | cut -d= -f2- | tr -d "\"'")"
[ -n "$SUPERMEMORY_CC_API_KEY" ] || echo "warn: SUPERMEMORY_CC_API_KEY missing in .env; memory will be off (see .env.example)" >&2
export SUPERMEMORY_CC_API_KEY
bash tools/memory-up.sh
exec claude "$@"
