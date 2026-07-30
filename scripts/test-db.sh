#!/usr/bin/env bash
# Starts/stops the disposable PostgreSQL used for DATABASE_URL_TEST-gated
# Go integration tests (backend/docker-compose.test.yml). Always use this
# instead of pointing DATABASE_URL_TEST at the dev stack's postgres
# (localhost:5432) — those integration tests insert rows and never clean
# them up, on the assumption that the database they run against is
# disposable. This one is: tmpfs storage, a port of its own (5434), gone the
# moment you run `down`.
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
compose=(docker compose -f "$repo_root/backend/docker-compose.test.yml")

case "${1:-}" in
  up)
    "${compose[@]}" up --detach --wait
    echo "Test database ready. Export this before running tests:"
    echo
    echo '  export DATABASE_URL_TEST="postgres://postgres:postgres@127.0.0.1:5434/finance_app?sslmode=disable"'
    ;;
  down)
    "${compose[@]}" down --volumes --remove-orphans
    ;;
  *)
    echo "usage: $0 {up|down}" >&2
    exit 1
    ;;
esac
