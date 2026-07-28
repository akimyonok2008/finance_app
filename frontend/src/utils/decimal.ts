/**
 * Decimal-string helpers for the backend's `money` package contract.
 *
 * The backend serializes authoritative financial values (Amount, Quantity,
 * Price, Ratio, IndexValue, Weight) as canonical decimal strings, e.g.
 * `"12.5"`, `"100.25"`, `"104.381920182"`. These strings can exceed the
 * precision/range of a JS `number`, so they must never be parsed with
 * `parseFloat`/`Number` for arithmetic or comparison — only the sanctioned
 * `decimalToChartNumber` boundary may convert to `number`, and only for
 * chart/visualization display.
 *
 * The backend never emits leading zeros (other than a single `0` before the
 * decimal point), never emits exponent notation, and always uses `.` as the
 * decimal separator with an optional leading `-`. See
 * backend/internal/money/money.go for the canonical String() formatting.
 */

export type DecimalString = string;
export type MoneyString = DecimalString;
export type QuantityString = DecimalString;
export type PriceString = DecimalString;
export type RatioString = DecimalString;
export type IndexString = DecimalString;

/** Strict canonical form: optional '-', digits, optional '.' + digits. No exponents, no commas. */
const CANONICAL_DECIMAL_RE = /^-?\d+(\.\d+)?$/;

/**
 * Looser form accepted while validating *input* that may not yet be
 * canonicalized (e.g. a value round-tripped through JSON as a bare number,
 * or a value with insignificant leading zeros). Still rejects exponents,
 * commas, whitespace, Infinity/NaN spellings.
 */
const LOOSE_DECIMAL_RE = /^-?\d+(\.\d+)?$/;

/**
 * True if `value` is a syntactically valid decimal string: an optional
 * leading '-', one or more digits, an optional '.' followed by one or more
 * digits. Rejects exponent notation, thousands separators (e.g. "1,234.56"),
 * empty string, whitespace, "NaN", "Infinity", and non-string input.
 */
export function isValidDecimalString(value: unknown): value is DecimalString {
  if (typeof value !== "string" || value.length === 0) return false;
  if (!LOOSE_DECIMAL_RE.test(value)) return false;
  return true;
}

/** True if `value` matches the backend's exact canonical String() form (no redundant leading/trailing zeros beyond one digit before '.'). */
export function isCanonicalDecimalString(value: unknown): value is DecimalString {
  if (typeof value !== "string" || !CANONICAL_DECIMAL_RE.test(value)) return false;
  const unsigned = value.startsWith("-") ? value.slice(1) : value;
  const [intPart] = unsigned.split(".");
  if (intPart.length > 1 && intPart.startsWith("0")) return false;
  return true;
}

/** Splits a validated decimal string into sign, integer digits, fractional digits. */
function splitDecimal(value: DecimalString): { negative: boolean; int: string; frac: string } {
  const negative = value.startsWith("-");
  const unsigned = negative ? value.slice(1) : value;
  const [intPart, fracPart = ""] = unsigned.split(".");
  return { negative, int: intPart.replace(/^0+(?=\d)/, ""), frac: fracPart };
}

/**
 * Compares two decimal strings exactly, without float coercion / precision
 * loss. Returns -1, 0, or 1, suitable for Array.prototype.sort.
 * Throws if either value is not a valid decimal string.
 */
export function compareDecimal(a: DecimalString, b: DecimalString): number {
  if (!isValidDecimalString(a) || !isValidDecimalString(b)) {
    throw new Error(`compareDecimal: invalid decimal string ("${a}", "${b}")`);
  }
  const da = splitDecimal(a);
  const db = splitDecimal(b);

  // Normalize zero sign: "-0" / "-0.00" behave as zero.
  const isZero = (d: ReturnType<typeof splitDecimal>) =>
    (d.int === "" || /^0*$/.test(d.int)) && /^0*$/.test(d.frac);
  const aZero = isZero(da);
  const bZero = isZero(db);
  if (aZero && bZero) return 0;

  const aNeg = da.negative && !aZero;
  const bNeg = db.negative && !bZero;
  if (aNeg !== bNeg) return aNeg ? -1 : 1;

  const fracLen = Math.max(da.frac.length, db.frac.length);
  const aDigits = (da.int || "0") + da.frac.padEnd(fracLen, "0");
  const bDigits = (db.int || "0") + db.frac.padEnd(fracLen, "0");
  const aTrim = aDigits.replace(/^0+(?=\d)/, "");
  const bTrim = bDigits.replace(/^0+(?=\d)/, "");

  let cmp: number;
  if (aTrim.length !== bTrim.length) {
    cmp = aTrim.length - bTrim.length;
  } else {
    cmp = aTrim < bTrim ? -1 : aTrim > bTrim ? 1 : 0;
  }
  return aNeg ? -cmp : cmp;
}

/**
 * The ONLY sanctioned way to turn a DecimalString into a JS number, for
 * chart/visualization use. Rejects non-finite results explicitly by
 * returning null so callers can render an "unavailable" state instead of
 * silently drawing corrupted data.
 */
export function decimalToChartNumber(value: DecimalString | null | undefined): number | null {
  if (value === null || value === undefined) return null;
  if (!isValidDecimalString(value)) return null;
  const n = Number(value);
  return Number.isFinite(n) ? n : null;
}

/** Currency minor-unit scale table. Extend as new currencies are supported. */
const CURRENCY_DECIMALS: Record<string, number> = {
  USD: 2,
  EUR: 2,
  GBP: 2,
  TRY: 2,
  JPY: 0,
};

export function currencyDecimals(currency: string | undefined): number {
  if (!currency) return 2;
  return CURRENCY_DECIMALS[currency.toUpperCase()] ?? 2;
}

/** Removes a rendered "-0" / "-0.00" and returns the non-negative form. */
function suppressNegativeZero(formatted: string): string {
  return formatted.replace(/^-(0(\.0+)?)$/, "$1").replace(/^-(\$?0(\.0+)?)$/, "$1");
}

// NOTE: formatMoney/formatPercent live in ./formatMoney.ts and
// ./formatPercent.ts (pre-existing, now decimal-string aware) to avoid two
// competing formatters. This module keeps the lower-level building blocks
// (validation, comparison, currency scale table, chart boundary) plus
// formatQuantity/formatPrice which had no prior home.

/** Formats a decimal-string quantity (fractional shares, crypto, etc.) without forcing a fixed scale. */
export function formatQuantity(value: DecimalString | null | undefined): string {
  if (value === null || value === undefined || !isValidDecimalString(value)) return "—";
  const n = Number(value);
  if (!Number.isFinite(n)) return "—";
  const formatted = new Intl.NumberFormat("en-US", {
    maximumFractionDigits: 8,
  }).format(n === 0 ? 0 : n);
  return suppressNegativeZero(formatted);
}

/** Formats a decimal-string unit price with currency-aware decimals. */
export function formatPrice(
  value: DecimalString | null | undefined,
  currency = "USD",
): string {
  if (value === null || value === undefined || !isValidDecimalString(value)) return "—";
  const n = Number(value);
  if (!Number.isFinite(n)) return "—";
  const digits = currencyDecimals(currency);
  const formatted = new Intl.NumberFormat("en-US", {
    style: "currency",
    currency,
    minimumFractionDigits: digits,
    maximumFractionDigits: digits,
  }).format(n === 0 ? 0 : n);
  return suppressNegativeZero(formatted);
}

/**
 * Builds a decimal-typed request payload field from raw user input,
 * validating it first. Returns null if the input is not a valid decimal
 * (callers should treat that as a form validation error, not send it).
 */
export function toDecimalPayload(rawInput: string): DecimalString | null {
  const trimmed = rawInput.trim();
  if (!isValidDecimalString(trimmed)) return null;
  return trimmed;
}
