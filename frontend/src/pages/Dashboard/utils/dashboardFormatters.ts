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

export function getPerformanceTone(
  value: number | undefined | null,
): PerformanceTone {
  if (value === undefined || value === null || !Number.isFinite(value)) {
    return "neutral";
  }
  if (value > 0) return "positive";
  if (value < 0) return "negative";
  return "neutral";
}
