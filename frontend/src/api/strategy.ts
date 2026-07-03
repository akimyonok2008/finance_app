import { apiRequest } from "@/api/client";
import type {
  CopyFromProfileResponse,
  CopyPreviewResponse,
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
