-- Cash-funded portfolio mechanics and immutable owner-private activity ledger.

CREATE TABLE IF NOT EXISTS portfolio_cash_balances (
    portfolio_id UUID NOT NULL REFERENCES portfolios(id) ON DELETE CASCADE,
    currency TEXT NOT NULL,
    amount NUMERIC(24, 8) NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (portfolio_id, currency),
    CONSTRAINT portfolio_cash_currency_upper CHECK (currency = upper(currency)),
    CONSTRAINT portfolio_cash_currency_supported CHECK (currency IN ('USD', 'TRY', 'EUR', 'GBP')),
    CONSTRAINT portfolio_cash_non_negative CHECK (amount >= 0)
);

CREATE INDEX IF NOT EXISTS portfolio_cash_currency_idx
    ON portfolio_cash_balances (portfolio_id, currency);

CREATE TABLE IF NOT EXISTS portfolio_activities (
    id UUID PRIMARY KEY,
    request_id TEXT,
    portfolio_id UUID NOT NULL REFERENCES portfolios(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    activity_type TEXT NOT NULL,
    symbol TEXT,
    asset_type TEXT,
    currency TEXT NOT NULL,
    quantity NUMERIC(24, 8),
    unit_price NUMERIC(24, 8),
    gross_amount NUMERIC(24, 8) NOT NULL,
    cost_basis_allocated NUMERIC(24, 8),
    realized_gain_loss_base NUMERIC(24, 8),
    realized_gain_loss_percentage NUMERIC(18, 8),
    occurred_at TIMESTAMPTZ NOT NULL,
    portfolio_version BIGINT NOT NULL,
    metadata_json JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    reversal_of_activity_id UUID REFERENCES portfolio_activities(id),
    CONSTRAINT portfolio_activity_type_valid CHECK (
        activity_type IN ('deposit', 'withdrawal', 'buy', 'sell', 'opening_balance')
    ),
    CONSTRAINT portfolio_activity_currency_upper CHECK (currency = upper(currency)),
    CONSTRAINT portfolio_activity_currency_supported CHECK (currency IN ('USD', 'TRY', 'EUR', 'GBP')),
    CONSTRAINT portfolio_activity_gross_positive CHECK (gross_amount > 0),
    CONSTRAINT portfolio_activity_quantity_positive CHECK (quantity IS NULL OR quantity > 0),
    CONSTRAINT portfolio_activity_price_positive CHECK (unit_price IS NULL OR unit_price > 0),
    CONSTRAINT portfolio_activity_required_fields CHECK (
        (activity_type IN ('deposit', 'withdrawal') AND symbol IS NULL AND quantity IS NULL AND unit_price IS NULL)
        OR
        (activity_type IN ('buy', 'sell', 'opening_balance') AND symbol IS NOT NULL AND quantity IS NOT NULL AND unit_price IS NOT NULL)
    )
);

UPDATE positions SET currency = upper(trim(currency));

CREATE UNIQUE INDEX IF NOT EXISTS portfolio_activities_request_uidx
    ON portfolio_activities (portfolio_id, request_id) WHERE request_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS portfolio_activities_timeline_idx
    ON portfolio_activities (portfolio_id, occurred_at DESC, created_at DESC);
CREATE INDEX IF NOT EXISTS portfolio_activities_user_idx
    ON portfolio_activities (user_id, occurred_at DESC);

DROP INDEX IF EXISTS portfolio_mutation_audit_request_key;
CREATE UNIQUE INDEX IF NOT EXISTS portfolio_mutation_audit_portfolio_request_uidx
    ON portfolio_mutation_audit (portfolio_id, request_id)
    WHERE request_id IS NOT NULL;

-- Existing holdings are legacy capital, not fabricated deposits or buys.
INSERT INTO portfolio_activities (
    id, portfolio_id, user_id, activity_type, symbol, asset_type, currency,
    quantity, unit_price, gross_amount, occurred_at, portfolio_version,
    metadata_json, created_at
)
SELECT gen_random_uuid(), p.portfolio_id, p.user_id, 'opening_balance',
       p.symbol, p.asset_type, upper(p.currency), p.quantity,
       p.average_buy_price, p.quantity * p.average_buy_price,
       p.created_at, COALESCE(pf.version, 1),
       '{"legacy_import":true,"funding_not_inferred":true}'::jsonb, now()
FROM positions p
JOIN portfolios pf ON pf.id = p.portfolio_id
WHERE COALESCE(p.status, 'open') = 'open'
  AND NOT EXISTS (
      SELECT 1 FROM portfolio_activities a
      WHERE a.portfolio_id = p.portfolio_id
        AND a.activity_type = 'opening_balance'
        AND a.metadata_json->>'legacy_position_id' = p.id::text
  );

-- Attach stable legacy ids after insert so reruns are idempotent on databases
-- where this migration file is applied by a repeatable local test harness.
UPDATE portfolio_activities a
SET metadata_json = a.metadata_json || jsonb_build_object(
    'legacy_position_id',
    (
        SELECT p.id::text FROM positions p
        WHERE p.portfolio_id = a.portfolio_id
          AND p.symbol = a.symbol
          AND p.asset_type = a.asset_type
          AND p.currency = a.currency
          AND p.created_at = a.occurred_at
        LIMIT 1
    )
)
WHERE a.activity_type = 'opening_balance'
  AND a.metadata_json->>'legacy_position_id' IS NULL;

CREATE UNIQUE INDEX IF NOT EXISTS portfolio_opening_activity_legacy_uidx
    ON portfolio_activities ((metadata_json->>'legacy_position_id'))
    WHERE activity_type = 'opening_balance';

-- Aggregate legacy duplicate open positions only after recording one opening
-- activity per source row. The materialized state uses weighted-average basis;
-- the immutable opening records preserve the migration provenance.
WITH grouped AS (
    SELECT portfolio_id, symbol, asset_type, currency,
           (array_agg(id ORDER BY created_at, id))[1] AS keep_id,
           sum(quantity) AS total_quantity,
           sum(quantity * average_buy_price) / sum(quantity) AS weighted_basis,
           min(created_at) AS first_created_at
    FROM positions
    WHERE COALESCE(status, 'open') = 'open'
    GROUP BY portfolio_id, symbol, asset_type, currency
    HAVING count(*) > 1
)
UPDATE positions p
SET quantity = g.total_quantity,
    average_buy_price = g.weighted_basis,
    created_at = g.first_created_at,
    updated_at = now()
FROM grouped g
WHERE p.id = g.keep_id;

WITH keepers AS (
    SELECT (array_agg(id ORDER BY created_at, id))[1] AS keep_id,
           portfolio_id, symbol, asset_type, currency
    FROM positions
    WHERE COALESCE(status, 'open') = 'open'
    GROUP BY portfolio_id, symbol, asset_type, currency
    HAVING count(*) > 1
)
DELETE FROM positions p
USING keepers k
WHERE p.portfolio_id = k.portfolio_id
  AND p.symbol = k.symbol
  AND p.asset_type = k.asset_type
  AND p.currency = k.currency
  AND COALESCE(p.status, 'open') = 'open'
  AND p.id <> k.keep_id;

CREATE UNIQUE INDEX IF NOT EXISTS positions_one_open_instrument_uidx
    ON positions (portfolio_id, symbol, asset_type, currency)
    WHERE COALESCE(status, 'open') = 'open';
