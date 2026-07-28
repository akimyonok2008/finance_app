-- Retain moderation evidence and audit history after account deletion.
--
-- Today user_reports.reporter_user_id/reported_user_id and
-- moderation_actions.moderator_id/target_user_id all CASCADE off users(id).
-- Account deletion is currently a soft delete (users.deleted_at) in the Go
-- code, so this CASCADE is dormant, but it is a live landmine: any future
-- hard-delete path (GDPR erasure, data-retention cleanup, etc.) would
-- silently destroy report/evidence/audit rows moderators and compliance
-- rely on. This migration removes that risk without waiting for such a path
-- to be written.
--
-- Design: a pseudonymous subject-id mapping table, `moderation_subject_ids`,
-- assigns every user_id referenced by moderation a random (not derived from
-- user_id/email) opaque id the moment they're first referenced. The FK
-- columns on user_reports/moderation_actions are changed from CASCADE to
-- SET NULL, and the new *_subject_id columns are backfilled from the
-- mapping table so a moderator can still tell "the same reporter" or "the
-- same target" apart across multiple reports even after the live user_id
-- FK has been nulled out by a future hard delete.
--
-- Backfill approach chosen: `gen_random_bytes(16)` via pgcrypto, one row per
-- distinct user_id already referenced by user_reports/moderation_actions.
-- This is simpler and safer than an HMAC-based derivation: it needs no
-- Go-readable secret inside a migration (which would either have to be
-- hardcoded — a real secret leak — or sourced from a GUC the app sets on
-- every connection, adding an operational dependency this migration
-- shouldn't need), and a random id is non-reversible by construction, not
-- merely non-reversible-assuming-the-key-stays-secret.

CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE IF NOT EXISTS moderation_subject_ids (
    user_id UUID PRIMARY KEY,
    subject_id TEXT NOT NULL DEFAULT encode(gen_random_bytes(16), 'hex'),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS moderation_subject_ids_subject_idx
    ON moderation_subject_ids(subject_id);

-- Backfill the mapping for every user_id already referenced by moderation
-- rows, whether or not the users row itself still exists.
INSERT INTO moderation_subject_ids (user_id)
SELECT DISTINCT user_id FROM (
    SELECT reporter_user_id AS user_id FROM user_reports
    UNION
    SELECT reported_user_id AS user_id FROM user_reports
    UNION
    SELECT moderator_id AS user_id FROM moderation_actions
    UNION
    SELECT target_user_id AS user_id FROM moderation_actions
) referenced_users
ON CONFLICT (user_id) DO NOTHING;

-- --- user_reports -----------------------------------------------------------

ALTER TABLE user_reports
    ADD COLUMN IF NOT EXISTS reporter_subject_id TEXT NULL,
    ADD COLUMN IF NOT EXISTS reported_subject_id TEXT NULL;

UPDATE user_reports r
SET reporter_subject_id = m.subject_id
FROM moderation_subject_ids m
WHERE m.user_id = r.reporter_user_id
  AND r.reporter_subject_id IS NULL;

UPDATE user_reports r
SET reported_subject_id = m.subject_id
FROM moderation_subject_ids m
WHERE m.user_id = r.reported_user_id
  AND r.reported_subject_id IS NULL;

ALTER TABLE user_reports
    ALTER COLUMN reporter_subject_id SET NOT NULL,
    ALTER COLUMN reported_subject_id SET NOT NULL;

ALTER TABLE user_reports
    ALTER COLUMN reporter_user_id DROP NOT NULL,
    ALTER COLUMN reported_user_id DROP NOT NULL;

ALTER TABLE user_reports DROP CONSTRAINT IF EXISTS user_reports_reporter_user_id_fkey;
ALTER TABLE user_reports
    ADD CONSTRAINT user_reports_reporter_user_id_fkey
    FOREIGN KEY (reporter_user_id) REFERENCES users(id) ON DELETE SET NULL;

ALTER TABLE user_reports DROP CONSTRAINT IF EXISTS user_reports_reported_user_id_fkey;
ALTER TABLE user_reports
    ADD CONSTRAINT user_reports_reported_user_id_fkey
    FOREIGN KEY (reported_user_id) REFERENCES users(id) ON DELETE SET NULL;

-- --- moderation_actions ------------------------------------------------------

ALTER TABLE moderation_actions
    ADD COLUMN IF NOT EXISTS moderator_subject_id TEXT NULL,
    ADD COLUMN IF NOT EXISTS target_subject_id TEXT NULL;

UPDATE moderation_actions a
SET moderator_subject_id = m.subject_id
FROM moderation_subject_ids m
WHERE m.user_id = a.moderator_id
  AND a.moderator_subject_id IS NULL;

UPDATE moderation_actions a
SET target_subject_id = m.subject_id
FROM moderation_subject_ids m
WHERE m.user_id = a.target_user_id
  AND a.target_subject_id IS NULL;

ALTER TABLE moderation_actions
    ALTER COLUMN moderator_subject_id SET NOT NULL,
    ALTER COLUMN target_subject_id SET NOT NULL;

ALTER TABLE moderation_actions
    ALTER COLUMN moderator_id DROP NOT NULL,
    ALTER COLUMN target_user_id DROP NOT NULL;

ALTER TABLE moderation_actions DROP CONSTRAINT IF EXISTS moderation_actions_moderator_id_fkey;
ALTER TABLE moderation_actions
    ADD CONSTRAINT moderation_actions_moderator_id_fkey
    FOREIGN KEY (moderator_id) REFERENCES users(id) ON DELETE SET NULL;

ALTER TABLE moderation_actions DROP CONSTRAINT IF EXISTS moderation_actions_target_user_id_fkey;
ALTER TABLE moderation_actions
    ADD CONSTRAINT moderation_actions_target_user_id_fkey
    FOREIGN KEY (target_user_id) REFERENCES users(id) ON DELETE SET NULL;

-- Note: user_reports_open_dedupe_idx (the partial unique index enforcing at
-- most one open/under_review report per reporter+reported+message for LIVE
-- accounts) is untouched: it is keyed on the still-present
-- reporter_user_id/reported_user_id columns, and once either goes NULL via a
-- future hard delete the report can no longer be "open" against a live
-- account pair, so the index's guarantee is preserved for the case it
-- exists to cover.
