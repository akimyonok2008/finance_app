CREATE TABLE IF NOT EXISTS provider_rate_limits (
    provider TEXT NOT NULL,
    window_key TEXT NOT NULL,
    request_count INTEGER NOT NULL DEFAULT 0,
    day_key TEXT NOT NULL,
    daily_count INTEGER NOT NULL DEFAULT 0,
    backoff_until TIMESTAMPTZ,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (provider, window_key, day_key)
);

CREATE INDEX IF NOT EXISTS idx_provider_rate_limits_updated_at ON provider_rate_limits (updated_at);
