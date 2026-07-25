-- Idempotency keys are scoped to a portfolio aggregate. The same client key
-- used by two different users must never replay or reveal another aggregate's
-- mutation.
DROP INDEX IF EXISTS portfolio_mutation_audit_request_key;
CREATE UNIQUE INDEX IF NOT EXISTS portfolio_mutation_audit_portfolio_request_uidx
    ON portfolio_mutation_audit (portfolio_id, request_id)
    WHERE request_id IS NOT NULL;
