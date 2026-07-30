-- Arena competition engine, step 1 of 3: reusable competition DEFINITIONS and
-- their immutable VERSIONS.
--
-- A definition is a template ("Crypto Challenge", "ETF Battle"); a version is
-- one immutable rule set for that template. Editions (scheduled occurrences,
-- migration 0045) snapshot the exact version they run under, so changing a
-- definition later can never alter a published, active, or completed edition.
--
-- Forward-only and purely additive: no existing table is touched.

CREATE TABLE competition_definitions (
    id                       UUID PRIMARY KEY,
    slug                     TEXT NOT NULL,
    name                     TEXT NOT NULL,
    description              TEXT NOT NULL DEFAULT '',
    category                 TEXT NOT NULL DEFAULT '',
    icon_key                 TEXT NOT NULL DEFAULT '',
    presentation_config_json JSONB,
    is_enabled               BOOLEAN NOT NULL DEFAULT TRUE,
    -- current_version is a pointer to the latest created version, maintained
    -- transactionally by CreateDefinitionVersion. 0 means "no version yet";
    -- such a definition cannot back an edition.
    current_version          BIGINT NOT NULL DEFAULT 0,
    created_at               TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at               TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX competition_definitions_slug_key ON competition_definitions (slug);

-- Versions are IMMUTABLE after creation: the repository exposes no UPDATE for
-- this table, and the composite primary key makes re-INSERTing an existing
-- version a hard conflict rather than a silent overwrite. Rule payloads are
-- stored as validated JSON documents (schema versioned inside the payload,
-- see the competitions/rules package); numeric thresholds inside them are
-- decimal STRINGS ("0.30"), never floats.
CREATE TABLE competition_definition_versions (
    definition_id          UUID NOT NULL REFERENCES competition_definitions(id) ON DELETE CASCADE,
    version                BIGINT NOT NULL CHECK (version >= 1),
    eligibility_rules_json JSONB NOT NULL,
    scoring_rules_json     JSONB NOT NULL,
    schedule_defaults_json JSONB,
    display_rules_json     JSONB,
    created_at             TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_by             TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (definition_id, version)
);
