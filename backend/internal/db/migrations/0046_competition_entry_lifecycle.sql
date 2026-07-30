-- Arena competition engine, step 3 of 3: entry lifecycle, eligibility
-- evidence, and identity-carrying snapshots.
--
-- Additive and safe against live sprint data: every new column is nullable or
-- defaulted. Pre-existing entries become 'legacy_active' — the sentinel for
-- rows created by the legacy join-anytime flow, which the legacy read paths
-- keep serving unchanged.

ALTER TABLE competition_entries
    ADD COLUMN IF NOT EXISTS entry_status                 TEXT NOT NULL DEFAULT 'legacy_active',
    ADD COLUMN IF NOT EXISTS portfolio_version            BIGINT,
    ADD COLUMN IF NOT EXISTS snapshot_captured_at         TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS eligibility_evidence_json    JSONB,
    ADD COLUMN IF NOT EXISTS scoring_scope                TEXT,
    ADD COLUMN IF NOT EXISTS eligible_starting_value_base NUMERIC(24, 8),
    ADD COLUMN IF NOT EXISTS baseline_status              TEXT,
    ADD COLUMN IF NOT EXISTS baseline_completed_at        TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS disqualification_reason      TEXT,
    ADD COLUMN IF NOT EXISTS withdrawn_at                 TIMESTAMPTZ;

ALTER TABLE competition_entries
    ADD CONSTRAINT competition_entries_status_valid CHECK (entry_status IN (
        'legacy_active',    -- pre-engine sprint entry (join-time baseline)
        'admitted',         -- joined during the window, awaiting common baseline
        'active',           -- baselined at starts_at, being ranked
        'withdrawn',        -- left before join_closes_at
        'baseline_failed',  -- could not be baselined within the operational window
        'disqualified',
        'finalized'
    ));

ALTER TABLE competition_entries
    ADD CONSTRAINT competition_entries_baseline_status_valid CHECK (
        baseline_status IS NULL OR baseline_status IN ('pending', 'completed', 'failed')
    );

-- The baseline worker claims admitted entries per competition.
CREATE INDEX competition_entries_status_idx
    ON competition_entries (competition_id, entry_status);

-- Snapshot positions gain stable instrument identity plus the frozen
-- classification evidence the entry was admitted under. instrument_id is a
-- reference WITHOUT a foreign key on purpose: the snapshot must remain a
-- self-contained historical record (classification_snapshot_json is the
-- authority for how this row was classified at join time), never invalidated
-- or blocked by later changes in the identity layer.
ALTER TABLE competition_entry_snapshot_positions
    ADD COLUMN IF NOT EXISTS instrument_id                UUID,
    ADD COLUMN IF NOT EXISTS venue_mic                    TEXT,
    ADD COLUMN IF NOT EXISTS classification_snapshot_json JSONB,
    ADD COLUMN IF NOT EXISTS included_in_score            BOOLEAN NOT NULL DEFAULT TRUE,
    ADD COLUMN IF NOT EXISTS starting_weight              NUMERIC(12, 8),
    ADD COLUMN IF NOT EXISTS baseline_price_observed_at   TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS baseline_fx_observed_at      TIMESTAMPTZ;

-- Cash was never part of the legacy snapshot; competitions whose scoring
-- config includes cash freeze it here at join time, alongside the positions.
CREATE TABLE competition_entry_snapshot_cash (
    id                   UUID PRIMARY KEY,
    competition_entry_id UUID NOT NULL REFERENCES competition_entries(id) ON DELETE CASCADE,
    currency             TEXT NOT NULL,
    amount               NUMERIC(24, 8) NOT NULL,
    value_base           NUMERIC(24, 8) NOT NULL,
    included_in_score    BOOLEAN NOT NULL DEFAULT TRUE,
    created_at           TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX competition_entry_snapshot_cash_entry_idx
    ON competition_entry_snapshot_cash (competition_entry_id);
