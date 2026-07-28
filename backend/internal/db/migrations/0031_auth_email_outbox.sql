-- Durable authentication-email delivery.
--
-- Registration writes the user, verification token, and this delivery intent
-- in one transaction. SMTP is deliberately outside that transaction: workers
-- claim committed rows with SKIP LOCKED and retry failures without forcing the
-- client to repeat registration or leaving an account without a token.

CREATE TABLE IF NOT EXISTS auth_email_outbox (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    kind TEXT NOT NULL CHECK (kind IN ('email_verification')),
    recipient TEXT NOT NULL,
    delivery_url TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    available_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    claimed_at TIMESTAMPTZ NULL,
    delivered_at TIMESTAMPTZ NULL,
    attempts INTEGER NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    last_error TEXT NULL
);

CREATE INDEX IF NOT EXISTS auth_email_outbox_pending_idx
    ON auth_email_outbox (available_at, created_at)
    WHERE delivered_at IS NULL;
