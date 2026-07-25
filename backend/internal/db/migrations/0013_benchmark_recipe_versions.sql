-- Benchmark data-integrity: durable, immutable recipe versions.
--
-- Every benchmark recipe used to award a permanent achievement must be
-- reproducible from an explicit, dated version. Static recipes are code-defined
-- (internal/benchmark.DefaultVersionStore assigns them <ID>_v1); dynamic 13F
-- recipes get one immutable version per authoritative SEC filing. This table is
-- the durable audit record of those versions — especially the externally
-- sourced Berkshire baskets, whose SEC accession, reporting period and mapping
-- coverage are recorded so every award is traceable to a filing.
--
-- No-look-ahead: a historical evaluation selects the newest version whose
-- publicly_known_at <= the evaluation start, so holdings are never used before
-- they were publicly filed.

CREATE TABLE IF NOT EXISTS benchmark_recipe_versions (
    id                        BIGSERIAL PRIMARY KEY,
    recipe_id                 TEXT NOT NULL,
    version_id                TEXT NOT NULL,
    name                      TEXT NOT NULL DEFAULT '',
    description               TEXT NOT NULL DEFAULT '',
    report_period_end         DATE,
    filed_at                  DATE,
    publicly_known_at         TIMESTAMPTZ NOT NULL,
    effective_from            TIMESTAMPTZ NOT NULL,
    effective_until           TIMESTAMPTZ,
    components_json           JSONB NOT NULL,
    source_type               TEXT NOT NULL,           -- static_model | sec_13f_hr
    source_url                TEXT NOT NULL DEFAULT '',
    source_accession          TEXT NOT NULL DEFAULT '',
    source_description        TEXT NOT NULL DEFAULT '',
    rebalancing_policy        TEXT NOT NULL DEFAULT 'daily_target_weight',
    total_return_model        BOOLEAN NOT NULL DEFAULT TRUE,
    investor_proxy            BOOLEAN NOT NULL DEFAULT FALSE,
    source_market_value_total NUMERIC,
    mapped_market_value_total NUMERIC,
    mapping_coverage_pct      DOUBLE PRECISION,
    excluded_positions_count  INTEGER NOT NULL DEFAULT 0,
    created_at                TIMESTAMPTZ NOT NULL DEFAULT now(),

    -- One row per (recipe, version); no duplicate version ids.
    CONSTRAINT uq_recipe_version UNIQUE (recipe_id, version_id),
    -- Effective range must be valid.
    CONSTRAINT ck_effective_range CHECK (effective_until IS NULL OR effective_until > effective_from),
    -- Externally sourced (13F) versions require full source metadata and a
    -- coverage figure at or above the documented threshold (0.90).
    CONSTRAINT ck_sec_source_metadata CHECK (
        source_type <> 'sec_13f_hr' OR (
            source_accession <> '' AND source_url <> '' AND report_period_end IS NOT NULL
            AND mapping_coverage_pct IS NOT NULL AND mapping_coverage_pct >= 0.90
        )
    )
);

CREATE INDEX IF NOT EXISTS idx_recipe_versions_recipe
    ON benchmark_recipe_versions (recipe_id, publicly_known_at DESC);

-- Seed the authoritative Berkshire Hathaway 13F versions (CIK 0001067983).
-- Weights are market-value weights among reliably-mapped large-cap positions,
-- renormalized to 1. See internal/benchmark/recipe_version.go for the source.
INSERT INTO benchmark_recipe_versions
    (recipe_id, version_id, name, description, report_period_end, filed_at,
     publicly_known_at, effective_from, effective_until, components_json,
     source_type, source_url, source_accession, source_description,
     rebalancing_policy, total_return_model, investor_proxy,
     source_market_value_total, mapped_market_value_total, mapping_coverage_pct,
     excluded_positions_count)
VALUES
    ('BUFFETT_13F', 'BUFFETT_13F_2025Q1',
     'Berkshire 13F Equity Basket (2025 Q1)',
     'Berkshire Hathaway disclosed public-equity basket, 2025 Q1 13F-HR, mapped large-caps, market-value weighted.',
     DATE '2025-03-31', DATE '2025-05-15',
     TIMESTAMPTZ '2025-05-15 00:00:00+00', TIMESTAMPTZ '2025-05-15 00:00:00+00', TIMESTAMPTZ '2026-05-15 00:00:00+00',
     '[]'::jsonb,
     'sec_13f_hr', 'https://www.sec.gov/Archives/edgar/data/1067983/000095012325005701/form13fInfoTable.xml',
     '0000950123-25-005701', 'Berkshire Hathaway Inc. 13F-HR for period 2025-03-31.',
     'daily_target_weight', TRUE, TRUE,
     258701144516, 239749268777, 0.9267, 23),
    ('BUFFETT_13F', 'BUFFETT_13F_2026Q1',
     'Berkshire 13F Equity Basket (2026 Q1)',
     'Berkshire Hathaway disclosed public-equity basket, 2026 Q1 13F-HR, mapped large-caps, market-value weighted.',
     DATE '2026-03-31', DATE '2026-05-15',
     TIMESTAMPTZ '2026-05-15 00:00:00+00', TIMESTAMPTZ '2026-05-15 00:00:00+00', NULL,
     '[]'::jsonb,
     'sec_13f_hr', 'https://www.sec.gov/Archives/edgar/data/1067983/000119312526226661/53405.xml',
     '0001193125-26-226661', 'Berkshire Hathaway Inc. 13F-HR for period 2026-03-31.',
     'daily_target_weight', TRUE, TRUE,
     263095703570, 254692792933, 0.9681, 14)
ON CONFLICT (recipe_id, version_id) DO NOTHING;

-- Evidence immutability: once an award is written, its evidence and unlock time
-- must never be silently rewritten by later provider/recipe/config changes. New
-- logic must use a new evidence_version, not edit history. This trigger blocks
-- in-place mutation of the evidence and unlocked_at columns (corrections must go
-- through an explicit revocation/re-award flow, i.e. delete + re-insert).
CREATE OR REPLACE FUNCTION forbid_benchmark_evidence_rewrite()
RETURNS TRIGGER AS $$
BEGIN
    IF (NEW.evidence IS DISTINCT FROM OLD.evidence)
       OR (NEW.unlocked_at IS DISTINCT FROM OLD.unlocked_at) THEN
        RAISE EXCEPTION 'benchmark achievement evidence is immutable (badge_key=%, user_id=%)',
            OLD.badge_key, OLD.user_id;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_forbid_benchmark_evidence_rewrite ON user_benchmark_achievements;
CREATE TRIGGER trg_forbid_benchmark_evidence_rewrite
    BEFORE UPDATE ON user_benchmark_achievements
    FOR EACH ROW EXECUTE FUNCTION forbid_benchmark_evidence_rewrite();
