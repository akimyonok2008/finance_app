-- Periodic portfolio-index snapshots powering trailing-window leaderboard
-- timeframes (1W/1M/3M/6M/1Y). The background worker records one row per user
-- per tick; ranking for a window compares the current index to the most recent
-- snapshot at/before (now - window).
--
-- Privacy: stores only the percentage index (100 = baseline), never money,
-- holdings, or quantities.

CREATE TABLE IF NOT EXISTS leaderboard_snapshots (
    id              BIGSERIAL PRIMARY KEY,
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    portfolio_index DOUBLE PRECISION NOT NULL,
    captured_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Lookup pattern: latest snapshot for a user at/before a cutoff.
CREATE INDEX IF NOT EXISTS leaderboard_snapshots_user_time_idx
    ON leaderboard_snapshots (user_id, captured_at DESC);
