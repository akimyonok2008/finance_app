-- Durable, independently retryable projection of competition finalization.
CREATE TABLE competition_outbox (
    id                 UUID PRIMARY KEY,
    event_type         TEXT NOT NULL CHECK (event_type = 'competition.finalized'),
    competition_id     TEXT NOT NULL REFERENCES competitions(id) ON DELETE CASCADE,
    participant_ids    JSONB NOT NULL DEFAULT '[]'::jsonb,
    attempt_count      INTEGER NOT NULL DEFAULT 0,
    claimed_at         TIMESTAMPTZ,
    next_attempt_at    TIMESTAMPTZ,
    processed_at       TIMESTAMPTZ,
    last_error         TEXT,
    dead_lettered_at   TIMESTAMPTZ,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (event_type, competition_id)
);

CREATE INDEX competition_outbox_claim_idx
    ON competition_outbox (next_attempt_at, created_at)
    WHERE processed_at IS NULL AND dead_lettered_at IS NULL;
CREATE INDEX competition_outbox_dead_letter_idx
    ON competition_outbox (dead_lettered_at)
    WHERE dead_lettered_at IS NOT NULL;
