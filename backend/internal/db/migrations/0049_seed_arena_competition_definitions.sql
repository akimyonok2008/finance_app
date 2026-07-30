-- Arena competition engine, phase 8: seed initial competition definitions
-- and one launch edition each, entirely through data (no application-code
-- branches) — proving the engine built in migrations 0044-0048 can host new
-- competitions by configuration alone.
--
-- Technology Challenge is deliberately NOT seeded here: its eligibility rule
-- would need a "sector" filter, and no instrument-metadata pipeline in this
-- repository currently populates PositionFacts.Sector (see
-- internal/competitions/eligibility.go's factsFromSnapshot and
-- internal/competitions/rules/evaluate.go) — seeding it would silently never
-- match anyone. That is a real gap in the instrument-classification layer,
-- not a rules-engine limitation, and is tracked separately rather than
-- shipped as a broken competition.
--
-- Forward-only and purely additive: no existing table or row is touched.

-- 1. Crypto Challenge (full-portfolio variant): eligibility and scoring both
--    look at the whole portfolio.
INSERT INTO competition_definitions (id, slug, name, description, category, icon_key, current_version)
VALUES (
    'c0000000-0000-4000-8000-000000000002',
    'crypto-challenge-full-portfolio',
    'Crypto Challenge',
    'Hold at least 30% of your portfolio in crypto assets, then compete on your complete portfolio''s return.',
    'crypto',
    'crypto',
    1
);
INSERT INTO competition_definition_versions
    (definition_id, version, eligibility_rules_json, scoring_rules_json, schedule_defaults_json, created_by)
VALUES (
    'c0000000-0000-4000-8000-000000000002',
    1,
    '{"schema_version":1,"all":[{"code":"minimum_crypto_weight","label":"At least 30% crypto exposure","metric":"portfolio_weight","filter":{"asset_types":["crypto"]},"operator":"gte","value":"0.30"}]}',
    '{"schema_version":1,"scope":"full_portfolio","include_cash":true}',
    '{"schema_version":1,"join_window_days":3,"duration_days":7}',
    'migration_0049'
);

-- 2. Crypto Performance Battle: the SAME 30% entry bar as above, but scoring
--    only the matching crypto sleeve — proving eligibility and scoring are
--    independently configurable on top of one shared rule engine.
INSERT INTO competition_definitions (id, slug, name, description, category, icon_key, current_version)
VALUES (
    'c0000000-0000-4000-8000-000000000003',
    'crypto-performance-battle',
    'Crypto Performance Battle',
    'Same 30% crypto entry bar as the Crypto Challenge, but only your crypto holdings are scored.',
    'crypto',
    'crypto-battle',
    1
);
INSERT INTO competition_definition_versions
    (definition_id, version, eligibility_rules_json, scoring_rules_json, schedule_defaults_json, created_by)
VALUES (
    'c0000000-0000-4000-8000-000000000003',
    1,
    '{"schema_version":1,"all":[{"code":"minimum_crypto_weight","label":"At least 30% crypto exposure","metric":"portfolio_weight","filter":{"asset_types":["crypto"]},"operator":"gte","value":"0.30"}]}',
    '{"schema_version":1,"scope":"matching_assets","filter":{"asset_types":["crypto"]},"include_cash":false}',
    '{"schema_version":1,"join_window_days":3,"duration_days":7}',
    'migration_0049'
);

-- 3. ETF Battle: eligibility requires a heavy ETF weighting; scoring is the
--    full portfolio for this launch edition.
INSERT INTO competition_definitions (id, slug, name, description, category, icon_key, current_version)
VALUES (
    'c0000000-0000-4000-8000-000000000004',
    'etf-battle',
    'ETF Battle',
    'Hold at least 50% of your portfolio in ETFs, then compete on your complete portfolio''s return.',
    'etf',
    'etf',
    1
);
INSERT INTO competition_definition_versions
    (definition_id, version, eligibility_rules_json, scoring_rules_json, schedule_defaults_json, created_by)
VALUES (
    'c0000000-0000-4000-8000-000000000004',
    1,
    '{"schema_version":1,"all":[{"code":"minimum_etf_weight","label":"At least 50% ETF exposure","metric":"portfolio_weight","filter":{"asset_types":["etf"]},"operator":"gte","value":"0.50"}]}',
    '{"schema_version":1,"scope":"full_portfolio","include_cash":true}',
    '{"schema_version":1,"join_window_days":3,"duration_days":14}',
    'migration_0049'
);

-- 4. Türkiye Equities Challenge: eligibility and scoring both key off venue
--    MIC metadata (XIST = Borsa Istanbul), never a symbol/ticker heuristic.
INSERT INTO competition_definitions (id, slug, name, description, category, icon_key, current_version)
VALUES (
    'c0000000-0000-4000-8000-000000000005',
    'turkiye-equities-challenge',
    'Türkiye Equities Challenge',
    'Hold at least 25% of your portfolio in XIST-listed equities. Only those positions are scored.',
    'regional',
    'turkiye',
    1
);
INSERT INTO competition_definition_versions
    (definition_id, version, eligibility_rules_json, scoring_rules_json, schedule_defaults_json, created_by)
VALUES (
    'c0000000-0000-4000-8000-000000000005',
    1,
    '{"schema_version":1,"all":[{"code":"minimum_xist_weight","label":"At least 25% XIST-listed exposure","metric":"portfolio_weight","filter":{"venue_mics":["XIST"]},"operator":"gte","value":"0.25"}]}',
    '{"schema_version":1,"scope":"matching_assets","filter":{"venue_mics":["XIST"]},"include_cash":false}',
    '{"schema_version":1,"join_window_days":3,"duration_days":14}',
    'migration_0049'
);

-- One launch edition per definition, immediately open for registration so
-- Arena has real content to display. Each snapshots its version's rules into
-- rules_snapshot_json exactly like migration 0045 did for the legacy sprint.
INSERT INTO competitions
    (id, name, type, starts_at, ends_at, status, created_at,
     definition_id, definition_version, join_opens_at, join_closes_at,
     lifecycle_status, rules_snapshot_json, scoring_scope, published_at)
SELECT
    'seed_crypto_challenge_launch', 'Crypto Challenge', 'engine',
    now() + interval '3 days', now() + interval '10 days', 'registration_open', now(),
    'c0000000-0000-4000-8000-000000000002', 1, now(), now() + interval '3 days',
    'registration_open',
    jsonb_build_object('schema_version', 1, 'eligibility', v.eligibility_rules_json, 'scoring', v.scoring_rules_json),
    'full_portfolio', now()
FROM competition_definition_versions v
WHERE v.definition_id = 'c0000000-0000-4000-8000-000000000002' AND v.version = 1;

INSERT INTO competitions
    (id, name, type, starts_at, ends_at, status, created_at,
     definition_id, definition_version, join_opens_at, join_closes_at,
     lifecycle_status, rules_snapshot_json, scoring_scope, published_at)
SELECT
    'seed_crypto_battle_launch', 'Crypto Performance Battle', 'engine',
    now() + interval '3 days', now() + interval '10 days', 'registration_open', now(),
    'c0000000-0000-4000-8000-000000000003', 1, now(), now() + interval '3 days',
    'registration_open',
    jsonb_build_object('schema_version', 1, 'eligibility', v.eligibility_rules_json, 'scoring', v.scoring_rules_json),
    'matching_assets', now()
FROM competition_definition_versions v
WHERE v.definition_id = 'c0000000-0000-4000-8000-000000000003' AND v.version = 1;

INSERT INTO competitions
    (id, name, type, starts_at, ends_at, status, created_at,
     definition_id, definition_version, join_opens_at, join_closes_at,
     lifecycle_status, rules_snapshot_json, scoring_scope, published_at)
SELECT
    'seed_etf_battle_launch', 'ETF Battle', 'engine',
    now() + interval '3 days', now() + interval '17 days', 'registration_open', now(),
    'c0000000-0000-4000-8000-000000000004', 1, now(), now() + interval '3 days',
    'registration_open',
    jsonb_build_object('schema_version', 1, 'eligibility', v.eligibility_rules_json, 'scoring', v.scoring_rules_json),
    'full_portfolio', now()
FROM competition_definition_versions v
WHERE v.definition_id = 'c0000000-0000-4000-8000-000000000004' AND v.version = 1;

INSERT INTO competitions
    (id, name, type, starts_at, ends_at, status, created_at,
     definition_id, definition_version, join_opens_at, join_closes_at,
     lifecycle_status, rules_snapshot_json, scoring_scope, published_at)
SELECT
    'seed_turkiye_equities_launch', 'Türkiye Equities Challenge', 'engine',
    now() + interval '3 days', now() + interval '17 days', 'registration_open', now(),
    'c0000000-0000-4000-8000-000000000005', 1, now(), now() + interval '3 days',
    'registration_open',
    jsonb_build_object('schema_version', 1, 'eligibility', v.eligibility_rules_json, 'scoring', v.scoring_rules_json),
    'matching_assets', now()
FROM competition_definition_versions v
WHERE v.definition_id = 'c0000000-0000-4000-8000-000000000005' AND v.version = 1;
