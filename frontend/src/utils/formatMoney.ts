/**
 * Format a numeric value as a currency string using the Intl API.
 * Returns an em dash for nullish/non-finite values so the UI never renders NaN.
 */
export function formatMoney(
  value: number | undefined | null,
  currency = "USD",
): string {
  if (value === undefined || value === null || !Number.isFinite(value)) {
    return "—";
  }

  try {
    return new Intl.NumberFormat("en-US", {
      style: "currency",
      currency,
      minimumFractionDigits: 2,
      maximumFractionDigits: 2,
    }).format(value);
  } catch {
    // Unknown/invalid currency code — fall back to a plain number + code.
    return `${value.toFixed(2)} ${currency}`;
  }
}
