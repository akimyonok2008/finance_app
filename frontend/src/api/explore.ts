import { apiRequest } from "@/api/client";
import type {
  ExploreBadge,
  ExploreConcentration,
  ExploreParams,
  ExploreProfile,
  ExplorePublicWeight,
  ExploreResponse,
  TrendingHolding,
} from "@/types/explore";

type UnknownRecord = Record<string, unknown>;

const record = (value: unknown): UnknownRecord =>
  value && typeof value === "object" && !Array.isArray(value)
    ? (value as UnknownRecord)
    : {};

const stringValue = (value: unknown): string | undefined =>
  typeof value === "string" ? value : undefined;

const numberValue = (value: unknown): number | undefined =>
  typeof value === "number" && Number.isFinite(value) ? value : undefined;

const booleanValue = (value: unknown): boolean | undefined =>
  typeof value === "boolean" ? value : undefined;

function normalizeWeight(value: unknown): ExplorePublicWeight | null {
  const raw = record(value);
  const symbol = stringValue(raw.symbol);
  const weight = numberValue(raw.weight_percentage) ?? numberValue(raw.weight);
  if (!symbol || weight === undefined) return null;
  return {
    symbol,
    weight_percentage: weight,
    asset_type: stringValue(raw.asset_type),
  };
}

function normalizeBadge(value: unknown): ExploreBadge | null {
  const raw = record(value);
  const key = stringValue(raw.key);
  const name = stringValue(raw.name);
  if (!key || !name) return null;
  return {
    key,
    name,
    icon_key: stringValue(raw.icon_key) ?? stringValue(raw.icon),
  };
}

function normalizeConcentration(value: unknown): ExploreConcentration | undefined {
  const raw = record(value);
  const result = {
    position_count: numberValue(raw.position_count),
    largest_weight_percentage:
      numberValue(raw.largest_weight_percentage) ??
      numberValue(raw.largest_position),
    top3_weight_percentage:
      numberValue(raw.top3_weight_percentage) ?? numberValue(raw.top_three),
  };
  return Object.values(result).some((item) => item !== undefined)
    ? result
    : undefined;
}

function normalizeProfile(value: unknown): ExploreProfile | null {
  const raw = record(value);
  const handle = stringValue(raw.handle);
  const displayName = stringValue(raw.display_name);
  if (!handle || !displayName) return null;

  const weights = Array.isArray(raw.public_weights)
    ? raw.public_weights
        .map(normalizeWeight)
        .filter((item): item is ExplorePublicWeight => item !== null)
        .sort((a, b) => b.weight_percentage - a.weight_percentage)
    : [];
  const concentration = normalizeConcentration(raw.concentration);

  return {
    handle,
    display_name: displayName,
    avatar_key: stringValue(raw.avatar_key),
    bio: stringValue(raw.bio),
    strategy_tag: stringValue(raw.strategy_tag),
    ranked_index:
      numberValue(raw.ranked_index) ?? numberValue(raw.portfolio_index),
    ranked_return_percentage:
      numberValue(raw.ranked_return_percentage) ??
      numberValue(raw.return_percentage),
    global_rank: numberValue(raw.global_rank) ?? null,
    sprint_rank: numberValue(raw.sprint_rank) ?? null,
    public_weights: weights,
    concentration:
      concentration || weights.length > 0
        ? {
            ...concentration,
            position_count: concentration?.position_count ?? weights.length,
          }
        : undefined,
    badges: Array.isArray(raw.badges)
      ? raw.badges
          .map(normalizeBadge)
          .filter((item): item is ExploreBadge => item !== null)
      : [],
  };
}

function normalizeProfiles(value: unknown): ExploreProfile[] {
  return Array.isArray(value)
    ? value
        .map(normalizeProfile)
        .filter((item): item is ExploreProfile => item !== null)
    : [];
}

function normalizeTrending(value: unknown): TrendingHolding | null {
  const raw = record(value);
  const symbol = stringValue(raw.symbol);
  const profileCount = numberValue(raw.profile_count);
  if (!symbol || profileCount === undefined) return null;
  return {
    symbol,
    profile_count: profileCount,
    average_weight_percentage:
      numberValue(raw.average_weight_percentage) ??
      numberValue(raw.average_weight),
    top10_count: numberValue(raw.top10_count),
    asset_type: stringValue(raw.asset_type),
  };
}

function queryString(params: ExploreParams): string {
  const search = new URLSearchParams();
  if (params.q?.trim()) search.set("q", params.q.trim());
  if (params.symbol?.trim()) search.set("symbol", params.symbol.trim().toUpperCase());
  if (params.sort && params.sort !== "top") search.set("sort", params.sort);
  if (params.limit) search.set("limit", String(params.limit));
  if (params.offset) search.set("offset", String(params.offset));
  const value = search.toString();
  return value ? `?${value}` : "";
}

export async function getExploreProfiles(
  params: ExploreParams,
  signal?: AbortSignal,
): Promise<ExploreResponse> {
  const raw = record(
    await apiRequest<unknown>(`/profiles/explore${queryString(params)}`, {
      signal,
    }),
  );
  const topPerformers = normalizeProfiles(raw.top_performers);
  const featured = normalizeProfiles(raw.featured);
  const similar = normalizeProfiles(raw.similar);
  const paginationRaw = record(raw.pagination);

  return {
    featured: featured.length > 0 ? featured : topPerformers.slice(0, 3),
    similar,
    top_performers: topPerformers,
    trending_holdings: Array.isArray(raw.trending_holdings)
      ? raw.trending_holdings
          .map(normalizeTrending)
          .filter((item): item is TrendingHolding => item !== null)
      : [],
    pagination:
      Object.keys(paginationRaw).length > 0
        ? {
            limit: numberValue(paginationRaw.limit) ?? params.limit ?? 20,
            offset: numberValue(paginationRaw.offset) ?? params.offset ?? 0,
            total: numberValue(paginationRaw.total),
            has_more: booleanValue(paginationRaw.has_more),
          }
        : undefined,
  };
}
