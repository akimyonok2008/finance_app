/**
 * Runtime validation for the critical decimal-string financial API
 * responses. Wired into the fetching layer so a contract violation (a bare
 * JSON number where the backend contract requires a string, a malformed
 * decimal, NaN/Infinity, exponent notation, etc.) fails loudly with a clear
 * error instead of silently corrupting the UI.
 *
 * These schemas intentionally validate only the decimal-typed / structurally
 * important fields, not every field on every DTO — the goal is to catch
 * contract regressions on the fields this migration was about, not to
 * re-implement the whole API surface in Zod.
 */
import { z } from "zod";

/**
 * A backend `money.*` decimal string: optional leading '-', digits, optional
 * '.' followed by digits. No exponent notation, no thousands separators.
 */
export const decimalStringSchema = z
  .string()
  .regex(/^-?\d+(\.\d+)?$/, "expected a canonical decimal string");

/** Same as decimalStringSchema, but the field itself may be absent. */
export const optionalDecimalStringSchema = decimalStringSchema.optional();

export const positionSummarySchema = z.object({
  position_id: z.string().optional(),
  id: z.string().optional(),
  symbol: z.string(),
  asset_type: z.enum(["stock", "etf", "crypto"]),
  quantity: decimalStringSchema,
  baseline_price: decimalStringSchema,
  current_price: optionalDecimalStringSchema,
  cost_basis: optionalDecimalStringSchema,
  current_value: optionalDecimalStringSchema,
  cost_basis_base: optionalDecimalStringSchema,
  current_value_base: optionalDecimalStringSchema,
  gain_loss: optionalDecimalStringSchema,
  gain_loss_base: optionalDecimalStringSchema,
  gain_loss_percentage: z.number().optional(),
  currency: z.string(),
});

export const closedPositionSchema = z.object({
  id: z.string(),
  symbol: z.string(),
  asset_type: z.enum(["stock", "etf", "crypto"]),
  quantity: decimalStringSchema,
  baseline_price: decimalStringSchema,
  close_price: decimalStringSchema,
  realized_gain_loss_base: decimalStringSchema,
  realized_gain_loss_percentage: z.number(),
  closed_cost_basis_base: optionalDecimalStringSchema,
});

export const cashBalanceSchema = z.object({
  currency: z.string(),
  amount: decimalStringSchema,
  value_base: decimalStringSchema,
  weight_percentage: z.number(),
});

export const cashResponseSchema = z.object({
  cash_balances: z.array(cashBalanceSchema),
  total_cash_value_base: z.number(),
  base_currency: z.string(),
});

export const buyPreviewSchema = z.object({
  symbol: z.string(),
  quantity: decimalStringSchema,
  execution_price: decimalStringSchema,
  fee: decimalStringSchema,
  gross_purchase_amount: decimalStringSchema,
  total_cash_required: decimalStringSchema,
  available_cash: decimalStringSchema,
  cash_used: decimalStringSchema,
  automatic_funding_amount: decimalStringSchema,
  remaining_cash: decimalStringSchema,
  resulting_quantity: decimalStringSchema,
  resulting_average_cost: decimalStringSchema,
});

export const sellPreviewSchema = z.object({
  position_id: z.string(),
  symbol: z.string(),
  available_quantity: decimalStringSchema,
  sold_quantity: decimalStringSchema,
  remaining_quantity: decimalStringSchema,
  execution_price: decimalStringSchema,
  gross_proceeds: decimalStringSchema,
  fee: decimalStringSchema,
  net_proceeds: decimalStringSchema,
  allocated_basis: decimalStringSchema,
  estimated_realized_pnl: decimalStringSchema,
});

export const activityMutationResponseSchema = z.object({
  portfolio_version: z.number(),
  ranked_index: decimalStringSchema,
  ranking_status: z.enum(["active", "paused"]),
});

export const rankedHistoryPointSchema = z.object({
  captured_at: z.string(),
  ranked_index: decimalStringSchema,
  return_percentage: z.number(),
  drawdown_percentage: z.number(),
  ranking_status: z.string(),
});

export const rankedPerformanceHistorySchema = z.object({
  available: z.boolean(),
  points: z.array(rankedHistoryPointSchema),
  starting_index: optionalDecimalStringSchema,
  ending_index: optionalDecimalStringSchema,
});

export const leaderboardEntrySchema = z.object({
  rank: z.number(),
  display_name: z.string(),
  ranked_index: decimalStringSchema,
  ranked_return_percentage: decimalStringSchema,
});

/**
 * Partial-shape check for GET /portfolio/summary. Only validates the
 * decimal-string-bearing sub-arrays (positions, closed_positions,
 * cash_balances) — the many float64 aggregate fields documented as
 * intentionally-still-`number` are left unchecked (`.passthrough()`) so this
 * schema doesn't need to duplicate the entire, still-evolving aggregate DTO.
 */
export const portfolioSummaryShapeSchema = z
  .object({
    positions: z.array(positionSummarySchema).optional(),
    closed_positions: z.array(closedPositionSchema).optional(),
    cash_balances: z.array(cashBalanceSchema).optional(),
  })
  .passthrough();

export const leaderboardStandingSchema = z.object({
  eligible: z.boolean(),
  ranked_index: decimalStringSchema,
  ranked_return_percentage: decimalStringSchema,
  percentile: z.number(),
});

/**
 * Validates `data` against `schema`. On failure, throws a descriptive Error
 * (rather than returning a coerced/partial value) so the caller sees a
 * clear, visible failure instead of silently rendering corrupted data.
 * `context` names the endpoint/response for the error message.
 */
export function assertShape<T>(schema: z.ZodType<T>, data: unknown, context: string): T {
  const result = schema.safeParse(data);
  if (!result.success) {
    throw new Error(
      `Contract violation in ${context}: ${result.error.issues
        .map((issue) => `${issue.path.join(".") || "(root)"}: ${issue.message}`)
        .join("; ")}`,
    );
  }
  return result.data;
}
