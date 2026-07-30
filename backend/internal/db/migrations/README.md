# Migration ordering

Migrations apply in **lexical order of the full filename** (`sort.Strings`
over the directory listing in `RunMigrations`), not by numeric prefix alone.

Two prefixes are intentionally duplicated in this history:

- `0014_archive_cash.sql` / `0014_income_fees_corporate_actions.sql`
- `0015_corporate_actions.sql` / `0015_portfolio_idempotency_scope.sql`

This looks like a merge artifact, but **do not renumber or rename these (or
any already-shipped migration) to "fix" it.** `schema_migrations` records the
applied set by the migration's exact filename string. Any environment that has
already run these — including this repository's own local Postgres — has rows
like `0014_archive_cash.sql` in `schema_migrations`. Renaming the file makes
`RunMigrations` treat it as a brand-new, unapplied migration and try to
re-execute its SQL against a database that already has that schema, which
fails outright for any non-idempotent statement (a plain `ALTER TABLE ADD
COLUMN` without `IF NOT EXISTS`, for instance) and corrupts migration tracking
everywhere this has already deployed.

Within each duplicated prefix, ordering is still fully deterministic — Go's
string comparison breaks the tie (`archive_cash` < `income_fees_...`,
`corporate_actions` < `portfolio_idempotency_scope`) — and
`TestMigrationOrdering` in `postgres_test.go` pins that order down, so a
regression here would fail CI rather than surface as a silent ordering change.

If you need a genuinely later step, just pick the next unused number (the
directory currently runs through `00NN`, check the highest and add one) —
don't touch history to make the prefixes look tidy.

# Forward-only, no rollback

There are no down-migrations for anything in this directory, and none are
planned. `RunMigrations` only ever applies files forward; there is no
"undo N" path.

This is a deliberate tradeoff, not an oversight: several of these migrations
drop constraints, backfill data, or restructure tables in ways where a
mechanically-generated reverse would not actually be safe to run (e.g. it
would silently discard data written under the new schema). Writing a correct
down-migration requires the same care as writing the forward one, on a
case-by-case basis.

**Consequence:** if a schema release turns out to be bad, the only recovery
path is restoring from backup — there is no "roll the schema back" button.
Plan releases (and the backup/restore drill) with that in mind.
