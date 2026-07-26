-- 0020_instrument_identity.sql — stable instrument identity layer.
--
-- Why: the domain identifies instruments by ticker symbol string (positions.symbol,
-- portfolio_activities.symbol). Tickers change (FB -> META), get reused after a
-- delisting, and collide across exchanges. A ticker is therefore not an identity.
-- This migration introduces an immutable internal identity plus a time-scoped
-- alias table so "who held instrument X at time T" survives renames.
--
-- NAME COLLISION NOTE: an `instruments` table already exists (migration 0004,
-- market-data search catalog, PRIMARY KEY (symbol)). It is a *searchable symbol
-- catalog*, not an identity register, and is deliberately left untouched. The
-- identity register is therefore named `instrument_master`.
--
-- Forward-only and non-destructive: nothing is dropped, no historical symbol
-- column is rewritten, no NOT NULL is added to an existing table, and the new
-- instrument_id columns are nullable with no backfill (backfill is a later slice).

CREATE TABLE IF NOT EXISTS instrument_master (
    -- Internal immutable identity. Deliberately NOT the FIGI: an instrument may
    -- be recorded before it is resolved, and provider identifiers can be
    -- corrected without invalidating every foreign key that points here.
    id                UUID PRIMARY KEY,
    figi              TEXT,
    composite_figi    TEXT,
    share_class_figi  TEXT,
    isin              TEXT,
    cusip             TEXT,
    cik               TEXT,
    current_symbol    TEXT,
    name              TEXT,
    security_type     TEXT,
    asset_type        TEXT,
    exchange_code     TEXT,
    mic               TEXT,
    currency          TEXT,
    country           TEXT,
    status            TEXT NOT NULL DEFAULT 'active',
    listed_at         TIMESTAMPTZ,
    delisted_at       TIMESTAMPTZ,
    -- verified | resolved | ambiguous | unresolved
    identity_quality  TEXT NOT NULL DEFAULT 'unresolved',
    identity_provider TEXT,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- A FIGI is globally unique when present; unresolved rows carry NULL and are
-- not constrained against each other (partial index semantics).
CREATE UNIQUE INDEX IF NOT EXISTS instrument_master_figi_key
    ON instrument_master (figi) WHERE figi IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_instrument_master_current_symbol
    ON instrument_master (current_symbol);

CREATE TABLE IF NOT EXISTS instrument_aliases (
    id                UUID PRIMARY KEY,
    instrument_id     UUID NOT NULL REFERENCES instrument_master(id) ON DELETE CASCADE,
    alias_type        TEXT NOT NULL,
    alias_value       TEXT NOT NULL,
    exchange_code     TEXT NOT NULL DEFAULT '',
    mic               TEXT NOT NULL DEFAULT '',
    valid_from        TIMESTAMPTZ NOT NULL DEFAULT now(),
    -- NULL means "still active". A ticker change closes the old row by setting
    -- valid_to; the historical row is retained so past holdings stay resolvable.
    valid_to          TIMESTAMPTZ,
    provider          TEXT NOT NULL DEFAULT '',
    provider_event_id TEXT NOT NULL DEFAULT '',
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT instrument_alias_type_valid CHECK (
        alias_type IN ('ticker', 'figi', 'composite_figi', 'share_class_figi',
                       'isin', 'cusip', 'cik', 'provider_symbol')
    )
);

-- Uniqueness is scoped to the ACTIVE window only: (alias_type, alias_value,
-- exchange_code, mic) may appear many times historically (ticker reuse), but at
-- most once with valid_to IS NULL. A globally unique (alias_type, alias_value)
-- would be wrong -- tickers are not globally unique.
CREATE UNIQUE INDEX IF NOT EXISTS instrument_aliases_active_key
    ON instrument_aliases (alias_type, alias_value, exchange_code, mic)
    WHERE valid_to IS NULL;

CREATE INDEX IF NOT EXISTS idx_instrument_aliases_lookup
    ON instrument_aliases (alias_type, alias_value, mic);

CREATE INDEX IF NOT EXISTS idx_instrument_aliases_instrument
    ON instrument_aliases (instrument_id);

-- Nullable identity columns on the existing ledger tables. No backfill, no
-- NOT NULL: legacy rows keep working on the symbol string alone until a later
-- slice wires resolution into the buy path and backfills unambiguous history.
ALTER TABLE positions
    ADD COLUMN IF NOT EXISTS instrument_id UUID REFERENCES instrument_master(id);

ALTER TABLE portfolio_activities
    ADD COLUMN IF NOT EXISTS instrument_id UUID REFERENCES instrument_master(id);

CREATE INDEX IF NOT EXISTS idx_positions_instrument_id
    ON positions (instrument_id) WHERE instrument_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_portfolio_activities_instrument_id
    ON portfolio_activities (instrument_id) WHERE instrument_id IS NOT NULL;

-- income_events / corporate_actions intentionally untouched in this slice: they
-- are keyed by symbol today and rewiring them belongs with entitlement
-- discovery, which is a later slice.
