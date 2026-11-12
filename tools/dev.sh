#!/usr/bin/env bash
# Run the Go server with .env loaded (Go doesn't read .env itself). Prints no values.
set -euo pipefail
cd "$(dirname "$0")/.."
[ -f .env ] || { echo "Missing .env: copy .env.example and fill it in." >&2; exit 1; }
set -a; . ./.env; set +a
exec go -C server run .
