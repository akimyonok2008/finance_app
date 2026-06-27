import type { CoachObservationStatus, CoachRiskLevel } from "@/types/coach";

/** Restrained badge classes per observation status. Never color-only — the
 * status word is always rendered as text alongside. */
export function statusBadgeClass(status: string): string {
  switch (status as CoachObservationStatus) {
    case "positive":
      return "border-emerald-500/20 bg-emerald-500/5 text-emerald-300";
    case "watch":
      return "border-amber-500/20 bg-amber-500/5 text-amber-300";
    case "risk":
      return "border-rose-500/20 bg-rose-500/5 text-rose-300";
    case "neutral":
    default:
      return "border-zinc-700 bg-zinc-800/40 text-zinc-400";
  }
}

/** Map a risk level to a restrained badge tone. */
export function riskBadgeClass(risk: string | undefined): string {
  switch (risk as CoachRiskLevel) {
    case "low":
      return "border-emerald-500/20 bg-emerald-500/5 text-emerald-300";
    case "elevated":
      return "border-amber-500/20 bg-amber-500/5 text-amber-300";
    case "high":
      return "border-rose-500/20 bg-rose-500/5 text-rose-300";
    case "moderate":
    case "unknown":
    default:
      return "border-zinc-700 bg-zinc-800/40 text-zinc-400";
  }
}
