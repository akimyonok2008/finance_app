import { describe, expect, it } from "vitest";

import { assertShape, positionSummarySchema, rankedHistoryPointSchema } from "@/api/schemas";
import { formatMoney } from "@/utils/formatMoney";
import { decimalToChartNumber } from "@/utils/decimal";
import { formatQuantity } from "@/utils/decimal";

/**
 * Regression fixture: `{"amount":"100.25","quantity":"2.5","ranked_index":"104.381920182"}`
 * proves the decimal-string contract is handled end-to-end without a
 * toFixed-style runtime error — validated, formatted, and chart-converted.
 */
describe("decimal-string contract regression fixture", () => {
  const fixture = {
    amount: "100.25",
    quantity: "2.5",
    ranked_index: "104.381920182",
  };

  it("validates against the runtime schemas", () => {
    const position = assertShape(
      positionSummarySchema,
      {
        symbol: "AAPL",
        asset_type: "stock",
        quantity: fixture.quantity,
        baseline_price: fixture.amount,
        currency: "USD",
      },
      "regression fixture",
    );
    expect(position.quantity).toBe("2.5");

    const point = assertShape(
      rankedHistoryPointSchema,
      {
        captured_at: "2026-07-01T00:00:00Z",
        ranked_index: fixture.ranked_index,
        return_percentage: 4.38,
        drawdown_percentage: 0,
        ranking_status: "active",
      },
      "regression fixture",
    );
    expect(point.ranked_index).toBe("104.381920182");
  });

  it("formats without throwing and without precision-losing coercion", () => {
    expect(formatMoney(fixture.amount, "USD")).toBe("$100.25");
    expect(formatQuantity(fixture.quantity)).toBe("2.5");
  });

  it("converts ranked_index to a chart number only at the chart boundary", () => {
    const chartValue = decimalToChartNumber(fixture.ranked_index);
    expect(chartValue).toBeCloseTo(104.381920182);
  });
});
