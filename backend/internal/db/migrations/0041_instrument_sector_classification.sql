-- Persists a coarse GICS-like sector classification on instrument identity
-- (instrument.Sector, instrument.ClassifySector). Nothing downstream reads
-- this yet except the new competitions eligibility path (migration 0042);
-- existing rows backfill to 'unknown' rather than being left NULL, since
-- eligibility rules must be able to treat "not yet classified" as a concrete,
-- comparable value.

ALTER TABLE instrument_master
    ADD COLUMN IF NOT EXISTS sector TEXT NOT NULL DEFAULT 'unknown';

ALTER TABLE instrument_master
    ADD CONSTRAINT instrument_master_sector_valid CHECK (
        sector IN (
            'technology', 'healthcare', 'financials', 'consumer_discretionary',
            'consumer_staples', 'energy', 'industrials', 'materials',
            'utilities', 'real_estate', 'communication_services', 'unknown'
        )
    );

CREATE INDEX IF NOT EXISTS idx_instrument_master_sector ON instrument_master (sector) WHERE sector <> 'unknown';
