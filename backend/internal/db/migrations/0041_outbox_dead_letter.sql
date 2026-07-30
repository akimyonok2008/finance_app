-- A single unpriceable symbol (delisted ticker, provider coverage gap) retried
-- portfolio_outbox forever: no attempt cap, no backoff, no way to stop it
-- poisoning the readiness backlog age check permanently. next_attempt_at adds
-- exponential backoff between retries, and dead_lettered_at gives failed events
-- a terminal state so they stop counting toward backlog age once attempts are
-- exhausted.
ALTER TABLE portfolio_outbox
    ADD COLUMN IF NOT EXISTS next_attempt_at   TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS dead_lettered_at  TIMESTAMPTZ;

CREATE INDEX IF NOT EXISTS idx_portfolio_outbox_dead_letter
    ON portfolio_outbox (dead_lettered_at)
    WHERE dead_lettered_at IS NOT NULL;
