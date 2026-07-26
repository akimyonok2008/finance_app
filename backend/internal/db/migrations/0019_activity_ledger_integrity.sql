-- Activity ledger integrity: bring the PostgreSQL CHECK constraints on
-- portfolio_activities up to parity with the full Go ActivityType enum
-- (activities_model.go / model.go). Forward-only and idempotent; does not
-- edit any previously applied migration.
--
-- Root cause: migration 0014 only whitelisted the first wave of income/fee/
-- corporate-action types. The domain has since grown additional income
-- subtypes (capital_gains_distribution, bond_coupon, cash_interest,
-- staking_reward, other_income), return_of_capital, and stock_dividend, none
-- of which Postgres would accept — inserting them fails the CHECK constraint
-- even though the Go domain considers them valid.

ALTER TABLE portfolio_activities
    DROP CONSTRAINT IF EXISTS portfolio_activity_type_valid,
    DROP CONSTRAINT IF EXISTS portfolio_activity_gross_non_negative,
    DROP CONSTRAINT IF EXISTS portfolio_activity_gross_positive,
    DROP CONSTRAINT IF EXISTS portfolio_activity_money_gross_positive,
    DROP CONSTRAINT IF EXISTS portfolio_activity_required_fields;

-- Full whitelist: base ledger + income + fees + corporate actions.
ALTER TABLE portfolio_activities
    ADD CONSTRAINT portfolio_activity_type_valid CHECK (
        activity_type IN (
            -- base
            'deposit', 'withdrawal', 'buy', 'sell', 'opening_balance',
            -- income
            'cash_dividend', 'etf_distribution', 'interest_income', 'reinvested_dividend',
            'capital_gains_distribution', 'bond_coupon', 'cash_interest', 'staking_reward',
            'other_income', 'return_of_capital', 'stock_dividend',
            -- fees
            'buy_fee', 'sell_fee', 'management_fee', 'custody_fee', 'other_fee',
            -- corporate actions
            'stock_split', 'reverse_split', 'symbol_change', 'write_off'
        )
    );

-- Gross amount is never negative.
ALTER TABLE portfolio_activities
    ADD CONSTRAINT portfolio_activity_gross_non_negative CHECK (gross_amount >= 0);

-- Every money-moving activity type must carry a strictly positive gross
-- amount. Zero-value transformations (stock_dividend, stock_split,
-- reverse_split, symbol_change, write_off) are permitted to have a zero gross
-- amount since they move quantity/identity, not cash.
ALTER TABLE portfolio_activities
    ADD CONSTRAINT portfolio_activity_money_gross_positive CHECK (
        activity_type NOT IN (
            'deposit', 'withdrawal', 'buy', 'sell', 'opening_balance',
            'cash_dividend', 'etf_distribution', 'interest_income', 'reinvested_dividend',
            'capital_gains_distribution', 'bond_coupon', 'cash_interest', 'staking_reward',
            'other_income', 'return_of_capital',
            'buy_fee', 'sell_fee', 'management_fee', 'custody_fee', 'other_fee'
        )
        OR gross_amount > 0
    );

-- Per-type field requirements, aligned with the domain (activities_model.go):
--   * trades (buy/sell/opening_balance) require symbol + quantity + price.
--   * symbol-scoped income/corporate-action types require a symbol.
--   * splits/write-offs require a symbol + quantity.
--   * stock_dividend is a quantity transformation only: symbol + quantity,
--     but no cash amount (unit_price) — its gross_amount may be zero.
--   * return_of_capital requires an affected symbol and carries a positive
--     cash amount (enforced above), but no quantity/unit_price.
--   * cash flows, interest, standalone fees, and instrument-agnostic income
--     (interest_income, cash_interest, capital_gains_distribution,
--     bond_coupon, staking_reward, other_income) may exist without an
--     instrument symbol.
ALTER TABLE portfolio_activities
    ADD CONSTRAINT portfolio_activity_required_fields CHECK (
        (activity_type IN ('buy', 'sell', 'opening_balance')
            AND symbol IS NOT NULL AND quantity IS NOT NULL AND unit_price IS NOT NULL)
        OR (activity_type IN ('cash_dividend', 'etf_distribution', 'reinvested_dividend', 'symbol_change')
            AND symbol IS NOT NULL)
        OR (activity_type IN ('stock_split', 'reverse_split', 'write_off')
            AND symbol IS NOT NULL AND quantity IS NOT NULL)
        OR (activity_type = 'stock_dividend'
            AND symbol IS NOT NULL AND quantity IS NOT NULL AND unit_price IS NULL)
        OR (activity_type = 'return_of_capital'
            AND symbol IS NOT NULL AND quantity IS NULL AND unit_price IS NULL)
        OR (activity_type IN ('deposit', 'withdrawal', 'interest_income', 'cash_interest',
            'capital_gains_distribution', 'bond_coupon', 'staking_reward', 'other_income',
            'management_fee', 'custody_fee', 'other_fee', 'buy_fee', 'sell_fee'))
    );
