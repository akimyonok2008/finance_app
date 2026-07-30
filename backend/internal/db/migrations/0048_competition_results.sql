-- Arena competition engine, phase 6: immutable final results.
--
-- Written exactly once per edition, atomically with the finalizing ->
-- completed lifecycle transition (see Service.FinalizeCompetitions /
-- FinalizeResults). After that write, nothing ever updates or deletes a row
-- here: a completed competition's leaderboard must always read from this
-- table, never repriced with current market data.

CREATE TABLE competition_results (
    id                       UUID PRIMARY KEY,
    competition_id           TEXT NOT NULL REFERENCES competitions(id) ON DELETE CASCADE,
    entry_id                 UUID NOT NULL REFERENCES competition_entries(id) ON DELETE CASCADE,
    user_id                  UUID NOT NULL,
    final_rank               INTEGER NOT NULL,
    final_index              NUMERIC(24, 8) NOT NULL,
    final_return_percentage  NUMERIC(12, 4) NOT NULL,
    finalized_at             TIMESTAMPTZ NOT NULL,
    result_evidence_json     JSONB,
    created_at               TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (competition_id, entry_id)
);

-- Completed-competition leaderboard reads: ORDER BY final_rank, scoped to one
-- competition — a plain indexed range scan, never a live repricing pass.
CREATE INDEX competition_results_leaderboard_idx
    ON competition_results (competition_id, final_rank);
