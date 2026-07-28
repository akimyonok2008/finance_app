#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$repo_root"

compose=(docker compose -f .github/ci/browser-e2e.yml)
cleanup() {
  "${compose[@]}" down --volumes --remove-orphans
}
trap cleanup EXIT

"${compose[@]}" up --detach --build --wait

export E2E_BASE_URL="${E2E_BASE_URL:-http://127.0.0.1:18091}"
export E2E_API_URL="${E2E_API_URL:-http://127.0.0.1:18090}"
export E2E_MAILPIT_URL="${E2E_MAILPIT_URL:-http://127.0.0.1:18092}"
export E2E_DATABASE_URL="${E2E_DATABASE_URL:-postgres://postgres:postgres@127.0.0.1:15433/finance_app?sslmode=disable}"

(
  cd frontend
  npm run test:e2e
)
