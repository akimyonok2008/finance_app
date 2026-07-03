CREATE TABLE IF NOT EXISTS instruments (
    symbol TEXT PRIMARY KEY,
    display_symbol TEXT NOT NULL,
    name TEXT NOT NULL,
    exchange TEXT NOT NULL,
    country TEXT NOT NULL,
    currency TEXT NOT NULL,
    asset_type TEXT NOT NULL,
    provider TEXT NOT NULL,
    provider_symbol TEXT NOT NULL,
    is_active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_seen_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_instruments_symbol_prefix ON instruments (symbol text_pattern_ops);
CREATE INDEX IF NOT EXISTS idx_instruments_name ON instruments (name);
CREATE INDEX IF NOT EXISTS idx_instruments_asset_country ON instruments (asset_type, country);

CREATE TABLE IF NOT EXISTS market_quotes (
    symbol TEXT PRIMARY KEY,
    price DOUBLE PRECISION NOT NULL,
    currency TEXT NOT NULL,
    change_percentage DOUBLE PRECISION,
    previous_close DOUBLE PRECISION,
    market_time TIMESTAMPTZ,
    provider TEXT NOT NULL,
    is_delayed BOOLEAN NOT NULL DEFAULT false,
    delay_minutes INTEGER,
    is_stale BOOLEAN NOT NULL DEFAULT false,
    fetched_at TIMESTAMPTZ NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    provider_status TEXT,
    raw_provider_symbol TEXT,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_market_quotes_expires_at ON market_quotes (expires_at);
