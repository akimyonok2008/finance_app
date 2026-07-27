-- Stable identity for provider-discovered income. Symbol remains the
-- provider-facing/historical alias, while instrument_id is the portfolio key.
ALTER TABLE income_events
    ADD COLUMN IF NOT EXISTS instrument_id UUID REFERENCES instrument_master(id);

CREATE INDEX IF NOT EXISTS income_events_instrument_id_idx
    ON income_events (instrument_id)
    WHERE instrument_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS positions_income_discovery_idx
    ON positions (closed_at, instrument_id)
    WHERE COALESCE(status, 'open') = 'closed';
