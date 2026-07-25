-- Canonical, epoch-safe ranked-performance history shared by timeframe
-- leaderboards and benchmark achievements. No absolute portfolio values,
-- quantities, cost basis, or segment baselines are stored.

CREATE TABLE IF NOT EXISTS ranked_performance_snapshots (
    id                  UUID PRIMARY KEY,
    portfolio_id        UUID NOT NULL REFERENCES portfolios(id) ON DELETE CASCADE,
    user_id             UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    tracking_started_at TIMESTAMPTZ NOT NULL,
    ranked_index        NUMERIC(30, 12) NOT NULL CHECK (ranked_index > 0),
    ranking_status      TEXT NOT NULL CHECK (ranking_status IN ('active', 'paused')),
    captured_at         TIMESTAMPTZ NOT NULL,
    snapshot_kind       TEXT NOT NULL CHECK (snapshot_kind IN ('intraday', 'daily', 'transition')),
    bucket_start        TIMESTAMPTZ,
    snapshot_date       DATE,
    valuation_as_of     TIMESTAMPTZ NOT NULL,
    data_quality_status TEXT NOT NULL CHECK (data_quality_status IN ('complete', 'stale', 'partial', 'invalid')),
    evidence_protected  BOOLEAN NOT NULL DEFAULT FALSE,
    evaluation_status   TEXT NOT NULL DEFAULT 'done'
        CHECK (evaluation_status IN ('pending', 'processing', 'done')),
    evaluation_claimed_at TIMESTAMPTZ,
    evaluation_attempts INTEGER NOT NULL DEFAULT 0,
    evaluation_last_error TEXT,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT ranked_snapshot_kind_fields CHECK (
        (snapshot_kind = 'intraday' AND bucket_start IS NOT NULL AND snapshot_date IS NULL)
        OR (snapshot_kind = 'daily' AND snapshot_date IS NOT NULL AND bucket_start IS NULL)
        OR (snapshot_kind = 'transition' AND bucket_start IS NULL AND snapshot_date IS NULL)
    )
);

CREATE UNIQUE INDEX IF NOT EXISTS ranked_snapshots_intraday_bucket_uidx
    ON ranked_performance_snapshots (portfolio_id, tracking_started_at, bucket_start)
    WHERE snapshot_kind = 'intraday';

CREATE UNIQUE INDEX IF NOT EXISTS ranked_snapshots_daily_uidx
    ON ranked_performance_snapshots (portfolio_id, tracking_started_at, snapshot_date)
    WHERE snapshot_kind = 'daily';

CREATE UNIQUE INDEX IF NOT EXISTS ranked_snapshots_transition_uidx
    ON ranked_performance_snapshots
       (portfolio_id, tracking_started_at, snapshot_kind, captured_at)
    WHERE snapshot_kind = 'transition';

CREATE INDEX IF NOT EXISTS ranked_snapshots_user_window_idx
    ON ranked_performance_snapshots
       (user_id, tracking_started_at, captured_at DESC);

CREATE INDEX IF NOT EXISTS ranked_snapshots_portfolio_epoch_idx
    ON ranked_performance_snapshots
       (portfolio_id, tracking_started_at, captured_at DESC);

CREATE INDEX IF NOT EXISTS ranked_snapshots_evaluation_idx
    ON ranked_performance_snapshots
       (evaluation_status, evaluation_claimed_at, captured_at)
    WHERE evaluation_status <> 'done';

CREATE INDEX IF NOT EXISTS ranked_snapshots_compaction_idx
    ON ranked_performance_snapshots (captured_at, portfolio_id, tracking_started_at)
    WHERE snapshot_kind = 'intraday' AND evidence_protected = FALSE;

-- Existing awards remain permanent but are explicitly labelled with their
-- original archive-derived evidence model. They are never rewritten as verified
-- ranked-snapshot awards.
UPDATE user_benchmark_achievements
SET evidence = evidence || jsonb_build_object(
    'evaluation_model', 'archive_model_v0',
    'evidence_version', 0
)
WHERE NOT (evidence ? 'evaluation_model');
