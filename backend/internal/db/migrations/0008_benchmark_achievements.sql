-- Prototype 4 benchmark-beating achievement badges.
--
-- Badge definitions are code-defined (see internal/benchmark.Badges), so this
-- migration stores only per-user unlocks and their privacy-safe evidence.
-- Evidence holds percentages, dates, and the benchmark recipe id only — never
-- monetary values, holdings, or identifiers.
--
-- The legacy milestone tables (achievements, user_achievements) are retired and
-- left in place, unused, to avoid a destructive drop.

CREATE TABLE IF NOT EXISTS user_benchmark_achievements (
    user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    badge_key   TEXT NOT NULL,
    unlocked_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    evidence    JSONB NOT NULL DEFAULT '{}'::jsonb,
    PRIMARY KEY (user_id, badge_key)
);

CREATE INDEX IF NOT EXISTS idx_user_benchmark_achievements_user
    ON user_benchmark_achievements (user_id);
