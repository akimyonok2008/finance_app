-- Backfill accounts created before portfolios moved from lazy GET
-- initialization to the account-creation workflow.
INSERT INTO portfolios (id, user_id, name, currency, version, created_at, updated_at)
SELECT gen_random_uuid(), u.id, 'Default Portfolio', 'USD', 1, now(), now()
FROM users u
WHERE u.deleted_at IS NULL
  AND NOT EXISTS (SELECT 1 FROM portfolios p WHERE p.user_id = u.id)
ON CONFLICT (user_id) DO NOTHING;
