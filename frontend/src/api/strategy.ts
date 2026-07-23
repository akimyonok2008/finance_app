import { apiRequest } from "@/api/client";
import type {
  CopyFromProfileResponse,
  CopyPreviewResponse,
  CompareProfileResponse,
  StrategySourceProfile,
  StrategyWeight,
} from "@/types/strategy";

type UnknownRecord = Record<string, unknown>;

const record = (value: unknown): UnknownRecord =>
  value && typeof value === "object" && !Array.isArray(value)
    ? (value as UnknownRecord)
    : {};

const stringValue = (value: unknown): string | undefined =>
  typeof value === "string" ? value : undefined;

const numberValue = (value: unknown): number =>
  typeof value === "number" && Number.isFinite(value) ? value : 0;

function normalizeSource(value: unknown): StrategySourceProfile {
  const raw = record(value);
  return {
    handle: stringValue(raw.handle) ?? "",
    display_name: stringValue(raw.display_name) ?? "Public strategy",
    avatar_key: stringValue(raw.avatar_key),
    strategy_tag: stringValue(raw.strategy_tag),
  };
}

function normalizeWeight(value: unknown): StrategyWeight | null {
  const raw = record(value);
  const symbol = stringValue(raw.symbol);
  if (!symbol) return null;
  return {
    symbol: symbol.toUpperCase(),
    asset_type: stringValue(raw.asset_type) ?? "other",
    weight_percentage: numberValue(raw.weight_percentage),
  };
}

function normalizeCopyResponse(value: unknown): CopyPreviewResponse {
  const raw = record(value);
  return {
    source_profile: normalizeSource(raw.source_profile),
    weights: Array.isArray(raw.weights)
      ? raw.weights
          .map(normalizeWeight)
          .filter((item): item is StrategyWeight => item !== null)
      : [],
    disclaimer:
      stringValue(raw.disclaimer) ??
      "This copies public strategy weights only. No trades are executed.",
  };
}

export async function copyPreview(handle: string): Promise<CopyPreviewResponse> {
  return normalizeCopyResponse(
    await apiRequest<unknown>("/strategy-portfolio/copy-preview", {
      method: "POST",
      body: { handle },
    }),
  );
}

export async function copyFromProfile(
  handle: string,
  weights: StrategyWeight[],
): Promise<CopyFromProfileResponse> {
  return normalizeCopyResponse(
    await apiRequest<unknown>("/strategy-portfolio/copy-from-profile", {
      method: "POST",
      body: { handle, weights },
    }),
  );
}

export async function compareProfile(handle: string): Promise<CompareProfileResponse> {
  const raw = record(
    await apiRequest<unknown>("/strategy-portfolio/compare-profile", {
      method: "POST",
      body: { handle },
    }),
  );
  const concentration = record(raw.concentration_comparison);
  return {
    target_profile: normalizeSource(raw.target_profile),
    summary: stringValue(raw.summary) ?? "",
    overlap_score: numberValue(raw.overlap_score),
    shared_symbols: Array.isArray(raw.shared_symbols)
      ? raw.shared_symbols.filter((item): item is string => typeof item === "string")
      : [],
    weight_differences: Array.isArray(raw.weight_differences)
      ? raw.weight_differences
          .map((item) => {
            const diff = record(item);
            return {
              symbol: stringValue(diff.symbol) ?? "",
              my_weight_percentage: numberValue(diff.my_weight_percentage),
              target_weight_percentage: numberValue(diff.target_weight_percentage),
              difference_percentage: numberValue(diff.difference_percentage),
              asset_type: stringValue(diff.asset_type),
            };
          })
          .filter((item) => item.symbol.length > 0)
      : [],
    concentration_comparison: {
      my_position_count: numberValue(concentration.my_position_count),
      target_position_count: numberValue(concentration.target_position_count),
      my_top3_weight_percentage: numberValue(concentration.my_top3_weight_percentage),
      target_top3_weight_percentage: numberValue(concentration.target_top3_weight_percentage),
    },
    learning_points: Array.isArray(raw.learning_points)
      ? raw.learning_points
          .map((item) => {
            const point = record(item);
            return {
              title: stringValue(point.title) ?? "",
              body: stringValue(point.body) ?? "",
            };
          })
          .filter((item) => item.title.length > 0 || item.body.length > 0)
      : [],
    disclaimer:
      stringValue(raw.disclaimer) ??
      "This is educational comparison, not investment advice.",
  };
}
