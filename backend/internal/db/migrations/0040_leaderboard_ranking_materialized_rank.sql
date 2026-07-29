-- Materializes the leaderboard rank instead of recomputing it on every read.
--
-- Before this migration, TopPage/RankOf/ValueAtRank each ran a RANK() window
-- function (with no PARTITION BY) over every eligible row for a timeframe's
-- active generation, joined to users for the display_name tie-break, before
-- LIMIT/filtering was applied. That is O(active population) per read, not
-- O(page size) as the RankingStore interface doc claimed.
--
-- The fix: rnk is computed once per generation, in PostgresRankingStore.
-- CompleteCycle, right before that generation is promoted to active. Reads
-- then become a plain indexed lookup/range-scan on the already-materialized
-- column — no window function, no full-population sort, at request time.

ALTER TABLE leaderboard_rankings ADD COLUMN rnk INTEGER;

CREATE INDEX leaderboard_rankings_materialized_rank_idx
    ON leaderboard_rankings (timeframe, generation, rnk);
