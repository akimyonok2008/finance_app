-- Persistent, chain-linked ranked-performance state — the single trusted source
-- of global ranked performance. Portfolio mutations checkpoint into this table so
-- that adding capital, resizing, deleting, or replacing positions can never
-- rewrite past ranked performance; only market/FX movement while active does.
--
-- Privacy: checkpoint_index and segment_start_value_base are PRIVATE. They are
-- never exposed through any public API — only the derived ranked index and
-- return percentage are. One row per portfolio.

CREATE TABLE IF NOT EXISTS ranked_performance_state (
    portfolio_id             UUID PRIMARY KEY REFERENCES portfolios(id) ON DELETE CASCADE,
    user_id                  UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    -- Ranked index at the start of the current segment; equals the live ranked
    -- index immediately after every mutation. Must be finite and > 0.
    checkpoint_index         NUMERIC(20, 8) NOT NULL CHECK (checkpoint_index > 0),
    -- Base-currency portfolio value at the segment start. Positive when active,
    -- NULL when paused (empty portfolio).
    segment_start_value_base NUMERIC(24, 8),
    status                   TEXT NOT NULL CHECK (status IN ('active', 'paused')),
    tracking_started_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    segment_started_at       TIMESTAMPTZ,
    updated_at               TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    version                  BIGINT NOT NULL DEFAULT 1,
    -- An active state must have a positive segment start; a paused state must not.
    CONSTRAINT ranked_state_segment_consistency CHECK (
        (status = 'active' AND segment_start_value_base IS NOT NULL AND segment_start_value_base > 0)
        OR (status = 'paused' AND segment_start_value_base IS NULL)
    )
);

CREATE INDEX IF NOT EXISTS ranked_performance_state_user_idx
    ON ranked_performance_state (user_id);
