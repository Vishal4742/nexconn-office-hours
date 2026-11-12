#!/usr/bin/env bash
# Run the Go server with .env loaded (Go doesn't read .env itself). Prints no values.
set -euo pipefail
cd "$(dirname "$0")/.."
[ -f .env ] || { echo "Missing .env: copy .env.example and fill it in." >&2; exit 1; }
set -a; . ./.env; set +a
# The server reads DATABASE_URL and nothing else. Postgres' own variables (PGUSER, PGHOST, …)
# would fill in what that address leaves out, so a database's settings pasted into .env would
# quietly change which user and server the local app connects as.
unset PGHOST PGHOSTADDR PGPORT PGDATABASE PGUSER PGPASSWORD PGSERVICE PGSSLMODE PGOPTIONS
exec go -C server run .
