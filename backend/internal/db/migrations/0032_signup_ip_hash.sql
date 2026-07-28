-- Multi-account ("survivorship gaming") signal: a keyed hash of the IP a
-- password account was registered from. Never the raw IP, and never exposed
-- via the API — it exists only so accounts created from the same network can
-- later be correlated, since the public leaderboard has no other identity
-- check today. Registration itself is additionally rate-limited per IP (see
-- internal/server/router.go); this column is the durable half of that
-- defense, for investigating accounts that already slipped through.

ALTER TABLE users ADD COLUMN IF NOT EXISTS signup_ip_hash TEXT NULL;

CREATE INDEX IF NOT EXISTS users_signup_ip_hash_idx
    ON users (signup_ip_hash)
    WHERE signup_ip_hash IS NOT NULL;
