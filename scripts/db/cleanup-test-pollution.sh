#!/usr/bin/env bash
# Finds and (optionally) deletes rows created by the Postgres integration
# test suite (backend/*_pg_test.go's seedUser-style helpers) and the E2E
# scale/sprint fixtures, in case they ever land in a non-disposable database
# again despite scripts/test-db.sh. Always dry-runs unless --yes is passed.
#
# Matches the conventions those helpers actually use:
#   - users.email ending in @example.com, @e.com, or @example.test
#   - competitions.id starting with "test_"
# KEEP_EMAILS must list every real account on the target database
# (comma-separated) so this can never delete one by accident, even if a real
# account happens to use one of those domains.
#
# Usage:
#   KEEP_EMAILS="me@gmail.com,other@gmail.com" scripts/db/cleanup-test-pollution.sh "$DATABASE_URL"
#   KEEP_EMAILS="..." scripts/db/cleanup-test-pollution.sh "$DATABASE_URL" --yes
set -euo pipefail

database_url="${1:?usage: $0 <DATABASE_URL> [--yes]}"
apply="${2:-}"

if [[ -z "${KEEP_EMAILS:-}" ]]; then
  echo "error: KEEP_EMAILS must be set to a comma-separated list of every real account's email on this database." >&2
  echo "This is a safety rail — without it, real accounts that happen to share a matched domain would be deleted too." >&2
  exit 1
fi

keep_sql=$(python3 - "$KEEP_EMAILS" <<'PY'
import sys
emails = [e.strip() for e in sys.argv[1].split(",") if e.strip()]
quoted = ",".join("'" + e.replace("'", "''") + "'" for e in emails)
print(quoted if quoted else "''")
PY
)

match_where="(email ~* '@(example\\.com|e\\.com|example\\.test)\$') AND email NOT IN (${keep_sql})"

echo "== users matching test-seed conventions (excluding KEEP_EMAILS) =="
psql "$database_url" -c "SELECT count(*) AS matched_users FROM users WHERE ${match_where};"
psql "$database_url" -c "SELECT email, display_name, created_at FROM users WHERE ${match_where} ORDER BY created_at DESC LIMIT 20;"

echo
echo "== competitions matching the 'test_' id prefix =="
psql "$database_url" -c "SELECT count(*) AS matched_competitions FROM competitions WHERE id LIKE 'test\\_%';"

if [[ "$apply" != "--yes" ]]; then
  echo
  echo "Dry run only — no rows deleted. Re-run with --yes to delete the rows listed above."
  exit 0
fi

echo
echo "Deleting…"
psql "$database_url" -v ON_ERROR_STOP=1 <<SQL
BEGIN;
DELETE FROM users WHERE ${match_where};
DELETE FROM competitions WHERE id LIKE 'test\_%';
COMMIT;
SQL
echo "Done."
