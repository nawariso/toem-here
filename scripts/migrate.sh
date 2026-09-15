#!/usr/bin/env bash
set -euo pipefail
: "${DATABASE_URL:?DATABASE_URL is required}"
command="${1:-up}"
if [[ "$command" != "up" && "$command" != "down" ]]; then
  printf 'usage: scripts/migrate.sh up|down\n' >&2
  exit 2
fi
go run ./services/api/cmd/migrate "$command"
