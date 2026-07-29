-- Bind each idempotency key to the exact normalized mutation request it first
-- committed. Nullable preserves old audit rows; all new coordinator writes set
-- the SHA-256 fingerprint.
ALTER TABLE portfolio_mutation_audit
    ADD COLUMN IF NOT EXISTS request_fingerprint TEXT;

