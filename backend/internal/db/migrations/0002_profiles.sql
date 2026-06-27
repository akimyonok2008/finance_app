CREATE TABLE IF NOT EXISTS profiles (
    user_id UUID PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    handle TEXT NOT NULL UNIQUE,
    display_name TEXT NOT NULL,
    avatar_key TEXT NOT NULL DEFAULT '',
    bio TEXT NOT NULL DEFAULT '',
    strategy_tag TEXT NOT NULL DEFAULT 'balanced_global',
    is_public BOOLEAN NOT NULL DEFAULT FALSE,
    show_public_weights BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT profiles_handle_format CHECK (handle ~ '^[a-z0-9_-]{3,30}$'),
    CONSTRAINT profiles_display_name_length CHECK (char_length(display_name) BETWEEN 2 AND 40),
    CONSTRAINT profiles_avatar_key_length CHECK (char_length(avatar_key) <= 40),
    CONSTRAINT profiles_bio_length CHECK (char_length(bio) <= 160),
    CONSTRAINT profiles_strategy_tag_valid CHECK (
        strategy_tag IN (
            'conservative', 'balanced_global', 'growth', 'dividend_income',
            'tech_focused', 'value', 'crypto_heavy', 'esg', 'active_trader',
            'long_term_investor'
        )
    )
);

CREATE INDEX IF NOT EXISTS profiles_handle_idx ON profiles(handle);
