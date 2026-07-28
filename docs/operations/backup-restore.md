# PostgreSQL backup and restore verification

The supported backup format is PostgreSQL custom format (`pg_dump -Fc`). It is
portable across hosts, supports selective inspection, and is restored with
`pg_restore`.

## Automated release check

Every manually dispatched `Release candidate` workflow:

1. starts a clean PostgreSQL 16 service;
2. runs the application so the source schema reaches the current migration;
3. creates a custom-format backup;
4. restores it into a separate, dedicated `*_restore_verify` database;
5. compares migration history and starts the application against the restored
   database;
6. drops the temporary restore database.

This verifies the procedure, not the durability of a production provider's
backup storage. Production still needs encrypted, access-controlled,
off-account retention and periodic restore drills using a sanitized snapshot.

## Manual verification

Install matching PostgreSQL client tools, build the API, and provide four
connection strings. The restore target is destructive and must be a dedicated
database whose name ends in `_restore_verify`.

```bash
cd /path/to/finance_app
go -C backend build -o ../.ci-finance-api ./cmd/api

SOURCE_DATABASE_URL='postgres://.../finance_app?sslmode=require' \
POSTGRES_ADMIN_URL='postgres://.../postgres?sslmode=require' \
RESTORE_DATABASE_NAME='finance_app_restore_verify' \
RESTORE_DATABASE_URL='postgres://.../finance_app_restore_verify?sslmode=require' \
API_BINARY="$PWD/.ci-finance-api" \
scripts/ci/verify-backup-restore.sh
```

The script refuses to use the same source and restore URL, validates the
dedicated restore database suffix, removes its temporary dump, and drops the
restore database on exit. It never modifies or drops the source database.

## Production procedure

- Take a provider snapshot or `pg_dump -Fc` backup before every schema release.
- Record the database identifier, application commit SHA, PostgreSQL version,
  UTC timestamp, encryption key reference, and backup checksum.
- Restore into an isolated database; never test a restore over production.
- Start the exact SHA-tagged backend image against the restored database and
  require `/ready` to return 200.
- Verify authentication, portfolio reads, leaderboard reads, outbox backlog,
  and row counts from the provider's reconciliation checklist.
- Retain evidence of the restore test and delete the isolated copy according to
  the data-retention policy.

Recovery time and recovery point objectives depend on the production database
provider and must be documented before launch. A successful CI restore is not
a substitute for provider-level point-in-time recovery testing.
