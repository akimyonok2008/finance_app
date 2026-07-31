-- Competition price observations were keyed and de-duplicated by symbol
-- alone, inconsistent with the stable instrument identity layer (migration
-- 0020/0037): symbols can be reused, cross-listed, or changed by corporate
-- actions, so a raw ticker string is not a safe uniqueness key. instrument_id
-- becomes the authoritative key when a snapshot resolved one; symbol remains
-- the fallback for legacy/unresolved snapshots, exactly mirroring the
-- positions precedent in migration 0022.
ALTER TABLE competition_price_observations
    ADD COLUMN IF NOT EXISTS instrument_id UUID REFERENCES instrument_master(id),
    ADD COLUMN IF NOT EXISTS venue_mic TEXT;

ALTER TABLE competition_price_observations DROP CONSTRAINT IF EXISTS competition_price_observations_observation_set_id_symbol_key;

CREATE UNIQUE INDEX IF NOT EXISTS competition_price_observations_set_instrument_uidx
    ON competition_price_observations (observation_set_id, instrument_id)
    WHERE instrument_id IS NOT NULL;

CREATE UNIQUE INDEX IF NOT EXISTS competition_price_observations_set_legacy_symbol_uidx
    ON competition_price_observations (observation_set_id, symbol)
    WHERE instrument_id IS NULL;
