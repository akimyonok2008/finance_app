-- Automatic corporate-action pipeline: normalized provider events and their
-- per-portfolio application records. Forward-only and idempotent.
--
-- Corporate actions are applied automatically by the background worker; users
-- never enter them. Uniqueness guarantees an event is applied at most once per
-- portfolio, even across multiple worker instances.

CREATE TABLE IF NOT EXISTS corporate_actions (
    id                 TEXT PRIMARY KEY,            -- provider:provider_event_id
    provider           TEXT NOT NULL,
    provider_event_id  TEXT NOT NULL,
    event_type         TEXT NOT NULL,
    source_symbol      TEXT NOT NULL,
    target_symbol      TEXT,
    effective_at       TIMESTAMPTZ NOT NULL,
    status             TEXT NOT NULL,
    quality            TEXT NOT NULL,
    normalized_payload JSONB NOT NULL DEFAULT '{}'::jsonb,
    source_url         TEXT,
    raw_fingerprint    TEXT NOT NULL,
    retrieved_at       TIMESTAMPTZ NOT NULL,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT corporate_actions_provider_event_uk UNIQUE (provider, provider_event_id)
);

CREATE INDEX IF NOT EXISTS corporate_actions_status_idx
    ON corporate_actions (status, effective_at);
CREATE INDEX IF NOT EXISTS corporate_actions_source_idx
    ON corporate_actions (source_symbol);

CREATE TABLE IF NOT EXISTS corporate_action_applications (
    corporate_action_id        TEXT NOT NULL REFERENCES corporate_actions(id) ON DELETE CASCADE,
    portfolio_id               UUID NOT NULL REFERENCES portfolios(id) ON DELETE CASCADE,
    user_id                    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    status                     TEXT NOT NULL,
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
    -- The same event is applied at most ONCE per portfolio.
    PRIMARY KEY (corporate_action_id, portfolio_id)
);

CREATE INDEX IF NOT EXISTS corporate_action_applications_user_idx
    ON corporate_action_applications (user_id, created_at DESC);
CREATE INDEX IF NOT EXISTS corporate_action_applications_status_idx
    ON corporate_action_applications (status, next_retry_at);
