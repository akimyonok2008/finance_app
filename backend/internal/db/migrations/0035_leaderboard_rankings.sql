-- Denormalized ranking projection for the leaderboard. Populated by the
-- periodic RefreshCache job (which already pays for one portfolio valuation
-- per user); read by PostgresRankingStore so board/rank/standing reads become
-- a single Postgres query with rank computed by a window function, instead of
-- enumerating every user and sorting in application memory.
--
-- Only rankable rows are stored: a paused or timeframe-ineligible user has no
-- row for that timeframe rather than a flagged one, mirroring the existing
-- "paused users are excluded from ranking" semantics.

CREATE TABLE IF NOT EXISTS leaderboard_rankings (
    user_id                  UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    timeframe                TEXT NOT NULL CHECK (timeframe IN ('ALL', '1W', '1M', '3M', '6M', '1Y')),
    ranked_index             NUMERIC(30, 12) NOT NULL CHECK (ranked_index > 0),
    ranked_return_percentage NUMERIC(30, 12) NOT NULL,
    tracking_started_at      TIMESTAMPTZ NOT NULL,
    computed_at              TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (timeframe, user_id)
);

CREATE INDEX IF NOT EXISTS leaderboard_rankings_rank_idx
    ON leaderboard_rankings (timeframe, ranked_return_percentage DESC);
