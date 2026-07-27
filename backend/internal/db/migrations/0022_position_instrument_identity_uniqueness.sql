-- Stable instrument identity is now authoritative for resolved open positions.
-- Keep the legacy symbol uniqueness rule only for nullable/unresolved rows; do
-- not backfill or guess identities for existing data.
DROP INDEX IF EXISTS positions_one_open_instrument_uidx;

CREATE UNIQUE INDEX IF NOT EXISTS positions_one_open_stable_instrument_uidx
    ON positions (portfolio_id, instrument_id, asset_type, currency)
    WHERE COALESCE(status, 'open') = 'open'
      AND instrument_id IS NOT NULL;

CREATE UNIQUE INDEX IF NOT EXISTS positions_one_open_legacy_symbol_uidx
    ON positions (portfolio_id, symbol, asset_type, currency)
    WHERE COALESCE(status, 'open') = 'open'
      AND instrument_id IS NULL;
