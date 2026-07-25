-- Promote position_episode_id from JSONB-only presentation metadata to a
-- first-class, FK-constrained column. The write path already assigns a
-- stable episode identity equal to the owning positions.id (see
-- coordinator.go MutationBuy/MutationSell): a rebuy after a full sale always
-- creates a new positions row, so positions.id already is a durable episode
-- identity. This migration just gives that identity a real column and
-- constraint instead of relying on JSONB extraction.
--
-- Forward-only and idempotent.

ALTER TABLE portfolio_activities
    ADD COLUMN IF NOT EXISTS position_episode_id UUID;

UPDATE portfolio_activities
SET position_episode_id = (metadata_json->>'position_episode_id')::uuid
WHERE position_episode_id IS NULL
  AND metadata_json->>'position_episode_id' ~*
      '^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$';

ALTER TABLE portfolio_activities
    DROP CONSTRAINT IF EXISTS portfolio_activities_episode_fk;
ALTER TABLE portfolio_activities
    ADD CONSTRAINT portfolio_activities_episode_fk
        FOREIGN KEY (position_episode_id) REFERENCES positions(id);

DROP INDEX IF EXISTS portfolio_activities_episode_idx;
CREATE INDEX IF NOT EXISTS portfolio_activities_episode_id_idx
    ON portfolio_activities (portfolio_id, position_episode_id, occurred_at DESC)
    WHERE position_episode_id IS NOT NULL;
