-- Authentication lifecycle hardening: verified email, revocable JWT versions,
-- explicit password ownership, and hashed single-use lifecycle tokens.

ALTER TABLE users
    ALTER COLUMN password_hash DROP NOT NULL,
    ADD COLUMN IF NOT EXISTS email_verified_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS auth_version INTEGER NOT NULL DEFAULT 1,
    ADD COLUMN IF NOT EXISTS has_password BOOLEAN NOT NULL DEFAULT TRUE;

ALTER TABLE users
    DROP CONSTRAINT IF EXISTS users_auth_version_positive,
    ADD CONSTRAINT users_auth_version_positive CHECK (auth_version > 0),
    DROP CONSTRAINT IF EXISTS users_password_state_valid,
    ADD CONSTRAINT users_password_state_valid
        CHECK ((has_password AND password_hash IS NOT NULL) OR
               (NOT has_password AND password_hash IS NULL));

-- Existing password accounts predate verification and remain usable. Provider
-- accounts created by the old implementation received a generated password.
-- The near-simultaneous identity heuristic distinguishes those accounts from
-- password accounts that linked a provider later.
UPDATE users
SET email_verified_at = COALESCE(email_verified_at, created_at)
WHERE email_verified_at IS NULL;

UPDATE users u
SET has_password = FALSE, password_hash = NULL
WHERE EXISTS (
    SELECT 1
    FROM auth_identities ai
    WHERE ai.user_id = u.id
      AND ai.email_verified = TRUE
      AND ai.created_at <= u.created_at + INTERVAL '1 minute'
);

CREATE TABLE IF NOT EXISTS email_verification_tokens (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash TEXT NOT NULL UNIQUE,
    expires_at TIMESTAMPTZ NOT NULL,
    consumed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT email_verification_token_hash_valid CHECK (char_length(token_hash) = 64)
);

CREATE INDEX IF NOT EXISTS email_verification_tokens_user_idx
    ON email_verification_tokens (user_id, created_at DESC);
CREATE INDEX IF NOT EXISTS email_verification_tokens_expiry_idx
    ON email_verification_tokens (expires_at)
    WHERE consumed_at IS NULL;

CREATE TABLE IF NOT EXISTS password_reset_tokens (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash TEXT NOT NULL UNIQUE,
    expires_at TIMESTAMPTZ NOT NULL,
    consumed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT password_reset_token_hash_valid CHECK (char_length(token_hash) = 64)
);

CREATE INDEX IF NOT EXISTS password_reset_tokens_user_idx
    ON password_reset_tokens (user_id, created_at DESC);
CREATE INDEX IF NOT EXISTS password_reset_tokens_expiry_idx
    ON password_reset_tokens (expires_at)
    WHERE consumed_at IS NULL;

CREATE TABLE IF NOT EXISTS reauthentication_tokens (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash TEXT NOT NULL UNIQUE,
    expires_at TIMESTAMPTZ NOT NULL,
    consumed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT reauthentication_token_hash_valid CHECK (char_length(token_hash) = 64)
);

CREATE INDEX IF NOT EXISTS reauthentication_tokens_user_idx
    ON reauthentication_tokens (user_id, created_at DESC);
CREATE INDEX IF NOT EXISTS reauthentication_tokens_expiry_idx
    ON reauthentication_tokens (expires_at)
    WHERE consumed_at IS NULL;
