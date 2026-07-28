-- Financial precision hardening.
--
-- The migration runner executes each file in one transaction. Existing
-- NUMERIC values are widened without a float conversion. The two legacy
-- DOUBLE PRECISION financial columns are checked for non-finite values before
-- their deterministic numeric casts. Historical binary-float rounding already
-- present in those rows cannot be reconstructed by this migration.

DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM market_quotes
        WHERE price::text IN ('NaN', 'Infinity', '-Infinity')
           OR previous_close::text IN ('NaN', 'Infinity', '-Infinity')
    ) THEN
        RAISE EXCEPTION 'market_quotes contains NaN/Infinity; quarantine those rows before migration';
    END IF;

    IF EXISTS (
        SELECT 1 FROM leaderboard_snapshots
        WHERE portfolio_index::text IN ('NaN', 'Infinity', '-Infinity')
    ) THEN
        RAISE EXCEPTION 'leaderboard_snapshots contains NaN/Infinity; quarantine those rows before migration';
    END IF;
END
$$;

ALTER TABLE market_quotes
    ALTER COLUMN price TYPE NUMERIC(36, 12)
        USING ROUND(price::NUMERIC, 12),
    ALTER COLUMN previous_close TYPE NUMERIC(36, 12)
        USING ROUND(previous_close::NUMERIC, 12);

ALTER TABLE leaderboard_snapshots
    ALTER COLUMN portfolio_index TYPE NUMERIC(38, 18)
        USING ROUND(portfolio_index::NUMERIC, 18);

ALTER TABLE positions
    ALTER COLUMN quantity TYPE NUMERIC(38, 18) USING quantity::NUMERIC(38, 18),
    ALTER COLUMN average_buy_price TYPE NUMERIC(36, 12) USING average_buy_price::NUMERIC(36, 12),
    ALTER COLUMN close_price TYPE NUMERIC(36, 12) USING close_price::NUMERIC(36, 12),
    ALTER COLUMN realized_gain_loss_base TYPE NUMERIC(38, 18) USING realized_gain_loss_base::NUMERIC(38, 18);

ALTER TABLE portfolio_cash_balances
    ALTER COLUMN amount TYPE NUMERIC(38, 18) USING amount::NUMERIC(38, 18);

ALTER TABLE portfolio_activities
    ALTER COLUMN quantity TYPE NUMERIC(38, 18) USING quantity::NUMERIC(38, 18),
    ALTER COLUMN unit_price TYPE NUMERIC(36, 12) USING unit_price::NUMERIC(36, 12),
    ALTER COLUMN gross_amount TYPE NUMERIC(38, 18) USING gross_amount::NUMERIC(38, 18),
    ALTER COLUMN fee_amount TYPE NUMERIC(38, 18) USING fee_amount::NUMERIC(38, 18),
    ALTER COLUMN net_amount TYPE NUMERIC(38, 18) USING net_amount::NUMERIC(38, 18),
    ALTER COLUMN cost_basis_allocated TYPE NUMERIC(38, 18) USING cost_basis_allocated::NUMERIC(38, 18),
    ALTER COLUMN realized_gain_loss_base TYPE NUMERIC(38, 18) USING realized_gain_loss_base::NUMERIC(38, 18);

ALTER TABLE ranked_performance_state
    ALTER COLUMN checkpoint_index TYPE NUMERIC(38, 18) USING checkpoint_index::NUMERIC(38, 18),
    ALTER COLUMN segment_start_value_base TYPE NUMERIC(38, 18) USING segment_start_value_base::NUMERIC(38, 18);

ALTER TABLE ranked_performance_snapshots
    ALTER COLUMN ranked_index TYPE NUMERIC(38, 18) USING ranked_index::NUMERIC(38, 18);

ALTER TABLE portfolio_mutation_audit
    ALTER COLUMN ranked_index_before TYPE NUMERIC(38, 18) USING ranked_index_before::NUMERIC(38, 18),
    ALTER COLUMN ranked_index_after TYPE NUMERIC(38, 18) USING ranked_index_after::NUMERIC(38, 18);

ALTER TABLE portfolio_outbox
    ALTER COLUMN ranked_index TYPE NUMERIC(38, 18) USING ranked_index::NUMERIC(38, 18);

ALTER TABLE portfolio_archive_snapshots
    ALTER COLUMN portfolio_index TYPE NUMERIC(38, 18) USING portfolio_index::NUMERIC(38, 18),
    ALTER COLUMN total_cost_basis TYPE NUMERIC(38, 18) USING total_cost_basis::NUMERIC(38, 18),
    ALTER COLUMN current_value TYPE NUMERIC(38, 18) USING current_value::NUMERIC(38, 18),
    ALTER COLUMN unrealized_gain_loss_base TYPE NUMERIC(38, 18) USING unrealized_gain_loss_base::NUMERIC(38, 18),
    ALTER COLUMN realized_gain_loss_base TYPE NUMERIC(38, 18) USING realized_gain_loss_base::NUMERIC(38, 18),
    ALTER COLUMN total_cash_value_base TYPE NUMERIC(38, 18) USING total_cash_value_base::NUMERIC(38, 18);

ALTER TABLE competition_entries
    ALTER COLUMN starting_value_base TYPE NUMERIC(38, 18) USING starting_value_base::NUMERIC(38, 18),
    ALTER COLUMN starting_index TYPE NUMERIC(38, 18) USING starting_index::NUMERIC(38, 18);

ALTER TABLE competition_entry_snapshot_positions
    ALTER COLUMN quantity TYPE NUMERIC(38, 18) USING quantity::NUMERIC(38, 18),
    ALTER COLUMN starting_price TYPE NUMERIC(36, 12) USING starting_price::NUMERIC(36, 12),
    ALTER COLUMN starting_value_base TYPE NUMERIC(38, 18) USING starting_value_base::NUMERIC(38, 18);

ALTER TABLE income_events
    ALTER COLUMN amount_per_unit TYPE NUMERIC(38, 18) USING amount_per_unit::NUMERIC(38, 18);

ALTER TABLE income_event_applications
    ALTER COLUMN eligible_quantity TYPE NUMERIC(38, 18) USING eligible_quantity::NUMERIC(38, 18),
    ALTER COLUMN gross_amount TYPE NUMERIC(38, 18) USING gross_amount::NUMERIC(38, 18),
    ALTER COLUMN withholding_amount TYPE NUMERIC(38, 18) USING withholding_amount::NUMERIC(38, 18),
    ALTER COLUMN fee_amount TYPE NUMERIC(38, 18) USING fee_amount::NUMERIC(38, 18),
    ALTER COLUMN net_amount TYPE NUMERIC(38, 18) USING net_amount::NUMERIC(38, 18),
    ALTER COLUMN reinvestment_quantity TYPE NUMERIC(38, 18) USING reinvestment_quantity::NUMERIC(38, 18);
