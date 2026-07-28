#!/usr/bin/env bash
set -euo pipefail

: "${SOURCE_DATABASE_URL:?SOURCE_DATABASE_URL is required}"
: "${POSTGRES_ADMIN_URL:?POSTGRES_ADMIN_URL is required}"
: "${RESTORE_DATABASE_URL:?RESTORE_DATABASE_URL is required}"
: "${API_BINARY:?API_BINARY is required}"

restore_db="${RESTORE_DATABASE_NAME:-finance_app_restore_verify}"
if [[ ! "$restore_db" =~ ^[a-z0-9_]+_restore_verify$ ]]; then
  echo "RESTORE_DATABASE_NAME must be a dedicated *_restore_verify database" >&2
  exit 1
fi
if [[ "$SOURCE_DATABASE_URL" == "$RESTORE_DATABASE_URL" ]]; then
  echo "source and restore databases must differ" >&2
  exit 1
fi

backup_file="$(mktemp "${TMPDIR:-/tmp}/finance-app-backup.XXXXXX.dump")"
cleanup() {
  psql "$POSTGRES_ADMIN_URL" -v ON_ERROR_STOP=1 -c \
    "SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = '$restore_db' AND pid <> pg_backend_pid()" >/dev/null
  psql "$POSTGRES_ADMIN_URL" -v ON_ERROR_STOP=1 -c "DROP DATABASE IF EXISTS \"$restore_db\"" >/dev/null
  rm -f "$backup_file"
}
trap cleanup EXIT
cleanup

pg_dump --format=custom --no-owner --no-privileges --file="$backup_file" "$SOURCE_DATABASE_URL"
psql "$POSTGRES_ADMIN_URL" -v ON_ERROR_STOP=1 -c "CREATE DATABASE \"$restore_db\"" >/dev/null
pg_restore --exit-on-error --no-owner --no-privileges --dbname="$RESTORE_DATABASE_URL" "$backup_file"

source_versions="$(psql "$SOURCE_DATABASE_URL" -Atc 'SELECT count(*) FROM schema_migrations')"
restore_versions="$(psql "$RESTORE_DATABASE_URL" -Atc 'SELECT count(*) FROM schema_migrations')"
test "$source_versions" = "$restore_versions"

DATABASE_URL="$RESTORE_DATABASE_URL" PORT=18083 API_BINARY="$API_BINARY" \
  API_LOG_FILE=restore-startup.log scripts/ci/check-api-startup.sh
echo "backup restored and application verified against $restore_versions migrations"
