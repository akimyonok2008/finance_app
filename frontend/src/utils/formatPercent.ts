import { isValidDecimalString } from "@/utils/decimal";

/**
 * Format a percentage value with an explicit sign so gain/loss never relies on
 * color alone (accessibility). Returns an em dash for nullish/non-finite input.
 *
 * Examples: 8.33 -> "+8.33%", -4.12 -> "-4.12%", 0 -> "0.00%", null -> "—".
 *
 * Accepts either a `number` (the documented float64 presentation fields) or a
 * `DecimalString` (authoritative ranked/return percentage fields converted to
 * decimal strings, e.g. `ranked_return_percentage`). The string form is only
 * ever parsed at this formatting boundary, never for arithmetic.
 */
export function formatPercent(value: number | string | undefined | null): string {
  let n: number;
  if (value === undefined || value === null) return "—";
  if (typeof value === "number") {
    n = value;
  } else {
    if (!isValidDecimalString(value)) return "—";
    n = Number(value);
  }
  if (!Number.isFinite(n)) return "—";

  const sign = n > 0 ? "+" : n < 0 ? "-" : "";
  return `${sign}${Math.abs(n).toFixed(2)}%`;
}
