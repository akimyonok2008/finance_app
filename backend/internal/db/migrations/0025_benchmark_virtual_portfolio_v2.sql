-- Correct the durable audit metadata to match the policy now executed by the
-- virtual benchmark engine. There are no existing benchmark awards to migrate
-- or revalidate; this changes recipe metadata only.

UPDATE benchmark_recipe_versions
SET rebalancing_policy = 'filing_snapshot_hold'
WHERE recipe_id = 'BUFFETT_13F'
  AND rebalancing_policy <> 'filing_snapshot_hold';
