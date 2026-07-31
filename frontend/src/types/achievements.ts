import type {
  BadgeCatalogueEntry,
  BadgeDifficulty,
} from "@/data/badgeCatalogue";

export type BadgeStatus = "locked" | "in_progress" | "unlocked";

export type BadgeEvidence = {
  startDate: string;
  endDate: string;
  portfolioReturnPct: number;
  benchmarkReturnPct: number;
  edgePoints: number;
  evaluationModel?: string;
  evidenceVersion?: number;
  trackingEpoch?: string;
  startRankedIndex?: number;
  endRankedIndex?: number;
  startSnapshotAt?: string;
  endSnapshotAt?: string;
  activeCoveragePct?: number;
  trustedCoveragePct?: number;
  benchmarkDataSource?: string;
  snapshotFrequency?: string;
  // Benchmark data-integrity provenance.
  benchmarkRecipeVersion?: string;
  verification?: string;
  rebalancingPolicy?: string;
  priceType?: string;
  dataQuality?: string;
  isSynthetic?: boolean;
  totalReturnMethod?: string;
  recipeSourceAccession?: string;
  recipeReportPeriodEnd?: string;
  mappingCoveragePct?: number;
};

// The per-badge view model consumed by the UI. It combines the static catalogue
// entry with the user's real status/evidence from the backend. Progress fields
// are only populated when the backend actually provides them — never faked.
export type AchievementProgress = BadgeCatalogueEntry & {
  status: BadgeStatus;
  /** 0-100, only when real progress is available. */
  progressPct?: number;
  historyCoveragePct?: number;
  activeCoveragePct?: number;
  trustedDataCoveragePct?: number;
  progressState?: string;
  trackingStartDate?: string;
  trackingEndDate?: string;
  latestSnapshotAt?: string;
  currentEdgePoints?: number;
  portfolioReturnPct?: number;
  benchmarkReturnPct?: number;
  awardedAt?: string;
  evidence?: BadgeEvidence;
  legacyEvidence?: boolean;
  /** Live benchmark tracking status or, if unavailable, the reason why. */
  unavailableReason?: string;
};

export type DifficultyBreakdown = Record<
  BadgeDifficulty,
  { unlocked: number; total: number }
>;

export type AchievementsSummary = {
  unlocked: number;
  total: number;
  byDifficulty: DifficultyBreakdown;
  /** Highest-difficulty unlocked badge, if any. */
  mostPrestigious?: AchievementProgress;
  /** Easiest still-locked badge — the natural next target. */
  nextTarget?: AchievementProgress;
  /** First still-locked elite badge. */
  nextEliteTarget?: AchievementProgress;
};

export type AchievementReturnRow = {
  key: string;
  name: string;
  difficulty: string;
  native_period: string;
  available: boolean;
  portfolio_return_percentage?: number;
  benchmark_return_percentage?: number;
  edge_points?: number;
  aligned_from?: string;
  aligned_to?: string;
  reason?: string;
};

export type AchievementReturnsResponse = {
  timeframe: import("@/types/leaderboard").LeaderboardTimeframe;
  from?: string;
  to: string;
  rows: AchievementReturnRow[];
};
