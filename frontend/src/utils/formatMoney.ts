import { currencyDecimals, isValidDecimalString } from "@/utils/decimal";

/**
 * Format a value as a currency string using the Intl API.
 * Returns an em dash for nullish/non-finite/invalid values so the UI never
 * renders NaN.
 *
 * Accepts either a `number` (the documented float64 aggregate fields, e.g.
 * PortfolioSummary totals) or a `DecimalString` (authoritative per-position
 * money fields). The string form is only parsed at this formatting
 * boundary — never for arithmetic.
 */
export function formatMoney(
  value: number | string | undefined | null,
  currency = "USD",
): string {
  let n: number;
  if (value === undefined || value === null) return "—";
  if (typeof value === "number") {
    n = value;
  } else {
    if (!isValidDecimalString(value)) return "—";
    n = Number(value);
  }
  if (!Number.isFinite(n)) return "—";

  const digits = currencyDecimals(currency);
  try {
    const formatted = new Intl.NumberFormat("en-US", {
      style: "currency",
      currency,
      minimumFractionDigits: digits,
      maximumFractionDigits: digits,
    }).format(n === 0 ? 0 : n);
    // Never render "-0.00" / "-$0.00".
    return formatted.replace(/^(-)(\D*0(\.0+)?)$/, "$2");
  } catch {
    // Unknown/invalid currency code — fall back to a plain number + code.
    return `${(n === 0 ? 0 : n).toFixed(digits)} ${currency}`;
  }
}
