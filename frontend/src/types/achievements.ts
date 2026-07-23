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
};

// The per-badge view model consumed by the UI. It combines the static catalogue
// entry with the user's real status/evidence from the backend. Progress fields
// are only populated when the backend actually provides them — never faked.
export type AchievementProgress = BadgeCatalogueEntry & {
  status: BadgeStatus;
  /** 0-100, only when real progress is available. */
  progressPct?: number;
  historyCoveragePct?: number;
  progressState?: string;
  trackingStartDate?: string;
  trackingEndDate?: string;
  currentEdgePoints?: number;
  portfolioReturnPct?: number;
  benchmarkReturnPct?: number;
  awardedAt?: string;
  evidence?: BadgeEvidence;
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
