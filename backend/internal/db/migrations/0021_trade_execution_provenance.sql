-- Accurate buy/sell recording: execution-price and fee provenance, a distinct
-- system-entry timestamp, and the portfolio-level automatic-funding preference.
--
-- Forward-only, idempotent, non-destructive. Nothing here is NOT NULL on
-- existing rows: historical activities predate provenance, so they are
-- backfilled to 'legacy_unknown' only where that is deterministically safe.
--
-- Columns 0012-0020 already provide: occurred_at (the effective/real-world
-- instant), created_at (row insertion), position_episode_id, group id via
-- metadata_json->>'activity_group_id'. This migration adds only what is
-- genuinely missing.

ALTER TABLE portfolio_activities
    ADD COLUMN IF NOT EXISTS execution_price_source TEXT,
    ADD COLUMN IF NOT EXISTS fee_source TEXT,
    -- recorded_at is when the activity was ENTERED INTO THE SYSTEM, as distinct
    -- from occurred_at (when the real-world trade happened). Backdated
    -- transactions have recorded_at > occurred_at.
    ADD COLUMN IF NOT EXISTS recorded_at TIMESTAMPTZ;

UPDATE portfolio_activities
SET recorded_at = created_at
WHERE recorded_at IS NULL;

-- Backfill provenance for pre-existing trades. Every legacy buy/sell/opening
-- balance was priced from the tracked market quote with no user-entered
-- execution detail and no recorded fee, but we cannot prove that per row, so
-- they are labelled 'legacy_unknown' rather than claiming a provenance.
UPDATE portfolio_activities
SET execution_price_source = 'legacy_unknown'
WHERE execution_price_source IS NULL
  AND activity_type IN ('buy', 'sell', 'opening_balance');

UPDATE portfolio_activities
SET fee_source = 'legacy_unknown'
WHERE fee_source IS NULL
  AND activity_type IN ('buy', 'sell');

ALTER TABLE portfolio_activities
    DROP CONSTRAINT IF EXISTS portfolio_activity_price_source_valid,
    DROP CONSTRAINT IF EXISTS portfolio_activity_fee_source_valid;

ALTER TABLE portfolio_activities
    ADD CONSTRAINT portfolio_activity_price_source_valid CHECK (
        execution_price_source IS NULL
        OR execution_price_source IN ('user_recorded', 'provider_estimate', 'legacy_unknown')
    ),
    ADD CONSTRAINT portfolio_activity_fee_source_valid CHECK (
        fee_source IS NULL
        OR fee_source IN ('user_recorded', 'default_zero', 'legacy_unknown')
    );

CREATE INDEX IF NOT EXISTS portfolio_activities_group_idx
    ON portfolio_activities (portfolio_id, (metadata_json->>'activity_group_id'), occurred_at)
    WHERE metadata_json ? 'activity_group_id';

-- Portfolio preference: automatic purchase funding. Default TRUE — recording a
-- real purchase must not be blocked by a cash balance the user never asked to
-- manage. Setting it to FALSE restores the strict insufficient-cash rejection.
ALTER TABLE portfolios
    ADD COLUMN IF NOT EXISTS auto_fund_purchases BOOLEAN NOT NULL DEFAULT TRUE;
