-- Bind each engine join's Idempotency-Key to the fingerprint of the request
-- it admitted, so a retried join replays the stored entry instead of relying
-- solely on the (competition_id, user_id) uniqueness backstop, and a reused
-- key against a different join is rejected rather than silently ignored.
ALTER TABLE competition_entries
    ADD COLUMN IF NOT EXISTS idempotency_key TEXT,
    ADD COLUMN IF NOT EXISTS request_fingerprint TEXT;

CREATE UNIQUE INDEX IF NOT EXISTS competition_entries_user_idempotency_uidx
    ON competition_entries (user_id, idempotency_key)
    WHERE idempotency_key IS NOT NULL;
