-- Protected competition control-plane audit and retry history.
-- Rows are append-only: there is intentionally no update/delete application path.

CREATE TABLE competition_admin_audit (
    id               UUID PRIMARY KEY,
    actor_user_id    UUID REFERENCES users(id) ON DELETE SET NULL,
    action           TEXT NOT NULL,
    target_type      TEXT NOT NULL CHECK (target_type IN ('definition', 'definition_version', 'edition', 'job')),
    target_id        TEXT NOT NULL,
    competition_id   TEXT REFERENCES competitions(id) ON DELETE SET NULL,
    request_id       TEXT,
    reason           TEXT,
    details_json     JSONB NOT NULL DEFAULT '{}'::jsonb,
    succeeded        BOOLEAN NOT NULL,
    error_message    TEXT,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX competition_admin_audit_target_idx
    ON competition_admin_audit (target_type, target_id, created_at DESC);
CREATE INDEX competition_admin_audit_competition_idx
    ON competition_admin_audit (competition_id, created_at DESC);
CREATE INDEX competition_admin_audit_actor_idx
    ON competition_admin_audit (actor_user_id, created_at DESC);
