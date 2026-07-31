-- Eligibility rules for competitions (competitions.rules.Filter,
-- competitions.eligibility.go). Existing competitions (the weekly sprint) get
-- eligibility_filter = NULL, which the Go layer treats as "admits everyone" —
-- identical to their behavior before this migration.
--
-- competition_entry_snapshot_positions.sector is captured once at join time
-- (from the SectorProvider configured on competitions.Service) so eligibility
-- can be re-evaluated from the durable snapshot without a second instrument
-- lookup, and so a later re-classification of an instrument never rewrites
-- history for an entry that already joined.

ALTER TABLE competitions
    ADD COLUMN IF NOT EXISTS eligibility_filter JSONB;

ALTER TABLE competition_entry_snapshot_positions
    ADD COLUMN IF NOT EXISTS sector TEXT NOT NULL DEFAULT 'unknown';
