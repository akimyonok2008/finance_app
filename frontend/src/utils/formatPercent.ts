/**
 * Format a percentage value with an explicit sign so gain/loss never relies on
 * color alone (accessibility). Returns an em dash for nullish/non-finite input.
 *
 * Examples: 8.33 -> "+8.33%", -4.12 -> "-4.12%", 0 -> "0.00%", null -> "—".
 */
export function formatPercent(value: number | undefined | null): string {
  if (value === undefined || value === null || !Number.isFinite(value)) {
    return "—";
  }

  const sign = value > 0 ? "+" : value < 0 ? "-" : "";
  return `${sign}${Math.abs(value).toFixed(2)}%`;
}
