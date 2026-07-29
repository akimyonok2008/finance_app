-- Privacy-safe, generation-swapped Explore projection.
--
-- Full public cards are built by the leader-elected background worker and
-- published atomically. Request handling then filters, sorts and paginates
-- these JSON documents in PostgreSQL instead of loading every public profile
-- and valuing every portfolio.

CREATE TABLE IF NOT EXISTS explore_projection_state (
    id                BOOLEAN PRIMARY KEY DEFAULT TRUE CHECK (id),
    active_generation BIGINT NOT NULL DEFAULT 0,
    updated_at        TIMESTAMPTZ
);

INSERT INTO explore_projection_state (id) VALUES (TRUE)
ON CONFLICT (id) DO NOTHING;

CREATE TABLE IF NOT EXISTS explore_public_cards (
    generation BIGINT NOT NULL,
    timeframe  TEXT NOT NULL CHECK (timeframe IN ('ALL', '1W', '1M', '3M', '6M', '1Y')),
    user_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    updated_at TIMESTAMPTZ NOT NULL,
    card       JSONB NOT NULL,
    PRIMARY KEY (generation, timeframe, user_id)
);

CREATE INDEX IF NOT EXISTS explore_public_cards_rank_idx
    ON explore_public_cards (
        generation,
        timeframe,
        ((card->>'global_rank')::INTEGER),
        ((card->>'handle'))
    );

CREATE INDEX IF NOT EXISTS explore_public_cards_return_idx
    ON explore_public_cards (
        generation,
        timeframe,
        ((card->>'return_percentage')::NUMERIC) DESC,
        ((card->>'handle'))
    );

CREATE INDEX IF NOT EXISTS explore_public_cards_recent_idx
    ON explore_public_cards (generation, timeframe, updated_at DESC);

CREATE INDEX IF NOT EXISTS explore_public_cards_search_idx
    ON explore_public_cards USING GIN (card jsonb_path_ops);

CREATE TABLE IF NOT EXISTS explore_trending_holdings (
    generation    BIGINT NOT NULL,
    timeframe     TEXT NOT NULL CHECK (timeframe IN ('ALL', '1W', '1M', '3M', '6M', '1Y')),
    symbol        TEXT NOT NULL,
    profile_count INTEGER NOT NULL CHECK (profile_count > 0),
    weight_sum    DOUBLE PRECISION NOT NULL,
    top10_count   INTEGER NOT NULL CHECK (top10_count >= 0),
    asset_type    TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (generation, timeframe, symbol)
);

CREATE INDEX IF NOT EXISTS explore_trending_holdings_order_idx
    ON explore_trending_holdings (generation, timeframe, profile_count DESC, weight_sum DESC, symbol);

-- A privacy/visibility change must never leave an old card generation active.
-- The next request uses the exact live fallback until the worker publishes a
-- new complete generation.
CREATE OR REPLACE FUNCTION invalidate_explore_projection_on_profile_change()
RETURNS TRIGGER AS $$
BEGIN
    IF OLD.is_public IS DISTINCT FROM NEW.is_public
       OR OLD.show_public_weights IS DISTINCT FROM NEW.show_public_weights
       OR OLD.handle IS DISTINCT FROM NEW.handle
       OR OLD.display_name IS DISTINCT FROM NEW.display_name
       OR OLD.avatar_key IS DISTINCT FROM NEW.avatar_key
       OR OLD.bio IS DISTINCT FROM NEW.bio
       OR OLD.strategy_tag IS DISTINCT FROM NEW.strategy_tag THEN
        UPDATE explore_projection_state SET updated_at = NULL WHERE id = TRUE;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS profiles_invalidate_explore_projection ON profiles;
CREATE TRIGGER profiles_invalidate_explore_projection
AFTER UPDATE ON profiles
FOR EACH ROW EXECUTE FUNCTION invalidate_explore_projection_on_profile_change();
