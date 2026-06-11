import type { LeaderboardEntry } from "@/types/leaderboard";

const STRATEGIES = [
  "Tech Heavy",
  "Crypto Maxi",
  "Dividend Focus",
  "Balanced",
  "BIST Watcher",
  "ETF Core",
  "High Momentum",
  "Defensive",
] as const;

export function formatSignedPercent(value: number | null | undefined): string {
  if (value === null || value === undefined || !Number.isFinite(value)) {
    return "—";
  }
  if (value > 0) return `+${value.toFixed(2)}%`;
  return `${value.toFixed(2)}%`;
}

export function getPercentTone(
  value: number | null | undefined,
): "positive" | "negative" | "neutral" {
  if (value === null || value === undefined || !Number.isFinite(value)) {
    return "neutral";
  }
  if (value > 0) return "positive";
  if (value < 0) return "negative";
  return "neutral";
}

export function getPercentClassName(
  value: number | null | undefined,
): string {
  const tone = getPercentTone(value);
  if (tone === "positive") {
    return "border-emerald-500/20 bg-emerald-500/10 text-emerald-400";
  }
  if (tone === "negative") {
    return "border-rose-500/20 bg-rose-500/10 text-rose-400";
  }
  return "border-zinc-700 bg-zinc-800/60 text-zinc-400";
}

export function getAvatarSymbol(avatarKey?: string): string {
  const avatars: Record<string, string> = {
    fox: "🦊",
    bull: "🐂",
    rocket: "🚀",
    bear: "◐",
    default: "◐",
  };
  return avatars[avatarKey?.toLowerCase() ?? "default"] ?? "◐";
}

export function getStrategyFallback(displayName: string, rank: number): string {
  let hash = rank;
  for (const char of displayName) hash = (hash * 31 + char.charCodeAt(0)) >>> 0;
  return STRATEGIES[hash % STRATEGIES.length];
}

export function isEntryVisible(
  entries: LeaderboardEntry[],
  me: LeaderboardEntry,
): boolean {
  return entries.some((entry) => entry.is_me || entry.rank === me.rank);
}
