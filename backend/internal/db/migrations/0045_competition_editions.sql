-- Arena competition engine, step 2 of 3: generalize the existing
-- `competitions` table into competition EDITIONS without breaking the
-- currently running weekly sprint.
--
-- An edition is one scheduled occurrence of a definition (0044). It carries
-- an IMMUTABLE rules snapshot (rules_snapshot_json) copied from the exact
-- definition version at creation time, so later definition changes cannot
-- rewrite a published/active/completed edition, and a stored lifecycle
-- instead of the read-time-derived status the legacy sprint used.
--
-- Additive and safe against live data: every new column is nullable or
-- defaulted. Pre-existing weekly-sprint rows keep working untouched under the
-- sentinel lifecycle 'legacy' (status still derived at read time by the
-- legacy code path); rows the legacy EnsureCurrentSprint keeps creating until
-- the engine cutover also land on that default. The new engine only operates
-- on rows whose lifecycle_status is a real lifecycle state.

ALTER TABLE competitions
    ADD COLUMN IF NOT EXISTS definition_id       UUID REFERENCES competition_definitions(id),
    ADD COLUMN IF NOT EXISTS definition_version  BIGINT,
    ADD COLUMN IF NOT EXISTS join_opens_at       TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS join_closes_at      TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS lifecycle_status    TEXT NOT NULL DEFAULT 'legacy',
    ADD COLUMN IF NOT EXISTS rules_snapshot_json JSONB,
    ADD COLUMN IF NOT EXISTS scoring_scope       TEXT,
    ADD COLUMN IF NOT EXISTS published_at        TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS finalized_at        TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS cancelled_at        TIMESTAMPTZ;

ALTER TABLE competitions
    ADD CONSTRAINT competitions_lifecycle_status_valid CHECK (lifecycle_status IN (
        'legacy', 'draft', 'published', 'registration_open', 'registration_closed',
        'active', 'finalizing', 'completed', 'cancelled'
    ));

-- Arena catalogue reads filter by lifecycle and order by schedule.
CREATE INDEX competitions_lifecycle_starts_idx ON competitions (lifecycle_status, starts_at);
CREATE INDEX competitions_definition_idx ON competitions (definition_id) WHERE definition_id IS NOT NULL;

-- Seed the legacy "Weekly Open Sprint" definition with a FIXED id so code and
-- later migrations can reference it deterministically, and stamp the
-- pre-existing sprint rows onto it. Its v1 rules snapshot documents the exact
-- semantics those historical rows actually ran under: any non-empty portfolio
-- could join (even mid-competition, with a join-time baseline — the behavior
-- the generalized engine deliberately abandons), and scoring repriced the
-- full frozen position snapshot with NO cash component (cash was never
-- captured by the legacy join).
INSERT INTO competition_definitions (id, slug, name, description, category, icon_key, current_version)
VALUES (
    'c0000000-0000-4000-8000-000000000001',
    'weekly-open-sprint-legacy',
    'Weekly Open Sprint',
    'Open weekly sprint: any non-empty portfolio competes on full-portfolio return.',
    'open',
    'sprint',
    1
);

INSERT INTO competition_definition_versions
    (definition_id, version, eligibility_rules_json, scoring_rules_json, schedule_defaults_json, created_by)
VALUES (
    'c0000000-0000-4000-8000-000000000001',
    1,
    '{"schema_version":1,"all":[{"code":"non_empty_portfolio","label":"Portfolio has at least one position","metric":"position_count","operator":"gte","value":"1"}]}',
    '{"schema_version":1,"scope":"full_portfolio","include_cash":false,"legacy_join_time_baseline":true}',
    '{"schema_version":1,"cadence":"weekly_iso","duration_days":7}',
    'migration_0045'
);

UPDATE competitions
SET definition_id       = 'c0000000-0000-4000-8000-000000000001',
    definition_version  = 1,
    scoring_scope       = 'full_portfolio',
    rules_snapshot_json = (SELECT jsonb_build_object(
        'schema_version', 1,
        'eligibility', v.eligibility_rules_json,
        'scoring', v.scoring_rules_json
    ) FROM competition_definition_versions v
      WHERE v.definition_id = 'c0000000-0000-4000-8000-000000000001' AND v.version = 1)
WHERE type = 'weekly_sprint' AND definition_id IS NULL;
