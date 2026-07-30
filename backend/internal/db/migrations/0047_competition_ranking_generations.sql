-- Arena competition engine, phase 5: ranking generations and the
-- cursor-paginated leaderboard projection they back.
--
-- Deliberately separate from the global leaderboard's tables and generation
-- machinery (leaderboard_rankings / leaderboard_ranking_state, migrations
-- 0035/0038/0040): competition rankings must never share storage, promotion,
-- or read paths with global platform-wide ranking. This mirrors the SAFE
-- parts of that architecture (a building/active generation pair, atomic
-- promotion, materialized ranks so reads are a plain indexed query) while
-- keeping the two domains fully independent.
--
-- Unlike leaderboard_ranking_state's single active/building pointer row per
-- timeframe, competition_ranking_generations keeps ONE ROW PER GENERATION
-- ATTEMPT, so expected/processed/successful/failed counts and any failure
-- reason stay attached to the specific attempt they describe (per the
-- product spec's shape) rather than being overwritten on every promotion.

CREATE TABLE competition_ranking_generations (
    id                 TEXT PRIMARY KEY,
    competition_id     TEXT NOT NULL REFERENCES competitions(id) ON DELETE CASCADE,
    generation         BIGINT NOT NULL,
    status             TEXT NOT NULL DEFAULT 'building'
        CHECK (status IN ('building', 'active', 'superseded', 'failed')),
    expected_entries   INTEGER NOT NULL DEFAULT 0,
    processed_entries  INTEGER NOT NULL DEFAULT 0,
    successful_entries INTEGER NOT NULL DEFAULT 0,
    failed_entries     INTEGER NOT NULL DEFAULT 0,
    cursor_entry_id    UUID,
    write_failure      BOOLEAN NOT NULL DEFAULT FALSE,
    started_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    completed_at       TIMESTAMPTZ,
    activated_at       TIMESTAMPTZ,
    failure_reason     TEXT,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (competition_id, generation)
);

-- At most one generation may be "active" (currently served) per competition.
-- PromoteGeneration supersedes the previous active row inside the same
-- transaction that activates the new one, so this can never be violated.
CREATE UNIQUE INDEX competition_ranking_generations_one_active_idx
    ON competition_ranking_generations (competition_id) WHERE status = 'active';

-- The worker looks up "the" building generation for a competition every tick.
CREATE UNIQUE INDEX competition_ranking_generations_one_building_idx
    ON competition_ranking_generations (competition_id) WHERE status = 'building';

-- Rows for one generation's ranking projection. Writes always target a
-- 'building' generation (enforced in application code via UpsertRanking's
-- EXISTS guard); reads always target the 'active' one. materialized_rank is
-- computed exactly once, by PromoteGeneration, using ROW_NUMBER() over
-- (return_percentage DESC, display_name ASC, user_id ASC) — a strict total
-- order since user_id is unique, so ranks are always SEQUENTIAL and never
-- shared between tied returns. This is the one canonical tie-break/rank
-- policy for competition rankings; it must not be reimplemented elsewhere.
CREATE TABLE competition_rankings (
    competition_id     TEXT NOT NULL REFERENCES competitions(id) ON DELETE CASCADE,
    generation         BIGINT NOT NULL,
    entry_id           UUID NOT NULL REFERENCES competition_entries(id) ON DELETE CASCADE,
    user_id            UUID NOT NULL,
    competition_index  NUMERIC(24, 8) NOT NULL,
    return_percentage  NUMERIC(12, 4) NOT NULL,
    materialized_rank  INTEGER,
    valued_at          TIMESTAMPTZ NOT NULL,
    is_stale           BOOLEAN NOT NULL DEFAULT FALSE,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (competition_id, generation, entry_id)
);

-- Cursor-paginated leaderboard reads: rank > $cursor ORDER BY rank LIMIT $n,
-- scoped to the active generation looked up via the partial index above.
CREATE INDEX competition_rankings_leaderboard_idx
    ON competition_rankings (competition_id, generation, materialized_rank);

-- Exact current-user rank lookup.
CREATE INDEX competition_rankings_user_idx
    ON competition_rankings (competition_id, generation, user_id);
