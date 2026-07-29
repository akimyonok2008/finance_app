#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$repo_root"

compose=(docker compose -f .github/ci/docker-smoke.yml)
ready() {
  "${compose[@]}" exec -T api wget -q -O /dev/null http://127.0.0.1:9090/ready
}
cleanup() {
  "${compose[@]}" down --volumes --remove-orphans
}
trap cleanup EXIT

"${compose[@]}" up --detach --build --wait

backend_user="$(docker image inspect --format '{{.Config.User}}' finance-app-backend:smoke)"
frontend_user="$(docker image inspect --format '{{.Config.User}}' finance-app-frontend:smoke)"
test -n "$backend_user"
test "$backend_user" != "0"
test "$backend_user" != "root"
test -n "$frontend_user"
test "$frontend_user" != "0"
test "$frontend_user" != "root"

curl --fail --silent http://127.0.0.1:18080/health >/dev/null
ready
curl --fail --silent http://127.0.0.1:18081/health >/dev/null
curl --fail --silent http://127.0.0.1:18081/ >/dev/null
curl --fail --silent http://127.0.0.1:18081/api/health >/dev/null

expected="$(find backend/internal/db/migrations -maxdepth 1 -name '*.sql' | wc -l | tr -d ' ')"
actual="$("${compose[@]}" exec -T postgres psql -U postgres -d finance_app -Atc \
  'SELECT count(*) FROM schema_migrations')"
test "$actual" = "$expected"

# Liveness remains up while readiness fails closed when a required dependency
# disappears. Then recover the dependency and ensure readiness recovers too.
"${compose[@]}" stop postgres
curl --fail --silent http://127.0.0.1:18080/health >/dev/null
for _ in {1..20}; do
  if ! ready; then
    readiness_failed=true
    break
  fi
  sleep 1
done
test "${readiness_failed:-false}" = "true"

"${compose[@]}" start postgres
for _ in {1..30}; do
  if ready; then
    readiness_recovered=true
    break
  fi
  sleep 1
done
test "${readiness_recovered:-false}" = "true"

api_id="$("${compose[@]}" ps -q api)"
"${compose[@]}" stop --timeout 20 api
test "$(docker inspect --format '{{.State.ExitCode}}' "$api_id")" = "0"

echo "production images, non-root users, migrations, readiness, and graceful shutdown verified"
