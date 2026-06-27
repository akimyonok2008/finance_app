// Types for the Portfolio Coach feature (POST /portfolio/coach). Optional fields
// are modeled defensively because the backend may omit sections per mode.

export type CoachMode =
  | "analyze_portfolio"
  | "compare_top10"
  | "technical_setup"
  | "fundamental_context";

export type CoachRiskLevel =
  | "low"
  | "moderate"
  | "elevated"
  | "high"
  | "unknown";

export type CoachObservationStatus = "positive" | "neutral" | "watch" | "risk";

export type CoachObservation = {
  label: string;
  status: CoachObservationStatus | string;
  text: string;
};

export type CoachTop10Comparison = {
  available: boolean;
  sample_size?: number;
  limited?: boolean;
  return_gap_percentage_points?: number;
  shared_symbols_count?: number;
  user_largest_weight_percentage?: number;
  top10_median_largest_weight_percentage?: number;
  notes?: string[];
};

export type CoachResponse = {
  mode: CoachMode | string;
  title: string;
  summary: string;
  risk_level?: CoachRiskLevel | string;
  observations?: CoachObservation[];
  technical_notes?: string[];
  fundamental_notes?: string[];
  top10_comparison?: CoachTop10Comparison;
  learning_points?: string[];
  questions_to_consider?: string[];
  disclaimer?: string;
  generated_at?: string;
};

/** UI metadata for each mode button. */
export type CoachModeMeta = {
  mode: CoachMode;
  label: string;
  description: string;
};

export const COACH_MODES: CoachModeMeta[] = [
  {
    mode: "fundamental_context",
    label: "Fundamental Analysis",
    description: "Business, sector, and valuation context where data allows.",
  },
  {
    mode: "technical_setup",
    label: "Technical Analysis",
    description: "Trend, momentum, volatility, and setup notes where data allows.",
  },
  {
    mode: "analyze_portfolio",
    label: "Portfolio Review",
    description: "Structure, concentration, risk, and learning points.",
  },
  {
    mode: "compare_top10",
    label: "Compare with Top 10",
    description: "Compare your weights and performance with top public portfolios.",
  },
];

/** Fallback disclaimer when the backend omits one. */
export const COACH_DISCLAIMER_FALLBACK =
  "Educational portfolio analysis only. Not financial advice.";
