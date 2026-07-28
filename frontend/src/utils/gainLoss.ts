import { compareDecimal, isValidDecimalString } from "@/utils/decimal";

/**
 * Tailwind text-color class for a gain/loss value.
 * Positive -> emerald, negative -> rose, zero/unknown -> slate.
 * Color is never the only signal — values are always shown with a +/- sign too.
 *
 * Accepts either a plain `number` (the documented float64 presentation
 * fields, e.g. `*_percentage`) or a `DecimalString` (authoritative money
 * fields) — comparison against zero uses `compareDecimal` for the string
 * case so precision is never lost.
 */
export function gainLossColor(value: number | string | undefined | null): string {
  if (value === undefined || value === null) return "text-slate-400";
  if (typeof value === "number") {
    if (!Number.isFinite(value)) return "text-slate-400";
    if (value > 0) return "text-emerald-400";
    if (value < 0) return "text-rose-400";
    return "text-slate-300";
  }
  if (!isValidDecimalString(value)) return "text-slate-400";
  const cmp = compareDecimal(value, "0");
  if (cmp > 0) return "text-emerald-400";
  if (cmp < 0) return "text-rose-400";
  return "text-slate-300";
}
