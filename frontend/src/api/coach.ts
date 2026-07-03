import { apiRequest } from "@/api/client";
import type { CoachMode, CoachResponse, CompareProfileResponse } from "@/types/coach";

/**
 * Request a Portfolio Coach analysis for the given mode.
 *
 * Uses the shared {@link apiRequest} wrapper, so JWT attachment, 401 handling
 * (clear session + redirect to /login), and `{ error }` normalization are all
 * inherited. Response-shape tolerance lives in the type layer / UI, not here.
 *
 * An explicit, opt-in mock is available behind `VITE_ENABLE_MOCK_COACH=true`
 * for prototype/demo use only — it is OFF by default and never ships as
 * production-looking data.
 */
export async function requestPortfolioCoach(
  mode: CoachMode,
  signal?: AbortSignal,
): Promise<CoachResponse> {
  if (import.meta.env.VITE_ENABLE_MOCK_COACH === "true") {
    return mockCoachResponse(mode);
  }
  return apiRequest<CoachResponse>("/portfolio/coach", {
    method: "POST",
    body: { mode },
    signal,
  });
}

type UnknownRecord = Record<string, unknown>;

const record = (value: unknown): UnknownRecord =>
  value && typeof value === "object" && !Array.isArray(value)
    ? (value as UnknownRecord)
    : {};

const stringValue = (value: unknown): string | undefined =>
  typeof value === "string" ? value : undefined;

const numberValue = (value: unknown): number =>
  typeof value === "number" && Number.isFinite(value) ? value : 0;

export async function compareProfile(handle: string): Promise<CompareProfileResponse> {
  const raw = record(
    await apiRequest<unknown>("/coach/compare-profile", {
      method: "POST",
      body: { handle },
    }),
  );
  const target = record(raw.target_profile);
  const concentration = record(raw.concentration_comparison);
  return {
    target_profile: {
      handle: stringValue(target.handle) ?? handle,
      display_name: stringValue(target.display_name) ?? "Public strategy",
      avatar_key: stringValue(target.avatar_key),
      strategy_tag: stringValue(target.strategy_tag),
    },
    summary: stringValue(raw.summary) ?? "",
    overlap_score: numberValue(raw.overlap_score),
    shared_symbols: Array.isArray(raw.shared_symbols)
      ? raw.shared_symbols.filter((item): item is string => typeof item === "string")
      : [],
    weight_differences: Array.isArray(raw.weight_differences)
      ? raw.weight_differences.map((item) => {
          const diff = record(item);
          return {
            symbol: stringValue(diff.symbol) ?? "",
            my_weight_percentage: numberValue(diff.my_weight_percentage),
            target_weight_percentage: numberValue(diff.target_weight_percentage),
            difference_percentage: numberValue(diff.difference_percentage),
            asset_type: stringValue(diff.asset_type),
          };
        }).filter((item) => item.symbol.length > 0)
      : [],
    concentration_comparison: {
      my_position_count: numberValue(concentration.my_position_count),
      target_position_count: numberValue(concentration.target_position_count),
      my_top3_weight_percentage: numberValue(concentration.my_top3_weight_percentage),
      target_top3_weight_percentage: numberValue(concentration.target_top3_weight_percentage),
    },
    learning_points: Array.isArray(raw.learning_points)
      ? raw.learning_points.map((item) => {
          const point = record(item);
          return {
            title: stringValue(point.title) ?? "",
            body: stringValue(point.body) ?? "",
          };
        }).filter((item) => item.title.length > 0 || item.body.length > 0)
      : [],
    disclaimer:
      stringValue(raw.disclaimer) ??
      "This is educational comparison, not investment advice.",
  };
}

/**
 * Prototype-only mock. Clearly labeled in the title so it can never be mistaken
 * for real analysis. Enabled only when VITE_ENABLE_MOCK_COACH=true.
 */
async function mockCoachResponse(mode: CoachMode): Promise<CoachResponse> {
  await new Promise((r) => setTimeout(r, 600));
  return {
    mode,
    title: "[MOCK] Portfolio readout",
    summary:
      "This is prototype mock output (VITE_ENABLE_MOCK_COACH=true). It is not real analysis and should not be shipped.",
    risk_level: "unknown",
    observations: [
      {
        label: "Mock",
        status: "neutral",
        text: "Connect the backend and disable VITE_ENABLE_MOCK_COACH to see real analysis.",
      },
    ],
    top10_comparison: { available: false, notes: ["Mock mode: no benchmark."] },
    learning_points: ["This is mock data for local UI development only."],
    disclaimer: "Educational portfolio analysis only. Not financial advice.",
    generated_at: new Date().toISOString(),
  };
}
