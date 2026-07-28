import { describe, expect, it } from "vitest";

import {
  compareDecimal,
  currencyDecimals,
  decimalToChartNumber,
  isCanonicalDecimalString,
  isValidDecimalString,
} from "@/utils/decimal";
import { formatMoney } from "@/utils/formatMoney";
import { formatPercent } from "@/utils/formatPercent";

describe("isValidDecimalString", () => {
  it("accepts canonical decimal strings", () => {
    expect(isValidDecimalString("12.5")).toBe(true);
    expect(isValidDecimalString("0")).toBe(true);
    expect(isValidDecimalString("-4.2")).toBe(true);
    expect(isValidDecimalString("104.381920182")).toBe(true);
  });

  it("rejects malformed strings", () => {
    expect(isValidDecimalString("")).toBe(false);
    expect(isValidDecimalString("abc")).toBe(false);
    expect(isValidDecimalString("1,234.56")).toBe(false);
    expect(isValidDecimalString("1.2.3")).toBe(false);
    expect(isValidDecimalString(".5")).toBe(false);
    expect(isValidDecimalString("5.")).toBe(false);
  });

  it("rejects exponent notation", () => {
    expect(isValidDecimalString("1e10")).toBe(false);
    expect(isValidDecimalString("1E-5")).toBe(false);
  });

  it("rejects NaN/Infinity spellings and bare JSON numbers", () => {
    expect(isValidDecimalString("NaN")).toBe(false);
    expect(isValidDecimalString("Infinity")).toBe(false);
    expect(isValidDecimalString(123.45)).toBe(false);
    expect(isValidDecimalString(null)).toBe(false);
    expect(isValidDecimalString(undefined)).toBe(false);
  });
});

describe("isCanonicalDecimalString", () => {
  it("rejects redundant leading zeros", () => {
    expect(isCanonicalDecimalString("0.5")).toBe(true);
    expect(isCanonicalDecimalString("00.5")).toBe(false);
    expect(isCanonicalDecimalString("01")).toBe(false);
  });
});

describe("compareDecimal", () => {
  it("compares without float precision loss", () => {
    expect(compareDecimal("100.25", "100.25")).toBe(0);
    expect(compareDecimal("100.3", "100.25")).toBeGreaterThan(0);
    expect(compareDecimal("99.9", "100")).toBeLessThan(0);
  });

  it("handles very large / high-precision values lexicographic sort would break", () => {
    expect(compareDecimal("999999999999999999999.1", "1000000000000000000000")).toBeLessThan(0);
    expect(compareDecimal("104.381920182", "104.38192018199")).toBeGreaterThan(0);
  });

  it("treats -0 and 0 as equal", () => {
    expect(compareDecimal("-0", "0")).toBe(0);
    expect(compareDecimal("-0.00", "0")).toBe(0);
  });

  it("handles negative comparisons", () => {
    expect(compareDecimal("-5", "-2")).toBeLessThan(0);
    expect(compareDecimal("-2", "-5")).toBeGreaterThan(0);
  });

  it("sorts an array correctly via Array.prototype.sort", () => {
    const values = ["10", "2", "100.5", "-3", "0"];
    const sorted = [...values].sort(compareDecimal);
    expect(sorted).toEqual(["-3", "0", "2", "10", "100.5"]);
  });
});

describe("decimalToChartNumber", () => {
  it("converts a valid decimal string", () => {
    expect(decimalToChartNumber("104.381920182")).toBeCloseTo(104.381920182);
  });

  it("returns null for non-finite / malformed / missing input", () => {
    expect(decimalToChartNumber("abc")).toBeNull();
    expect(decimalToChartNumber(undefined)).toBeNull();
    expect(decimalToChartNumber(null)).toBeNull();
    expect(decimalToChartNumber("1e10")).toBeNull();
  });
});

describe("currencyDecimals", () => {
  it("returns 2 for common currencies and 0 for JPY", () => {
    expect(currencyDecimals("USD")).toBe(2);
    expect(currencyDecimals("EUR")).toBe(2);
    expect(currencyDecimals("GBP")).toBe(2);
    expect(currencyDecimals("JPY")).toBe(0);
  });
});

describe("formatMoney (decimal-string aware)", () => {
  it("formats fractional quantities/amounts correctly", () => {
    expect(formatMoney("100.25", "USD")).toBe("$100.25");
  });

  it("never renders -0.00", () => {
    expect(formatMoney("-0", "USD")).not.toContain("-");
    expect(formatMoney("-0.00", "USD")).not.toContain("-");
  });

  it("supports JPY with zero decimal places", () => {
    expect(formatMoney("1500", "JPY")).toBe("¥1,500");
  });

  it("handles large values beyond safe integer range without throwing", () => {
    expect(() => formatMoney("999999999999999999999.99", "USD")).not.toThrow();
  });

  it("preserves negative signs for real negatives", () => {
    expect(formatMoney("-42.50", "USD")).toContain("-");
  });

  it("returns an em dash for invalid/missing input", () => {
    expect(formatMoney(undefined)).toBe("—");
    expect(formatMoney(null)).toBe("—");
    expect(formatMoney("not-a-number")).toBe("—");
  });

  it("still accepts plain numbers (legacy float64 aggregate fields)", () => {
    expect(formatMoney(100.25, "USD")).toBe("$100.25");
  });
});

describe("formatPercent (decimal-string aware)", () => {
  it("accepts a DecimalString", () => {
    expect(formatPercent("8.33")).toBe("+8.33%");
  });

  it("accepts a plain number", () => {
    expect(formatPercent(-4.12)).toBe("-4.12%");
  });

  it("never renders -0.00%", () => {
    expect(formatPercent("-0")).toBe("0.00%");
    expect(formatPercent(-0)).toBe("0.00%");
  });

  it("returns an em dash for invalid input", () => {
    expect(formatPercent(undefined)).toBe("—");
    expect(formatPercent("garbage")).toBe("—");
  });
});
