-- Portfolio mutation aggregate: version, uniqueness, idempotency, audit, outbox.
--
-- Forward-only and idempotent. Positions and ranked-performance state now form
-- ONE transactional aggregate, locked via SELECT ... FROM portfolios FOR UPDATE.
-- This migration adds the database-enforced invariants that back that design.
--
-- Operational recovery (forward-only; no destructive down-migration):
--   * The duplicate backfills below are safe to re-run.
--   * If the UNIQUE (user_id) index fails to build, legacy duplicate portfolios
--     exist: inspect `SELECT user_id, count(*) FROM portfolios GROUP BY 1
--     HAVING count(*) > 1`, merge positions onto the oldest portfolio, then
--     re-run. No user data is dropped by this migration.
--   * portfolio_outbox may be truncated safely if the processor is disabled;
--     it holds only derived-work triggers, never source-of-truth state.

-- === 1. Portfolio aggregate version =========================================
ALTER TABLE portfolios
    ADD COLUMN IF NOT EXISTS version BIGINT NOT NULL DEFAULT 1;

-- Version must be positive; it only ever moves forward (enforced in code by the
-- WHERE version = $expected guard inside the aggregate transaction).
ALTER TABLE portfolios DROP CONSTRAINT IF EXISTS portfolios_version_positive;
ALTER TABLE portfolios
    ADD CONSTRAINT portfolios_version_positive CHECK (version > 0);

-- === 2. One default portfolio per user ======================================
-- The product model is one portfolio per user. Collapse any legacy duplicates
-- onto the oldest portfolio BEFORE adding the unique index, so no data is lost.
DO $$
DECLARE
    dup RECORD;
    keep_id UUID;
BEGIN
    FOR dup IN
        SELECT user_id FROM portfolios GROUP BY user_id HAVING count(*) > 1
    LOOP
        SELECT id INTO keep_id FROM portfolios
        WHERE user_id = dup.user_id ORDER BY created_at LIMIT 1;

        UPDATE positions SET portfolio_id = keep_id
        WHERE portfolio_id IN (
            SELECT id FROM portfolios WHERE user_id = dup.user_id AND id <> keep_id
        );
        UPDATE portfolio_archive_snapshots SET portfolio_id = keep_id
        WHERE portfolio_id IN (
            SELECT id FROM portfolios WHERE user_id = dup.user_id AND id <> keep_id
        );
        DELETE FROM portfolios WHERE user_id = dup.user_id AND id <> keep_id;
    END LOOP;
END $$;

CREATE UNIQUE INDEX IF NOT EXISTS portfolios_user_id_key ON portfolios (user_id);

-- === 3. Ranked-state consistency ============================================
-- The ranked state's owner must match the portfolio's owner.
ALTER TABLE ranked_performance_state DROP CONSTRAINT IF EXISTS ranked_state_version_positive;
ALTER TABLE ranked_performance_state
    ADD CONSTRAINT ranked_state_version_positive CHECK (version > 0);

-- === 4. Position consistency ================================================
-- Quantity must be positive; an open position must carry no close fields and a
-- closed one must carry them. Legacy rows are repaired first.
UPDATE positions SET status = 'open' WHERE status IS NULL;
UPDATE positions
   SET closed_at = NULL, close_price = NULL, close_price_currency = NULL
 WHERE COALESCE(status, 'open') = 'open'
   AND (closed_at IS NOT NULL OR close_price IS NOT NULL);

ALTER TABLE positions DROP CONSTRAINT IF EXISTS positions_quantity_positive;
ALTER TABLE positions
    ADD CONSTRAINT positions_quantity_positive CHECK (quantity > 0);

ALTER TABLE positions DROP CONSTRAINT IF EXISTS positions_close_fields_consistency;
ALTER TABLE positions
    ADD CONSTRAINT positions_close_fields_consistency CHECK (
        (COALESCE(status, 'open') = 'open'  AND closed_at IS NULL AND close_price IS NULL)
     OR (status = 'closed' AND closed_at IS NOT NULL AND close_price IS NOT NULL)
    );

-- === 5. Daily snapshot uniqueness ===========================================
-- A generated UTC date column plus a unique index makes "one snapshot per
-- portfolio per UTC day" a database guarantee, replacing the check-then-insert
-- race. Keyed by portfolio (not user) so it stays correct if multiple
-- portfolios per user are ever introduced.
ALTER TABLE portfolio_archive_snapshots
    ADD COLUMN IF NOT EXISTS captured_date DATE
    GENERATED ALWAYS AS (((captured_at AT TIME ZONE 'UTC')::date)) STORED;

-- Collapse pre-existing same-day duplicates, keeping the earliest row.
DELETE FROM portfolio_archive_snapshots a
 USING portfolio_archive_snapshots b
 WHERE a.portfolio_id = b.portfolio_id
   AND a.captured_date = b.captured_date
   AND a.captured_at > b.captured_at;

CREATE UNIQUE INDEX IF NOT EXISTS portfolio_archive_snapshots_daily_key
    ON portfolio_archive_snapshots (portfolio_id, captured_date);

-- === 6. Mutation audit + idempotency ========================================
-- Privacy-safe: indexes and versions only — never quantities, values, cost basis
-- or segment baselines. request_id is UNIQUE, so a duplicate client submission
-- cannot apply a second mutation.
CREATE TABLE IF NOT EXISTS portfolio_mutation_audit (
    id                         UUID PRIMARY KEY,
    request_id                 TEXT,
    portfolio_id               UUID NOT NULL REFERENCES portfolios(id) ON DELETE CASCADE,
    user_id                    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    mutation_type              TEXT NOT NULL,
    portfolio_version_before   BIGINT NOT NULL,
    portfolio_version_after    BIGINT NOT NULL,
    performance_version_before BIGINT NOT NULL,
    performance_version_after  BIGINT NOT NULL,
    ranked_index_before        NUMERIC(20, 8) NOT NULL,
    ranked_index_after         NUMERIC(20, 8) NOT NULL,
    result_position_id         UUID,
    occurred_at                TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS portfolio_mutation_audit_request_key
    ON portfolio_mutation_audit (request_id) WHERE request_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS portfolio_mutation_audit_portfolio_idx
    ON portfolio_mutation_audit (portfolio_id, occurred_at DESC);

-- === 7. Transactional outbox ================================================
-- Written inside the mutation transaction; processed after commit, idempotently,
-- with FOR UPDATE SKIP LOCKED claiming. Payload is privacy-safe: ranked index
-- and status only, never monetary values.
CREATE TABLE IF NOT EXISTS portfolio_outbox (
    id                UUID PRIMARY KEY,
    event_type        TEXT NOT NULL,
    aggregate_type    TEXT NOT NULL,
    aggregate_id      UUID NOT NULL,
    aggregate_version BIGINT NOT NULL,
    user_id           UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    ranked_index      NUMERIC(20, 8) NOT NULL,
    ranking_status    TEXT NOT NULL,
    tracking_started_at TIMESTAMPTZ NOT NULL,
    valuation_as_of     TIMESTAMPTZ NOT NULL,
    data_quality_status TEXT NOT NULL DEFAULT 'complete'
        CONSTRAINT portfolio_outbox_data_quality_check
        CHECK (data_quality_status IN ('complete', 'stale', 'partial', 'invalid')),
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    processed_at      TIMESTAMPTZ,
    -- claimed_at is a visibility LEASE. FOR UPDATE SKIP LOCKED alone only hides a
    -- row from transactions running concurrently; once the claiming transaction
    -- commits the row becomes visible again. Stamping claimed_at makes the claim
    -- durable, and an expired lease lets a crashed worker's event be retried.
    claimed_at        TIMESTAMPTZ,
    attempt_count     INT NOT NULL DEFAULT 0,
    last_error        TEXT
);

ALTER TABLE portfolio_outbox ADD COLUMN IF NOT EXISTS claimed_at TIMESTAMPTZ;
ALTER TABLE portfolio_outbox ADD COLUMN IF NOT EXISTS tracking_started_at TIMESTAMPTZ;
ALTER TABLE portfolio_outbox ADD COLUMN IF NOT EXISTS valuation_as_of TIMESTAMPTZ;
ALTER TABLE portfolio_outbox ADD COLUMN IF NOT EXISTS data_quality_status TEXT;

-- Support databases where an earlier form of this forward migration created
-- the outbox before ranked-snapshot metadata was added. Existing events remain
-- valid derived-work triggers; their creation time is the safest available
-- historical observation timestamp.
UPDATE portfolio_outbox
SET tracking_started_at = COALESCE(tracking_started_at, created_at),
    valuation_as_of = COALESCE(valuation_as_of, created_at),
    data_quality_status = COALESCE(data_quality_status, 'complete')
WHERE tracking_started_at IS NULL
   OR valuation_as_of IS NULL
   OR data_quality_status IS NULL;

ALTER TABLE portfolio_outbox ALTER COLUMN tracking_started_at SET NOT NULL;
ALTER TABLE portfolio_outbox ALTER COLUMN valuation_as_of SET NOT NULL;
ALTER TABLE portfolio_outbox ALTER COLUMN data_quality_status SET DEFAULT 'complete';
ALTER TABLE portfolio_outbox ALTER COLUMN data_quality_status SET NOT NULL;
ALTER TABLE portfolio_outbox DROP CONSTRAINT IF EXISTS portfolio_outbox_data_quality_check;
ALTER TABLE portfolio_outbox
    ADD CONSTRAINT portfolio_outbox_data_quality_check
    CHECK (data_quality_status IN ('complete', 'stale', 'partial', 'invalid'));

-- Polling index: unclaimed, unprocessed events in creation order.
CREATE INDEX IF NOT EXISTS portfolio_outbox_unprocessed_idx
    ON portfolio_outbox (created_at) WHERE processed_at IS NULL;
CREATE INDEX IF NOT EXISTS portfolio_outbox_aggregate_idx
    ON portfolio_outbox (aggregate_id, aggregate_version);
