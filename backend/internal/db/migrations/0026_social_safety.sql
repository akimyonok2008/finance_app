-- Minimum viable social-safety system: blocking, reporting + evidence,
-- moderation actions, message visibility/unread tracking, notifications,
-- and account role/suspension/ban state.

ALTER TABLE users
    ADD COLUMN IF NOT EXISTS role TEXT NOT NULL DEFAULT 'user'
        CHECK (role IN ('user', 'moderator', 'admin')),
    ADD COLUMN IF NOT EXISTS suspended_until TIMESTAMPTZ NULL,
    ADD COLUMN IF NOT EXISTS suspension_reason TEXT NULL,
    ADD COLUMN IF NOT EXISTS banned_at TIMESTAMPTZ NULL,
    ADD COLUMN IF NOT EXISTS ban_reason TEXT NULL;

CREATE TABLE IF NOT EXISTS user_blocks (
    blocker_user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    blocked_user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (blocker_user_id, blocked_user_id),
    CONSTRAINT user_blocks_no_self CHECK (blocker_user_id <> blocked_user_id)
);

CREATE INDEX IF NOT EXISTS user_blocks_blocked_idx ON user_blocks(blocked_user_id);

CREATE TABLE IF NOT EXISTS user_reports (
    id UUID PRIMARY KEY,
    reporter_user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    reported_user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    message_id UUID NULL REFERENCES dm_messages(id) ON DELETE SET NULL,
    category TEXT NOT NULL CHECK (category IN (
        'harassment', 'spam', 'impersonation', 'fraud_or_scam',
        'hate_or_abuse', 'sexual_content', 'threats', 'privacy_violation', 'other'
    )),
    explanation TEXT NULL,
    status TEXT NOT NULL DEFAULT 'open'
        CHECK (status IN ('open', 'under_review', 'resolved_action_taken', 'resolved_no_action')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    reviewed_at TIMESTAMPTZ NULL,
    reviewer_id UUID NULL REFERENCES users(id) ON DELETE SET NULL,
    decision TEXT NULL,
    moderator_notes TEXT NULL,
    CONSTRAINT user_reports_no_self CHECK (reporter_user_id <> reported_user_id)
);

CREATE INDEX IF NOT EXISTS user_reports_status_idx ON user_reports(status);
CREATE INDEX IF NOT EXISTS user_reports_reported_idx ON user_reports(reported_user_id);
-- Prevent duplicate OPEN/UNDER_REVIEW reports for the same reporter+target+evidence.
CREATE UNIQUE INDEX IF NOT EXISTS user_reports_open_dedupe_idx
    ON user_reports(reporter_user_id, reported_user_id, COALESCE(message_id, '00000000-0000-0000-0000-000000000000'))
    WHERE status IN ('open', 'under_review');

CREATE TABLE IF NOT EXISTS report_evidence (
    id UUID PRIMARY KEY,
    report_id UUID NOT NULL REFERENCES user_reports(id) ON DELETE CASCADE,
    message_id UUID NULL,
    conversation_id UUID NULL,
    sender_id UUID NULL,
    participant_ids TEXT[] NOT NULL DEFAULT '{}',
    message_text TEXT NULL,
    message_created_at TIMESTAMPTZ NULL,
    report_created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS report_evidence_report_idx ON report_evidence(report_id);

CREATE TABLE IF NOT EXISTS moderation_actions (
    id UUID PRIMARY KEY,
    moderator_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    target_user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    report_id UUID NULL REFERENCES user_reports(id) ON DELETE SET NULL,
    action_type TEXT NOT NULL CHECK (action_type IN (
        'no_action', 'warning', 'temporary_suspension', 'permanent_ban', 'content_removal'
    )),
    reason TEXT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at TIMESTAMPTZ NULL
);

CREATE INDEX IF NOT EXISTS moderation_actions_target_idx ON moderation_actions(target_user_id);
CREATE INDEX IF NOT EXISTS moderation_actions_report_idx ON moderation_actions(report_id);

CREATE TABLE IF NOT EXISTS message_visibility (
    message_id UUID NOT NULL REFERENCES dm_messages(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    hidden_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (message_id, user_id)
);

ALTER TABLE dm_messages
    ADD COLUMN IF NOT EXISTS removed_at TIMESTAMPTZ NULL,
    ADD COLUMN IF NOT EXISTS removed_by UUID NULL REFERENCES users(id) ON DELETE SET NULL;

CREATE TABLE IF NOT EXISTS conversation_participant_state (
    conversation_id UUID NOT NULL REFERENCES dm_conversations(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    last_read_message_id UUID NULL,
    last_read_at TIMESTAMPTZ NULL,
    state TEXT NOT NULL DEFAULT 'active' CHECK (state IN ('active', 'blocked', 'declined')),
    PRIMARY KEY (conversation_id, user_id)
);

CREATE INDEX IF NOT EXISTS conversation_participant_state_user_idx ON conversation_participant_state(user_id);

CREATE TABLE IF NOT EXISTS notifications (
    id UUID PRIMARY KEY,
    recipient_user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    type TEXT NOT NULL,
    payload JSONB NOT NULL DEFAULT '{}'::jsonb,
    dedupe_key TEXT NOT NULL,
    read_at TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (recipient_user_id, dedupe_key)
);

CREATE INDEX IF NOT EXISTS notifications_recipient_unread_idx ON notifications(recipient_user_id, read_at);
