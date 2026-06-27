import { apiRequest } from "@/api/client";
import type { ProfileBadge } from "@/types/profile";
import type {
  LeaderboardEntry,
  LeaderboardGlobalResponse,
  LeaderboardQueryParams,
  LeaderboardStanding,
  PublicAssetType,
  PublicWeight,
} from "@/types/leaderboard";

function numberValue(value: unknown): number {
  const parsed = Number(value);
  return Number.isFinite(parsed) ? parsed : 0;
}

function publicWeights(value: unknown): PublicWeight[] {
  if (!Array.isArray(value)) return [];
  return value
    .filter((item): item is Record<string, unknown> => Boolean(item) && typeof item === "object")
    .map((item) => ({
      symbol: String(item.symbol ?? "").toUpperCase(),
      asset_type: String(item.asset_type ?? "other") as PublicAssetType,
      weight_percentage: numberValue(item.weight_percentage),
    }))
    .filter((item) => item.symbol.length > 0);
}

function badges(value: unknown): ProfileBadge[] {
  if (!Array.isArray(value)) return [];
  return value
    .filter((item): item is Record<string, unknown> => Boolean(item) && typeof item === "object")
    .map((item) => ({
      key: String(item.key ?? ""),
      name: String(item.name ?? ""),
      icon_key: item.icon_key ? String(item.icon_key) : undefined,
      unlocked_at: item.unlocked_at ? String(item.unlocked_at) : undefined,
    }))
    .filter((item) => item.key.length > 0);
}

function normalizeEntry(raw: Record<string, unknown>): LeaderboardEntry {
  return {
    rank: numberValue(raw.rank),
    display_name: String(raw.display_name ?? "Anonymous investor"),
    handle: raw.handle ? String(raw.handle) : undefined,
    avatar_key: raw.avatar_key ? String(raw.avatar_key) : undefined,
    strategy_tag: raw.strategy_tag ? String(raw.strategy_tag) : undefined,
    ranked_index: numberValue(raw.ranked_index ?? raw.portfolio_index ?? 100),
    ranked_return_percentage: numberValue(
      raw.ranked_return_percentage ?? raw.gain_loss_percentage,
    ),
    public_weights: publicWeights(raw.public_weights),
    badges: badges(raw.badges),
    is_me: Boolean(raw.is_me),
  };
}

export async function getGlobalLeaderboard(
  params: LeaderboardQueryParams,
): Promise<LeaderboardGlobalResponse> {
  const response = await apiRequest<unknown>(
    `/leaderboard?timeframe=${encodeURIComponent(params.timeframe)}`,
  );
  // The endpoint returns a bare array; tolerate an {entries:[...]} envelope too.
  const rawEntries = Array.isArray(response)
    ? response
    : response && typeof response === "object" && Array.isArray((response as Record<string, unknown>).entries)
      ? ((response as Record<string, unknown>).entries as unknown[])
      : [];
  return {
    timeframe: params.timeframe,
    entries: rawEntries
      .filter((item): item is Record<string, unknown> => Boolean(item) && typeof item === "object")
      .map(normalizeEntry),
  };
}

export async function getLeaderboardStanding(
  timeframe: LeaderboardQueryParams["timeframe"],
  signal?: AbortSignal,
): Promise<LeaderboardStanding> {
  const raw = await apiRequest<Record<string, unknown>>(
    `/leaderboard/me?timeframe=${encodeURIComponent(timeframe)}`,
    { signal },
  );
  return {
    timeframe,
    eligible: Boolean(raw.eligible),
    rank: raw.rank === null || raw.rank === undefined ? null : numberValue(raw.rank),
    total_participants: numberValue(raw.total_participants),
    ranked_return_percentage: numberValue(raw.ranked_return_percentage),
    ranked_index: numberValue(raw.ranked_index),
  };
}
