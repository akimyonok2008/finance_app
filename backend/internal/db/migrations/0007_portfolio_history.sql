-- Prototype 4 portfolio history foundations.
-- Closing a position is owner-private bookkeeping only; it does not execute a
-- trade. Archive snapshots are owner-private and may contain quantities,
-- values, prices, and realized/unrealized performance data.

ALTER TABLE positions
ADD COLUMN IF NOT EXISTS status TEXT NOT NULL DEFAULT 'open',
ADD COLUMN IF NOT EXISTS closed_at TIMESTAMPTZ,
ADD COLUMN IF NOT EXISTS close_price NUMERIC(24, 8),
ADD COLUMN IF NOT EXISTS close_price_currency TEXT,
ADD COLUMN IF NOT EXISTS realized_gain_loss_base NUMERIC(24, 8) NOT NULL DEFAULT 0,
ADD COLUMN IF NOT EXISTS realized_gain_loss_percentage NUMERIC(12, 4) NOT NULL DEFAULT 0;

ALTER TABLE positions
DROP CONSTRAINT IF EXISTS positions_status_valid;

ALTER TABLE positions
ADD CONSTRAINT positions_status_valid CHECK (status IN ('open', 'closed'));

CREATE INDEX IF NOT EXISTS idx_positions_user_status
ON positions(user_id, status);

CREATE TABLE IF NOT EXISTS portfolio_archive_snapshots (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    portfolio_id UUID NOT NULL REFERENCES portfolios(id) ON DELETE CASCADE,
    captured_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    base_currency TEXT NOT NULL DEFAULT 'USD',
    portfolio_index NUMERIC(12, 4) NOT NULL,
    gain_loss_percentage NUMERIC(12, 4) NOT NULL,
    total_cost_basis NUMERIC(24, 8) NOT NULL,
    current_value NUMERIC(24, 8) NOT NULL,
    unrealized_gain_loss_base NUMERIC(24, 8) NOT NULL,
    realized_gain_loss_base NUMERIC(24, 8) NOT NULL,
    positions_snapshot JSONB NOT NULL DEFAULT '[]'::jsonb,
    closed_positions_snapshot JSONB NOT NULL DEFAULT '[]'::jsonb
);

CREATE INDEX IF NOT EXISTS idx_portfolio_archives_user_time
ON portfolio_archive_snapshots(user_id, captured_at DESC);
