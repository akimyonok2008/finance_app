import { apiRequest } from "@/api/client";
import type { CoachMode, CoachResponse } from "@/types/coach";

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
