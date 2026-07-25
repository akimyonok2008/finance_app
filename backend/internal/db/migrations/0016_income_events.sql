-- Automatic provider-driven income pipeline: normalized issuer/fund income
-- events and their per-portfolio application records. Forward-only and
-- idempotent.
--
-- Routine income (dividends, ETF/fund distributions, bond coupons, return of
-- capital, stock dividends) is detected and applied automatically by the
-- background worker; users never enter ordinary income. Uniqueness guarantees an
-- event credits a portfolio at most once, even across multiple worker instances.

CREATE TABLE IF NOT EXISTS income_events (
    id                 TEXT PRIMARY KEY,            -- provider:provider_event_id
    provider           TEXT NOT NULL,
    provider_event_id  TEXT NOT NULL,
    event_type         TEXT NOT NULL,
    instrument_symbol  TEXT NOT NULL,
    amount_per_unit    NUMERIC(24,10) NOT NULL DEFAULT 0,
    currency           TEXT NOT NULL,
    declaration_at     TIMESTAMPTZ,
    ex_date            TIMESTAMPTZ,
    record_date        TIMESTAMPTZ,
    payment_date       TIMESTAMPTZ NOT NULL,
    status             TEXT NOT NULL,
    quality            TEXT NOT NULL,
    tax_classification TEXT,
    normalized_payload JSONB NOT NULL DEFAULT '{}'::jsonb,
    source_url         TEXT,
    raw_fingerprint    TEXT NOT NULL,
    retrieved_at       TIMESTAMPTZ NOT NULL,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT income_events_provider_event_uk UNIQUE (provider, provider_event_id)
);

CREATE INDEX IF NOT EXISTS income_events_status_idx
    ON income_events (status, payment_date);
CREATE INDEX IF NOT EXISTS income_events_symbol_idx
    ON income_events (instrument_symbol);

CREATE TABLE IF NOT EXISTS income_event_applications (
    income_event_id            TEXT NOT NULL REFERENCES income_events(id) ON DELETE CASCADE,
    portfolio_id               UUID NOT NULL REFERENCES portfolios(id) ON DELETE CASCADE,
    user_id                    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    status                     TEXT NOT NULL,
    eligible_quantity          NUMERIC(24,10) NOT NULL DEFAULT 0,
    gross_amount               NUMERIC(24,10) NOT NULL DEFAULT 0,
    withholding_amount         NUMERIC(24,10) NOT NULL DEFAULT 0,
    fee_amount                 NUMERIC(24,10) NOT NULL DEFAULT 0,
    net_amount                 NUMERIC(24,10) NOT NULL DEFAULT 0,
    cash_currency              TEXT,
    reinvestment_quantity      NUMERIC(24,10) NOT NULL DEFAULT 0,
    estimated                  BOOLEAN NOT NULL DEFAULT false,
    portfolio_version_before   BIGINT,
    portfolio_version_after    BIGINT,
    performance_version_before BIGINT,
    performance_version_after  BIGINT,
    applied_at                 TIMESTAMPTZ,
    error_code                 TEXT,
    retry_count                INT NOT NULL DEFAULT 0,
    next_retry_at              TIMESTAMPTZ,
    created_at                 TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at                 TIMESTAMPTZ NOT NULL DEFAULT now(),
    -- The same event credits a portfolio at most ONCE.
    PRIMARY KEY (income_event_id, portfolio_id)
);

CREATE INDEX IF NOT EXISTS income_event_applications_user_idx
    ON income_event_applications (user_id, created_at DESC);
CREATE INDEX IF NOT EXISTS income_event_applications_status_idx
    ON income_event_applications (status, next_retry_at);
