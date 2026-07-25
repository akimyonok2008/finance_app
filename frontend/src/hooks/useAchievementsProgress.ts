import { useMemo } from "react";
import { useQuery } from "@tanstack/react-query";

import { evaluateAchievements } from "@/api/achievements";
import {
  DIFFICULTY_ORDER,
  badgeCatalogue,
  type BadgeDifficulty,
} from "@/data/badgeCatalogue";
import { queryKeys } from "@/hooks/queryKeys";
import type { Achievement } from "@/types/dashboard";
import type {
  AchievementProgress,
  AchievementsSummary,
  BadgeStatus,
  DifficultyBreakdown,
} from "@/types/achievements";

const NO_LIVE_PROGRESS_REASON =
  "Benchmark tracking is active and collecting the portfolio history required for this badge.";
const NO_BACKEND_REASON =
  "Benchmark tracking is active. Progress will appear as portfolio history becomes available.";

/**
 * Merges the static badge catalogue with the user's real backend status and
 * award evidence. Locked badges use the backend's real history coverage,
 * portfolio return, benchmark return, and current edge.
 */
export function mergeAchievements(
  backend: Achievement[] | undefined,
): AchievementProgress[] {
  const byKey = new Map<string, Achievement>();
  for (const a of backend ?? []) byKey.set(a.key, a);
  const hasBackend = Boolean(backend);

  return badgeCatalogue.map((entry) => {
    const row = byKey.get(entry.id);

    if (row?.unlocked) {
      const ev = row.evidence ?? null;
      return {
        ...entry,
        status: "unlocked" as BadgeStatus,
        progressPct: 100,
        currentEdgePoints: ev?.edge_points,
        portfolioReturnPct: ev?.portfolio_return_pct,
        benchmarkReturnPct: ev?.benchmark_return_pct,
        awardedAt: row.unlocked_at ?? undefined,
        evidence: ev
          ? {
              startDate: ev.start_date,
              endDate: ev.end_date,
              portfolioReturnPct: ev.portfolio_return_pct,
              benchmarkReturnPct: ev.benchmark_return_pct,
              edgePoints: ev.edge_points,
              evaluationModel: ev.evaluation_model,
              evidenceVersion: ev.evidence_version,
              trackingEpoch: ev.tracking_epoch,
              startRankedIndex: ev.start_ranked_index,
              endRankedIndex: ev.end_ranked_index,
              startSnapshotAt: ev.start_snapshot_at,
              endSnapshotAt: ev.end_snapshot_at,
              activeCoveragePct: ev.active_coverage_percentage,
              trustedCoveragePct: ev.trusted_data_coverage_percentage,
              benchmarkDataSource: ev.benchmark_data_source,
              snapshotFrequency: ev.snapshot_frequency,
              benchmarkRecipeVersion: ev.benchmark_recipe_version,
              verification: ev.verification,
              rebalancingPolicy: ev.rebalancing_policy,
              priceType: ev.benchmark_data?.price_type,
              dataQuality: ev.benchmark_data?.quality,
              isSynthetic: ev.benchmark_data?.is_synthetic,
              totalReturnMethod: ev.benchmark_data?.total_return_method,
              recipeSourceAccession: ev.benchmark_data?.recipe_source_accession,
              recipeReportPeriodEnd: ev.benchmark_data?.recipe_report_period_end,
              mappingCoveragePct: ev.benchmark_data?.mapping_coverage_percentage,
            }
          : undefined,
        legacyEvidence: row.legacy_evidence,
      };
    }

    if (row?.progress) {
      const progress = row.progress;
      return {
        ...entry,
        status: "in_progress" as BadgeStatus,
        progressPct: progress.progress_percentage,
        historyCoveragePct: progress.history_coverage_percentage,
        activeCoveragePct: progress.active_coverage_percentage,
        trustedDataCoveragePct: progress.trusted_data_coverage_percentage,
        progressState: progress.state,
        trackingStartDate: progress.effective_start_at ?? progress.start_date,
        trackingEndDate: progress.effective_end_at ?? progress.end_date,
        latestSnapshotAt: progress.latest_snapshot_at,
        currentEdgePoints: progress.current_edge_points,
        portfolioReturnPct: progress.portfolio_return_percentage,
        benchmarkReturnPct: progress.benchmark_return_percentage,
        unavailableReason: progress.reason,
      };
    }

    return {
      ...entry,
      status: "locked" as BadgeStatus,
      unavailableReason: hasBackend ? NO_LIVE_PROGRESS_REASON : NO_BACKEND_REASON,
    };
  });
}

export function summarize(items: AchievementProgress[]): AchievementsSummary {
  const byDifficulty = DIFFICULTY_ORDER.reduce((acc, d) => {
    acc[d] = { unlocked: 0, total: 0 };
    return acc;
  }, {} as DifficultyBreakdown);

  for (const item of items) {
    byDifficulty[item.difficulty].total += 1;
    if (item.status === "unlocked") byDifficulty[item.difficulty].unlocked += 1;
  }

  const rank = (d: BadgeDifficulty) => DIFFICULTY_ORDER.indexOf(d);
  const unlockedItems = items.filter((i) => i.status === "unlocked");
  const lockedItems = items.filter((i) => i.status !== "unlocked");

  const mostPrestigious = unlockedItems.reduce<AchievementProgress | undefined>(
    (best, item) =>
      !best || rank(item.difficulty) > rank(best.difficulty) ? item : best,
    undefined,
  );

  const nextTarget = lockedItems.reduce<AchievementProgress | undefined>(
    (easiest, item) =>
      !easiest || rank(item.difficulty) < rank(easiest.difficulty)
        ? item
        : easiest,
    undefined,
  );

  const nextEliteTarget = lockedItems.find((i) => i.difficulty === "elite");

  return {
    unlocked: unlockedItems.length,
    total: items.length,
    byDifficulty,
    mostPrestigious,
    nextTarget,
    nextEliteTarget,
  };
}

/**
 * Loads and merges achievements. Returns the view-model list, summary, and the
 * underlying query so the page can render loading/error/empty states honestly.
 */
export function useAchievementsProgress() {
  const query = useQuery({
    // Distinct sub-key so this POST-evaluate query has its own cache entry
    // (separate from the GET-based useAchievements), while still being reached
    // by prefix-based invalidateQueries({ queryKey: queryKeys.achievements }).
    queryKey: [...queryKeys.achievements, "progress"],
    queryFn: ({ signal }) => evaluateAchievements(signal),
    retry: 1,
  });

  const items = useMemo(() => mergeAchievements(query.data), [query.data]);
  const summary = useMemo(() => summarize(items), [items]);

  return { query, items, summary };
}
