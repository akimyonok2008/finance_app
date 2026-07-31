-- Arena competition engine: bounded, resumable finalization.
--
-- Before this migration, FinalizeCompetitions loaded every EntryActive
-- entry for a competition in one query, valued and sorted all of them in one
-- process call, then wrote every result row in one transaction. That is
-- unbounded in a large public competition. This mirrors the SAME
-- building/promote generation pattern competition_ranking_generations
-- already uses (see migration 0047's doc comment) — but adapted for a
-- ONE-SHOT, terminal pass instead of a continuously-refreshed live board:
-- a finalization generation is either still building (bounded batches
-- claimed across one or more worker ticks, exactly like ranking refresh) or
-- completed (materialized into the immutable competition_results table and
-- never touched again). There is no "active"/"superseded" state here,
-- because an edition is only ever finalized once.
CREATE TABLE competition_finalization_generations (
    id                 TEXT PRIMARY KEY,
    competition_id     TEXT NOT NULL REFERENCES competitions(id) ON DELETE CASCADE,
    generation         BIGINT NOT NULL,
    status             TEXT NOT NULL DEFAULT 'building'
        CHECK (status IN ('building', 'completed', 'failed')),
    expected_entries   INTEGER NOT NULL DEFAULT 0,
    processed_entries  INTEGER NOT NULL DEFAULT 0,
    successful_entries INTEGER NOT NULL DEFAULT 0,
    -- failed_entries counts only entries still awaiting a retry WITHIN the
    -- finalization window (see Service.finalizeEdition) — an entry that has
    -- already been explicitly disqualified past the window is resolved and
    -- is no longer counted here, so it never blocks promotion.
    failed_entries     INTEGER NOT NULL DEFAULT 0,
    cursor_entry_id    UUID,
    write_failure      BOOLEAN NOT NULL DEFAULT FALSE,
    started_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    completed_at       TIMESTAMPTZ,
    failure_reason     TEXT,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (competition_id, generation)
);

-- The worker looks up "the" building generation for a competition every tick.
CREATE UNIQUE INDEX competition_finalization_generations_one_building_idx
    ON competition_finalization_generations (competition_id) WHERE status = 'building';

-- Draft rows for one generation's finalization pass. Writes always target a
-- 'building' generation (enforced in application code via the EXISTS guard
-- on UpsertFinalizationRow, mirroring UpsertRanking); once the generation's
-- coverage is complete (zero failed_entries, no write_failure), Promote
-- materializes sequential ranks with the SAME tie-break chain as ranking
-- generations (return_percentage DESC, display_name ASC, user_id ASC — see
-- migration 0047), copies the rows into the immutable competition_results
-- table in one atomic transaction, and prunes these draft rows.
CREATE TABLE competition_finalization_rows (
    competition_id     TEXT NOT NULL REFERENCES competitions(id) ON DELETE CASCADE,
    generation         BIGINT NOT NULL,
    entry_id           UUID NOT NULL REFERENCES competition_entries(id) ON DELETE CASCADE,
    user_id            UUID NOT NULL,
    competition_index  NUMERIC(24, 8) NOT NULL,
    return_percentage  NUMERIC(12, 4) NOT NULL,
    materialized_rank  INTEGER,
    valued_at          TIMESTAMPTZ NOT NULL,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (competition_id, generation, entry_id)
);

CREATE INDEX competition_finalization_rows_rank_idx
    ON competition_finalization_rows (competition_id, generation, materialized_rank);
