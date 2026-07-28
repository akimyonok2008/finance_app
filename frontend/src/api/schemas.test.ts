import { describe, expect, it } from "vitest";

import {
  assertShape,
  decimalStringSchema,
  positionSummarySchema,
  cashResponseSchema,
  rankedPerformanceHistorySchema,
} from "@/api/schemas";

describe("decimalStringSchema", () => {
  it("passes valid decimal strings", () => {
    expect(decimalStringSchema.safeParse("100.25").success).toBe(true);
    expect(decimalStringSchema.safeParse("-4.2").success).toBe(true);
    expect(decimalStringSchema.safeParse("0").success).toBe(true);
  });

  it("fails bare JSON numbers", () => {
    expect(decimalStringSchema.safeParse(100.25).success).toBe(false);
  });

  it("fails malformed strings and exponent notation", () => {
    expect(decimalStringSchema.safeParse("1,234.56").success).toBe(false);
    expect(decimalStringSchema.safeParse("1e10").success).toBe(false);
    expect(decimalStringSchema.safeParse("NaN").success).toBe(false);
    expect(decimalStringSchema.safeParse("Infinity").success).toBe(false);
  });
});

describe("assertShape", () => {
  it("throws a descriptive error on a contract violation", () => {
    expect(() =>
      assertShape(
        positionSummarySchema,
        { symbol: "AAPL", asset_type: "stock", quantity: 10, baseline_price: "1", currency: "USD" },
        "GET /portfolio/summary",
      ),
    ).toThrow(/Contract violation in GET \/portfolio\/summary/);
  });

  it("passes through a valid response unchanged", () => {
    const valid = {
      symbol: "AAPL",
      asset_type: "stock" as const,
      quantity: "10",
      baseline_price: "189.42",
      currency: "USD",
    };
    expect(assertShape(positionSummarySchema, valid, "test")).toEqual(valid);
  });
});

describe("cashResponseSchema", () => {
  it("rejects a cash balance with a numeric amount", () => {
    const result = cashResponseSchema.safeParse({
      cash_balances: [{ currency: "USD", amount: 100, value_base: "100", weight_percentage: 50 }],
      total_cash_value_base: 100,
      base_currency: "USD",
    });
    expect(result.success).toBe(false);
  });
});

describe("rankedPerformanceHistorySchema", () => {
  it("validates a real point shape", () => {
    const result = rankedPerformanceHistorySchema.safeParse({
      available: true,
      points: [
        {
          captured_at: "2026-07-01T00:00:00Z",
          ranked_index: "104.381920182",
          return_percentage: 4.38,
          drawdown_percentage: -1.2,
          ranking_status: "active",
        },
      ],
      starting_index: "100",
      ending_index: "104.381920182",
      risk: {},
      benchmark: {},
    });
    expect(result.success).toBe(true);
  });
});
