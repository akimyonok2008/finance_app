-- Generation-based ranking projection. Fixes two related freshness bugs in
-- the original leaderboard_rankings design (migration 0035):
--
--  1. Freshness was judged by MIN(computed_at) across all rows for a
--     timeframe. A single user whose valuation repeatedly failed left a row
--     that was never updated or removed, permanently pinning the whole
--     timeframe as "stale" and forcing every read onto the O(N) live path.
--  2. The board trusted the projection whenever it had a recent-enough row,
--     without checking that every currently-eligible user was represented.
--     A newly ranked user not yet reached by a refresh batch was silently
--     omitted from the public board.
--
-- The fix: rows are tagged with a generation. RefreshCache always writes
-- into the "building" generation. Only once a full pass over every eligible
-- user completes (see Service.nextBatch's cycle tracking) does the building
-- generation get promoted to "active" — so readers only ever see a
-- generation that was fully attempted, never a partial one, and a lone
-- failing user simply drops out of the next generation instead of freezing
-- the whole projection.

ALTER TABLE leaderboard_rankings DROP CONSTRAINT leaderboard_rankings_pkey;
ALTER TABLE leaderboard_rankings ADD COLUMN generation BIGINT NOT NULL DEFAULT 0;
ALTER TABLE leaderboard_rankings ADD PRIMARY KEY (timeframe, generation, user_id);

DROP INDEX IF EXISTS leaderboard_rankings_rank_idx;
CREATE INDEX leaderboard_rankings_rank_idx
    ON leaderboard_rankings (timeframe, generation, ranked_return_percentage DESC);

-- Singleton row (id is always TRUE) tracking, per the whole projection, which
-- generation is currently being built and which one is active for reads.
-- Existing rows from before this migration carry generation = 0, which
-- starts out active, so they keep serving reads without downtime until the
-- first post-migration refresh cycle completes and promotes generation 1.
-- activated_at is left NULL for that pre-existing generation 0 data: nothing
-- verified its coverage, so rankingFresh treats it as not-yet-trustworthy
-- and falls back to live computation for one cycle, exactly like a cold
-- start with an empty table.
CREATE TABLE leaderboard_ranking_state (
    id                  BOOLEAN PRIMARY KEY DEFAULT TRUE CHECK (id),
    active_generation   BIGINT NOT NULL DEFAULT 0,
    building_generation BIGINT NOT NULL DEFAULT 1,
    cycle_started_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    activated_at        TIMESTAMPTZ
);

INSERT INTO leaderboard_ranking_state (id) VALUES (TRUE);
