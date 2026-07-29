-- First-class venue and issuer entities, plus the reconciliation-queue table
-- and identity columns that the corporate-action and outbox pipelines need to
-- carry stable instrument identity through, instead of symbol strings.
--
-- Forward-only, non-destructive: every new column is nullable, every new
-- table is additive, nothing existing is dropped or rewritten.

CREATE TABLE IF NOT EXISTS venues (
    id            UUID PRIMARY KEY,
    mic           TEXT NOT NULL,
    exchange_code TEXT NOT NULL DEFAULT '',
    name          TEXT,
    country       TEXT,
    currency      TEXT,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS venues_mic_key ON venues (mic) WHERE mic <> '';

CREATE TABLE IF NOT EXISTS issuers (
    id         UUID PRIMARY KEY,
    name       TEXT NOT NULL,
    cik        TEXT,
    lei        TEXT,
    country    TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS issuers_cik_key ON issuers (cik) WHERE cik IS NOT NULL AND cik <> '';

-- instrument_master keeps its exchange_code/mic text columns as a cheap
-- denormalized snapshot; venue_id/issuer_id become the authoritative link,
-- populated opportunistically as instruments are resolved (see
-- instrument.Resolver.FindOrCreateVenue/FindOrCreateIssuer). No forced
-- backfill here.
ALTER TABLE instrument_master
    ADD COLUMN IF NOT EXISTS venue_id  UUID REFERENCES venues(id),
    ADD COLUMN IF NOT EXISTS issuer_id UUID REFERENCES issuers(id);

CREATE INDEX IF NOT EXISTS idx_instrument_master_venue_id ON instrument_master (venue_id) WHERE venue_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_instrument_master_issuer_id ON instrument_master (issuer_id) WHERE issuer_id IS NOT NULL;

-- A ticker alias is scoped to a specific listing/venue; venue_id is the
-- first-class version of the existing exchange_code/mic scope columns.
ALTER TABLE instrument_aliases
    ADD COLUMN IF NOT EXISTS venue_id UUID REFERENCES venues(id);

CREATE INDEX IF NOT EXISTS idx_instrument_aliases_venue_id ON instrument_aliases (venue_id) WHERE venue_id IS NOT NULL;

-- Legacy positions/portfolio_activities rows with no instrument_id are
-- resolved in stages (see instrument.BackfillJob): strong evidence
-- (FIGI/ISIN, or ticker+MIC+date) is applied directly; everything else lands
-- here for administrative review rather than being guessed.
CREATE TABLE IF NOT EXISTS identity_reconciliation_queue (
    id                     UUID PRIMARY KEY,
    table_name             TEXT NOT NULL,
    record_id              TEXT NOT NULL,
    symbol                 TEXT NOT NULL,
    candidate_instrument_id UUID REFERENCES instrument_master(id),
    evidence               TEXT NOT NULL,
    confidence             TEXT NOT NULL,
    status                 TEXT NOT NULL DEFAULT 'pending',
    created_at             TIMESTAMPTZ NOT NULL DEFAULT now(),
    resolved_at            TIMESTAMPTZ,
    resolved_by            TEXT,
    CONSTRAINT identity_reconciliation_status_valid CHECK (
        status IN ('pending', 'resolved', 'rejected')
    ),
    CONSTRAINT identity_reconciliation_confidence_valid CHECK (
        confidence IN ('high', 'medium', 'low')
    )
);

CREATE INDEX IF NOT EXISTS idx_identity_reconciliation_status
    ON identity_reconciliation_queue (status);

-- One open reconciliation row per (table, record): re-running the backfill
-- job must not pile up duplicate queue entries for the same unresolved row.
CREATE UNIQUE INDEX IF NOT EXISTS identity_reconciliation_pending_uidx
    ON identity_reconciliation_queue (table_name, record_id) WHERE status = 'pending';

-- Corporate actions: identity-aware matching plus a revision counter. The
-- row stays keyed by (provider, provider_event_id) as it already was
-- (corporate_actions_provider_event_uk, migration 0015; corporate_action_
-- applications.corporate_action_id references the PK, so that scheme is
-- deliberately left untouched here). revision is an incrementing audit
-- counter the application bumps every time UpsertEvent detects the
-- provider re-sent the same event with different terms, so a correction is
-- visible instead of silently indistinguishable from the original.
ALTER TABLE corporate_actions
    ADD COLUMN IF NOT EXISTS revision INT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS source_instrument_id UUID REFERENCES instrument_master(id),
    ADD COLUMN IF NOT EXISTS target_instrument_id UUID REFERENCES instrument_master(id);

CREATE INDEX IF NOT EXISTS idx_corporate_actions_source_instrument
    ON corporate_actions (source_instrument_id) WHERE source_instrument_id IS NOT NULL;

-- Outbox events carry the instrument identity plus symbol/provider-reference
-- snapshots at the time of the mutation, so a later rename or alias change
-- never rewrites what already happened.
ALTER TABLE portfolio_outbox
    ADD COLUMN IF NOT EXISTS instrument_id UUID REFERENCES instrument_master(id),
    ADD COLUMN IF NOT EXISTS display_symbol_at_event_time TEXT,
    ADD COLUMN IF NOT EXISTS provider_reference_at_event_time TEXT;
