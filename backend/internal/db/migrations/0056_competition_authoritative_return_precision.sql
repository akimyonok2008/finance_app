-- Competition return is authoritative ranking data, not a display value.
-- The original NUMERIC(12,4) columns silently rounded before ROW_NUMBER()
-- sorted a generation. Unconstrained NUMERIC preserves the exact decimal
-- emitted by the fixed-point valuation engine; clients round only for display.
ALTER TABLE competition_rankings
    ALTER COLUMN return_percentage TYPE NUMERIC;

ALTER TABLE competition_finalization_rows
    ALTER COLUMN return_percentage TYPE NUMERIC;

ALTER TABLE competition_results
    ALTER COLUMN final_return_percentage TYPE NUMERIC;
