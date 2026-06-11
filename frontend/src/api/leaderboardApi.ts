import { apiRequest } from "@/api/client";
import { getStrategyFallback } from "@/pages/leaderboard/leaderboardUtils";
import type {
  LeaderboardEntry,
  LeaderboardGlobalResponse,
  LeaderboardQueryParams,
} from "@/types/leaderboard";

type LegacyEntry = {
  rank: number;
  display_name: string;
  avatar_key?: string;
  gain_loss_percentage: number;
  portfolio_index?: number;
};

function normalizeEntry(
  entry: Partial<LeaderboardEntry & LegacyEntry>,
): LeaderboardEntry {
  const rank = Number(entry.rank ?? 0);
  const displayName = String(entry.display_name ?? "Anonymous investor");
  return {
    rank,
    display_name: displayName,
    avatar_key: entry.avatar_key,
    strategy_tag:
      entry.strategy_tag || getStrategyFallback(displayName, rank),
    change_24h_percentage: entry.change_24h_percentage ?? null,
    total_roi_percentage: Number(
      entry.total_roi_percentage ?? entry.gain_loss_percentage ?? 0,
    ),
    portfolio_index: entry.portfolio_index,
    is_me: entry.is_me ?? false,
  };
}

function normalizeLeaderboard(
  data: LegacyEntry[],
  params: Required<LeaderboardQueryParams>,
): LeaderboardGlobalResponse {
  const normalized = data.map(normalizeEntry);
  const start = (params.page - 1) * params.page_size;
  return {
    timeframe: params.timeframe,
    page: params.page,
    page_size: params.page_size,
    total: normalized.length,
    entries: normalized.slice(start, start + params.page_size),
    me: null,
  };
}

export async function getGlobalLeaderboard(
  params: LeaderboardQueryParams,
): Promise<LeaderboardGlobalResponse> {
  const normalizedParams: Required<LeaderboardQueryParams> = {
    timeframe: params.timeframe,
    page: params.page ?? 1,
    page_size: params.page_size ?? 50,
  };
  // TODO: Wire timeframe query params when backend supports /leaderboard/global.
  const response = await apiRequest<LegacyEntry[]>("/leaderboard");
  return normalizeLeaderboard(response, normalizedParams);
}
