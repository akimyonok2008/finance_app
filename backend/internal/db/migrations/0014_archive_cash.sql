-- Owner-private archive extension for cash-aware portfolio value.
ALTER TABLE portfolio_archive_snapshots
    ADD COLUMN IF NOT EXISTS total_cash_value_base NUMERIC(24, 8) NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS cash_balances_snapshot JSONB NOT NULL DEFAULT '[]'::jsonb;
