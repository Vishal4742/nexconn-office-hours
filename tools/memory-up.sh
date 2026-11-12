#!/usr/bin/env bash
# SessionStart hook: start this repo's supermemory server (port 6768) if it isn't running. Idempotent; never blocks.
# Detached (own process session) so it survives the Claude session that started it.
cd "$(dirname "$0")/.." || exit 0
curl -s -m 1 -o /dev/null http://localhost:6768/ && exit 0
grep -q '^SUPERMEMORY_API_KEY=' .env 2>/dev/null || exit 0   # no keys yet: memory stays off, session continues
mkdir -p .supermemory
python3 -I -c 'import subprocess; subprocess.Popen(["bash","tools/supermemory.sh"], stdin=subprocess.DEVNULL, stdout=open(".supermemory/server.log","a"), stderr=subprocess.STDOUT, start_new_session=True)'
exit 0
