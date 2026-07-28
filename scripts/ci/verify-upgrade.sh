#!/usr/bin/env bash
set -euo pipefail

: "${POSTGRES_ADMIN_URL:?POSTGRES_ADMIN_URL is required}"
: "${UPGRADE_DATABASE_URL:?UPGRADE_DATABASE_URL is required}"
: "${API_BINARY:?API_BINARY is required}"

upgrade_db="${UPGRADE_DATABASE_NAME:-finance_app_upgrade_verify}"
if [[ ! "$upgrade_db" =~ ^[a-z0-9_]+_upgrade_verify$ ]]; then
  echo "UPGRADE_DATABASE_NAME must be a dedicated *_upgrade_verify database" >&2
  exit 1
fi

cleanup() {
  psql "$POSTGRES_ADMIN_URL" -v ON_ERROR_STOP=1 -c \
    "SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = '$upgrade_db' AND pid <> pg_backend_pid()" >/dev/null
  psql "$POSTGRES_ADMIN_URL" -v ON_ERROR_STOP=1 -c "DROP DATABASE IF EXISTS \"$upgrade_db\"" >/dev/null
}
trap cleanup EXIT
cleanup
psql "$POSTGRES_ADMIN_URL" -v ON_ERROR_STOP=1 -c "CREATE DATABASE \"$upgrade_db\"" >/dev/null

psql "$UPGRADE_DATABASE_URL" -v ON_ERROR_STOP=1 -f backend/internal/db/migrations/0001_init.sql >/dev/null
psql "$UPGRADE_DATABASE_URL" -v ON_ERROR_STOP=1 <<'SQL' >/dev/null
CREATE TABLE schema_migrations (
  version TEXT PRIMARY KEY,
  applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
INSERT INTO schema_migrations(version) VALUES ('0001_init.sql');
SQL

DATABASE_URL="$UPGRADE_DATABASE_URL" PORT=18082 API_BINARY="$API_BINARY" \
  API_LOG_FILE=upgrade-first-start.log scripts/ci/check-api-startup.sh
DATABASE_URL="$UPGRADE_DATABASE_URL" PORT=18082 API_BINARY="$API_BINARY" \
  API_LOG_FILE=upgrade-repeat-start.log scripts/ci/check-api-startup.sh

expected="$(find backend/internal/db/migrations -maxdepth 1 -name '*.sql' | wc -l | tr -d ' ')"
actual="$(psql "$UPGRADE_DATABASE_URL" -Atc 'SELECT count(*) FROM schema_migrations')"
test "$actual" = "$expected"
echo "oldest supported schema upgraded repeat-safely through $actual migrations"
