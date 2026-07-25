-- Explicit Activity → Portfolio → Performance read architecture.
-- Activity presentation metadata remains in the immutable metadata_json column
-- so historical rows are enriched without rewriting their financial facts.

UPDATE portfolio_activities
SET metadata_json = metadata_json || jsonb_build_object(
    'origin',
    CASE
        WHEN activity_type = 'opening_balance' THEN 'migration_generated'
        WHEN metadata_json->>'provenance' = 'provider_reported' THEN 'provider_generated'
        WHEN metadata_json->>'provenance' = 'system_generated' THEN 'system_generated'
        WHEN metadata_json->>'provenance' = 'migration_generated' THEN 'migration_generated'
        WHEN activity_type IN (
            'cash_dividend', 'etf_distribution', 'interest_income',
            'reinvested_dividend', 'stock_split', 'reverse_split',
            'symbol_change', 'stock_dividend'
        ) THEN 'system_generated'
        ELSE 'user_recorded'
    END,
    'status', COALESCE(metadata_json->>'status', 'completed')
)
WHERE NOT metadata_json ? 'origin' OR NOT metadata_json ? 'status';

UPDATE portfolio_activities a
SET metadata_json = metadata_json || jsonb_build_object(
    'position_episode_id',
    COALESCE(
        metadata_json->>'legacy_position_id',
        (
            SELECT p.id::text
            FROM positions p
            WHERE p.portfolio_id = a.portfolio_id
              AND p.symbol = a.symbol
            ORDER BY
                CASE WHEN p.created_at <= a.occurred_at THEN 0 ELSE 1 END,
                abs(extract(epoch FROM (a.occurred_at - p.created_at)))
            LIMIT 1
        )
    )
)
WHERE a.symbol IS NOT NULL
  AND a.activity_type IN ('buy', 'sell', 'opening_balance')
  AND NOT a.metadata_json ? 'position_episode_id';

CREATE INDEX IF NOT EXISTS portfolio_activities_episode_idx
    ON portfolio_activities (portfolio_id, (metadata_json->>'position_episode_id'), occurred_at DESC)
    WHERE metadata_json ? 'position_episode_id';

CREATE INDEX IF NOT EXISTS portfolio_activities_origin_idx
    ON portfolio_activities (portfolio_id, (metadata_json->>'origin'), occurred_at DESC);

CREATE INDEX IF NOT EXISTS portfolio_activities_status_idx
    ON portfolio_activities (portfolio_id, (metadata_json->>'status'), occurred_at DESC);
