import type { PerformanceTone } from "@/types/dashboard";

export function formatRank(rank: number | null | undefined): string {
  if (rank === null || rank === undefined) return "—";
  return `#${rank}`;
}

export function formatParticipants(count: number | null | undefined): string {
  if (count === null || count === undefined) return "—";
  return `of ${count.toLocaleString()} investors`;
}

export function getDaysRemaining(endsAt: string | undefined): number | null {
  if (!endsAt) return null;
  const end = new Date(endsAt).getTime();
  const now = Date.now();
  if (Number.isNaN(end)) return null;
  return Math.max(0, Math.ceil((end - now) / (1000 * 60 * 60 * 24)));
}

/**
 * Accepts either a plain `number` (e.g. a percentage/ratio field) or a
 * decimal-string money field. The string form is only parsed here, at this
 * display boundary, never for arithmetic.
 */
export function getPerformanceTone(
  value: number | string | undefined | null,
): PerformanceTone {
  const n = typeof value === "string" ? Number(value) : value;
  if (n === undefined || n === null || !Number.isFinite(n)) {
    return "neutral";
  }
  if (n > 0) return "positive";
  if (n < 0) return "negative";
  return "neutral";
}
