-- Support durable sender-wide message abuse checks. Conversation-window checks
-- already use dm_messages_conversation_created_idx from migration 0005.

CREATE INDEX IF NOT EXISTS dm_messages_sender_created_idx
    ON dm_messages (sender_user_id, created_at DESC);
