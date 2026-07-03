CREATE TABLE IF NOT EXISTS follows (
    follower_user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    following_user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (follower_user_id, following_user_id),
    CONSTRAINT follows_no_self CHECK (follower_user_id <> following_user_id)
);

CREATE INDEX IF NOT EXISTS follows_following_idx ON follows(following_user_id);

CREATE TABLE IF NOT EXISTS dm_conversations (
    id UUID PRIMARY KEY,
    participant_a_user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    participant_b_user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    pair_key TEXT NOT NULL UNIQUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_message_at TIMESTAMPTZ NULL,
    CONSTRAINT dm_conversations_no_self CHECK (participant_a_user_id <> participant_b_user_id)
);

CREATE INDEX IF NOT EXISTS dm_conversations_participant_a_idx ON dm_conversations(participant_a_user_id);
CREATE INDEX IF NOT EXISTS dm_conversations_participant_b_idx ON dm_conversations(participant_b_user_id);

CREATE TABLE IF NOT EXISTS dm_conversation_participants (
    conversation_id UUID NOT NULL REFERENCES dm_conversations(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (conversation_id, user_id)
);

CREATE INDEX IF NOT EXISTS dm_conversation_participants_user_idx ON dm_conversation_participants(user_id);

CREATE TABLE IF NOT EXISTS dm_messages (
    id UUID PRIMARY KEY,
    conversation_id UUID NOT NULL REFERENCES dm_conversations(id) ON DELETE CASCADE,
    sender_user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    body TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    edited_at TIMESTAMPTZ NULL,
    deleted_at TIMESTAMPTZ NULL,
    CONSTRAINT dm_messages_body_length CHECK (char_length(body) BETWEEN 1 AND 1000)
);

CREATE INDEX IF NOT EXISTS dm_messages_conversation_created_idx ON dm_messages(conversation_id, created_at);
