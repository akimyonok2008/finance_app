/**
 * Tailwind text-color class for a gain/loss value.
 * Positive -> emerald, negative -> rose, zero/unknown -> slate.
 * Color is never the only signal — values are always shown with a +/- sign too.
 */
export function gainLossColor(value: number | undefined | null): string {
  if (value === undefined || value === null || !Number.isFinite(value)) {
    return "text-slate-400";
  }
  if (value > 0) return "text-emerald-400";
  if (value < 0) return "text-rose-400";
  return "text-slate-300";
}
