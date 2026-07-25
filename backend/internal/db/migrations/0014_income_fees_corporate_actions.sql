-- Income, fees, and corporate actions on the immutable activity ledger.
--
-- Forward-only and idempotent. Extends portfolio_activities to accept the new
-- user-reported activity types without destroying legacy history:
--   income  : cash_dividend, etf_distribution, interest_income, reinvested_dividend
--   fees    : buy_fee, sell_fee, management_fee, custody_fee, other_fee
--   corp    : stock_split, reverse_split, symbol_change, write_off
--
-- Grouping and provenance are stored in metadata_json (activity_group_id,
-- provenance, performance_effect, corporate_action, ...), so no destructive
-- column rewrite is required; a functional index makes grouped legs queryable.

ALTER TABLE portfolio_activities
    DROP CONSTRAINT IF EXISTS portfolio_activity_type_valid,
    DROP CONSTRAINT IF EXISTS portfolio_activity_gross_positive,
    DROP CONSTRAINT IF EXISTS portfolio_activity_required_fields;

ALTER TABLE portfolio_activities
    ADD CONSTRAINT portfolio_activity_type_valid CHECK (
        activity_type IN (
            'deposit', 'withdrawal', 'buy', 'sell', 'opening_balance',
            'cash_dividend', 'etf_distribution', 'interest_income', 'reinvested_dividend',
            'buy_fee', 'sell_fee', 'management_fee', 'custody_fee', 'other_fee',
            'stock_split', 'reverse_split', 'symbol_change', 'write_off'
        )
    );

-- Gross amount is non-negative overall; corporate-action transforms (split,
-- symbol change, write-off) carry a zero gross, while every money-moving type
-- must be strictly positive.
ALTER TABLE portfolio_activities
    ADD CONSTRAINT portfolio_activity_gross_non_negative CHECK (gross_amount >= 0);

ALTER TABLE portfolio_activities
    ADD CONSTRAINT portfolio_activity_money_gross_positive CHECK (
        activity_type NOT IN (
            'deposit', 'withdrawal', 'buy', 'sell', 'opening_balance',
            'cash_dividend', 'etf_distribution', 'interest_income', 'reinvested_dividend',
            'buy_fee', 'sell_fee', 'management_fee', 'custody_fee', 'other_fee'
        )
        OR gross_amount > 0
    );

-- Per-type field requirements. Trades carry symbol+quantity+price; dividends and
-- symbol-affecting corporate actions carry a symbol; splits/write-offs carry a
-- symbol and a quantity; cash flows, interest, and standalone fees need neither.
ALTER TABLE portfolio_activities
    ADD CONSTRAINT portfolio_activity_required_fields CHECK (
        (activity_type IN ('buy', 'sell', 'opening_balance')
            AND symbol IS NOT NULL AND quantity IS NOT NULL AND unit_price IS NOT NULL)
        OR (activity_type IN ('cash_dividend', 'etf_distribution', 'reinvested_dividend', 'symbol_change')
            AND symbol IS NOT NULL)
        OR (activity_type IN ('stock_split', 'reverse_split', 'write_off')
            AND symbol IS NOT NULL AND quantity IS NOT NULL)
        OR (activity_type IN ('deposit', 'withdrawal', 'interest_income',
            'management_fee', 'custody_fee', 'other_fee', 'buy_fee', 'sell_fee'))
    );

-- Grouped economic events (e.g. reinvested dividend = income leg + buy leg)
-- share an activity_group_id in metadata; index it for timeline grouping.
CREATE INDEX IF NOT EXISTS portfolio_activities_group_idx
    ON portfolio_activities ((metadata_json ->> 'activity_group_id'))
    WHERE metadata_json ? 'activity_group_id';
