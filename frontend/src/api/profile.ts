import { apiRequest } from "@/api/client";
import type {
  Concentration,
  Exposure,
  MyProfile,
  PerformanceHistoryPoint,
  PublicClosedPosition,
  ProfileBadge,
  ProfileBenchmark,
  ProfileContributor,
  ProfileDNAExplanations,
  ProfileDNAScores,
  ProfileInsights,
  PublicProfile,
  PublicWeight,
  UpdateProfileRequest,
} from "@/types/profile";

type UnknownRecord = Record<string, unknown>;

const record = (value: unknown): UnknownRecord =>
  value && typeof value === "object" && !Array.isArray(value)
    ? (value as UnknownRecord)
    : {};

const stringValue = (value: unknown): string | undefined =>
  typeof value === "string" ? value : undefined;

const nonEmptyString = (value: unknown): string | undefined => {
  const text = stringValue(value)?.trim();
  return text ? text : undefined;
};

const numberValue = (value: unknown): number | undefined =>
  typeof value === "number" && Number.isFinite(value) ? value : undefined;

const booleanValue = (value: unknown): boolean =>
  typeof value === "boolean" ? value : false;

function normalizeBadge(value: unknown): ProfileBadge | null {
  const raw = record(value);
  const key = stringValue(raw.key);
  const name = stringValue(raw.name);
  if (!key || !name) return null;
  return {
    key,
    name,
    icon_key: stringValue(raw.icon_key) ?? stringValue(raw.icon),
    unlocked_at: stringValue(raw.unlocked_at),
  };
}

function normalizePerformancePoint(value: unknown): PerformanceHistoryPoint | null {
  const raw = record(value);
  const capturedAt = stringValue(raw.captured_at);
  const portfolioIndex = numberValue(raw.portfolio_index);
  const returnPercentage =
    numberValue(raw.return_percentage) ?? numberValue(raw.gain_loss_percentage);
  if (!capturedAt || portfolioIndex === undefined || returnPercentage === undefined) {
    return null;
  }
  return {
    captured_at: capturedAt,
    portfolio_index: portfolioIndex,
    return_percentage: returnPercentage,
  };
}

function normalizeClosedPosition(value: unknown): PublicClosedPosition | null {
  const raw = record(value);
  const symbol = stringValue(raw.symbol);
  const returnPercentage =
    numberValue(raw.return_percentage) ??
    numberValue(raw.realized_gain_loss_percentage);
  if (!symbol || returnPercentage === undefined) return null;
  return {
    symbol,
    asset_type: stringValue(raw.asset_type),
    closed_at: stringValue(raw.closed_at),
    return_percentage: returnPercentage,
  };
}

function normalizeWeight(value: unknown): PublicWeight | null {
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

function normalizeExposure(value: unknown): Exposure | null {
  const raw = record(value);
  const name =
    stringValue(raw.name) ??
    stringValue(raw.asset_type) ??
    stringValue(raw.currency);
  const weight = numberValue(raw.weight_percentage) ?? numberValue(raw.weight);
  if (!name || weight === undefined) return null;
  return { name, weight_percentage: weight };
}

function normalizeConcentration(value: unknown): Concentration | undefined {
  const raw = record(value);
  const concentration = {
    position_count: numberValue(raw.position_count),
    largest_weight_percentage:
      numberValue(raw.largest_weight_percentage) ??
      numberValue(raw.largest_position),
    top3_weight_percentage:
      numberValue(raw.top3_weight_percentage) ?? numberValue(raw.top_three),
  };
  return Object.values(concentration).some((item) => item !== undefined)
    ? concentration
    : undefined;
}

const emptyDNA = (): ProfileDNAScores => ({
  growth: 0,
  income: 0,
  commodities: 0,
  defensive: 0,
  international: 0,
  concentration: 0,
  volatility: 0,
});

function normalizeDNA(value: unknown): ProfileDNAScores {
  const raw = record(value);
  return {
    growth: numberValue(raw.growth) ?? 0,
    income: numberValue(raw.income) ?? 0,
    commodities: numberValue(raw.commodities) ?? 0,
    defensive: numberValue(raw.defensive) ?? 0,
    international: numberValue(raw.international) ?? 0,
    concentration: numberValue(raw.concentration) ?? 0,
    volatility: numberValue(raw.volatility) ?? 0,
  };
}

const emptyDNAExplanations = (): ProfileDNAExplanations => ({
  growth: [],
  income: [],
  commodities: [],
  defensive: [],
  international: [],
  concentration: [],
  volatility: [],
});

function normalizeDNAExplanations(value: unknown): ProfileDNAExplanations {
  const raw = record(value);
  return {
    growth: normalizeStringArray(raw.growth),
    income: normalizeStringArray(raw.income),
    commodities: normalizeStringArray(raw.commodities),
    defensive: normalizeStringArray(raw.defensive),
    international: normalizeStringArray(raw.international),
    concentration: normalizeStringArray(raw.concentration),
    volatility: normalizeStringArray(raw.volatility),
  };
}

function normalizeStringArray(value: unknown): string[] {
  return Array.isArray(value)
    ? value.filter((item): item is string => typeof item === "string")
    : [];
}

function normalizeContributor(value: unknown): ProfileContributor | null {
  const raw = record(value);
  const symbol = stringValue(raw.symbol);
  const contribution = numberValue(raw.contribution_points);
  if (!symbol || contribution === undefined) return null;
  return {
    symbol,
    name: stringValue(raw.name),
    contribution_points: contribution,
  };
}

function normalizeBenchmark(value: unknown): ProfileBenchmark | null {
  const raw = record(value);
  const symbol = stringValue(raw.symbol);
  const name = stringValue(raw.name);
  const index = numberValue(raw.index);
  const edge = numberValue(raw.edge_points);
  if (!symbol || !name || index === undefined || edge === undefined) return null;
  return { symbol, name, index, edge_points: edge };
}

function normalizeInsights(
  value: unknown,
  fallbackIndex?: number,
): ProfileInsights {
  const raw = record(value);
  const drivers = record(raw.performance_drivers);
  const benchmarks = record(raw.benchmark_context);
  const openClosed = record(raw.open_closed_performance);

  return {
    investment_style:
      nonEmptyString(raw.investment_style) ?? "Profile warming up",
    style_summary:
      nonEmptyString(raw.style_summary) ??
      "Portfolio DNA will appear after this investor adds enough positions.",
    focus_areas: normalizeStringArray(raw.focus_areas),
    dna: Object.keys(record(raw.dna)).length > 0 ? normalizeDNA(raw.dna) : emptyDNA(),
    dna_explanations:
      Object.keys(record(raw.dna_explanations)).length > 0
        ? normalizeDNAExplanations(raw.dna_explanations)
        : emptyDNAExplanations(),
    performance_drivers: {
      summary:
        nonEmptyString(drivers.summary) ??
        "Performance drivers will appear after this investor has enough portfolio activity.",
      positive_drivers: normalizeStringArray(drivers.positive_drivers),
      negative_drivers: normalizeStringArray(drivers.negative_drivers),
      open_contribution_points:
        numberValue(drivers.open_contribution_points) ?? 0,
      closed_contribution_points:
        numberValue(drivers.closed_contribution_points) ?? 0,
    },
    benchmark_context: {
      investor_index:
        numberValue(benchmarks.investor_index) ?? fallbackIndex ?? 100,
      benchmarks: Array.isArray(benchmarks.benchmarks)
        ? benchmarks.benchmarks
            .map(normalizeBenchmark)
            .filter((item): item is ProfileBenchmark => item !== null)
        : [],
      note: nonEmptyString(benchmarks.note),
    },
    contributors: Array.isArray(raw.contributors)
      ? raw.contributors
          .map(normalizeContributor)
          .filter((item): item is ProfileContributor => item !== null)
      : [],
    detractors: Array.isArray(raw.detractors)
      ? raw.detractors
          .map(normalizeContributor)
          .filter((item): item is ProfileContributor => item !== null)
      : [],
    open_closed_performance: {
      open_return_percentage:
        numberValue(openClosed.open_return_percentage) ?? 0,
      closed_return_percentage:
        numberValue(openClosed.closed_return_percentage) ?? 0,
      open_contribution_points:
        numberValue(openClosed.open_contribution_points) ?? 0,
      closed_contribution_points:
        numberValue(openClosed.closed_contribution_points) ?? 0,
      has_closed_positions: booleanValue(openClosed.has_closed_positions),
      composition_visible: booleanValue(openClosed.composition_visible),
      includes_self_reported_prices: booleanValue(
        openClosed.includes_self_reported_prices,
      ),
    },
  };
}

export function normalizePublicProfile(value: unknown): PublicProfile {
  const raw = record(value);
  const weights = Array.isArray(raw.public_weights)
    ? raw.public_weights
        .map(normalizeWeight)
        .filter((item): item is PublicWeight => item !== null)
        .sort((a, b) => b.weight_percentage - a.weight_percentage)
    : [];
  const concentration = normalizeConcentration(raw.concentration);

  return {
    handle: stringValue(raw.handle) ?? "",
    display_name: stringValue(raw.display_name) ?? "Investor",
    avatar_key: stringValue(raw.avatar_key),
    bio: stringValue(raw.bio),
    strategy_tag: stringValue(raw.strategy_tag),
    joined_at: stringValue(raw.joined_at),
    portfolio_index: numberValue(raw.portfolio_index),
    return_percentage: numberValue(raw.return_percentage),
    global_rank: numberValue(raw.global_rank) ?? null,
    sprint_rank: numberValue(raw.sprint_rank) ?? null,
    badges: Array.isArray(raw.badges)
      ? raw.badges
          .map(normalizeBadge)
          .filter((item): item is ProfileBadge => item !== null)
      : [],
    performance_history: Array.isArray(raw.performance_history)
      ? raw.performance_history
          .map(normalizePerformancePoint)
          .filter((item): item is PerformanceHistoryPoint => item !== null)
      : [],
    public_closed_positions: Array.isArray(raw.public_closed_positions)
      ? raw.public_closed_positions
          .map(normalizeClosedPosition)
          .filter((item): item is PublicClosedPosition => item !== null)
      : [],
    public_weights: weights,
    asset_type_exposure: Array.isArray(raw.asset_type_exposure)
      ? raw.asset_type_exposure
          .map(normalizeExposure)
          .filter((item): item is Exposure => item !== null)
      : [],
    currency_exposure: Array.isArray(raw.currency_exposure)
      ? raw.currency_exposure
          .map(normalizeExposure)
          .filter((item): item is Exposure => item !== null)
      : [],
    concentration:
      concentration || weights.length > 0
        ? {
            ...concentration,
            position_count:
              concentration?.position_count ??
              (weights.length > 0 ? weights.length : undefined),
          }
        : undefined,
    insights: normalizeInsights(raw.insights, numberValue(raw.portfolio_index)),
  };
}

function normalizeMyProfile(value: unknown): MyProfile {
  const raw = record(value);
  return {
    handle: stringValue(raw.handle) ?? "",
    display_name: stringValue(raw.display_name) ?? "Investor",
    avatar_key: stringValue(raw.avatar_key),
    bio: stringValue(raw.bio),
    strategy_tag: stringValue(raw.strategy_tag),
    is_public: booleanValue(raw.is_public),
    show_public_weights: booleanValue(raw.show_public_weights),
    created_at: stringValue(raw.created_at),
    updated_at: stringValue(raw.updated_at),
    public_preview: normalizePublicProfile(raw.public_preview),
  };
}

export async function getMyProfile(signal?: AbortSignal): Promise<MyProfile> {
  return normalizeMyProfile(
    await apiRequest<unknown>("/profiles/me", { signal }),
  );
}

export async function updateMyProfile(
  input: UpdateProfileRequest,
): Promise<MyProfile> {
  return normalizeMyProfile(
    await apiRequest<unknown>("/profiles/me", { method: "PATCH", body: input }),
  );
}

export async function getPublicProfile(
  handle: string,
  signal?: AbortSignal,
): Promise<PublicProfile> {
  return normalizePublicProfile(
    await apiRequest<unknown>(`/profiles/${encodeURIComponent(handle)}`, {
      signal,
    }),
  );
}
