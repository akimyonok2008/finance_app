import { BadgeDifficultyPill } from "@/components/achievements/BadgeDifficultyPill";
import { BadgeMark } from "@/components/achievements/BadgeMark";
import { BadgeProgressBar } from "@/components/achievements/BadgeProgressBar";
import { BadgeStatusPill } from "@/components/achievements/BadgeStatusPill";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { DIFFICULTY_PRESTIGE } from "@/data/badgeCatalogue";
import type { AchievementProgress } from "@/types/achievements";

function pct(value: number): string {
  return `${value >= 0 ? "+" : ""}${value.toFixed(2)}%`;
}

function pts(value: number): string {
  return `${value >= 0 ? "+" : ""}${value.toFixed(1)} pts`;
}

function Row({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex items-start justify-between gap-4 py-2">
      <dt className="text-xs text-zinc-500">{label}</dt>
      <dd className="text-right text-xs font-medium text-zinc-200">{value}</dd>
    </div>
  );
}

export function BadgeDetailModal({
  badge,
  onClose,
}: {
  badge: AchievementProgress | null;
  onClose: () => void;
}) {
  return (
    <Dialog open={Boolean(badge)} onOpenChange={(open) => !open && onClose()}>
      <DialogContent className="max-h-[92vh] overflow-y-auto border-violet-300/15 bg-[radial-gradient(circle_at_top_left,rgba(167,139,250,0.09),transparent_32%),radial-gradient(circle_at_bottom_right,rgba(34,211,238,0.045),transparent_34%),rgba(9,9,11,0.97)] sm:max-w-lg">
        {badge && (
          <>
            <DialogHeader>
              <div className="flex items-center gap-3">
                <BadgeMark
                  badgeId={badge.id}
                  className="h-11 w-11 rounded-2xl"
                  iconClassName="h-5 w-5"
                />
                <div className="min-w-0">
                  <DialogTitle className="achievements-card-title text-xl text-zinc-50">
                    {badge.name}
                  </DialogTitle>
                  <DialogDescription className="text-xs text-zinc-500">
                    Inspired by {badge.inspiredBy}
                  </DialogDescription>
                </div>
              </div>
            </DialogHeader>

            <div className="flex flex-wrap items-center gap-1.5">
              <BadgeStatusPill status={badge.status} />
              <BadgeDifficultyPill difficulty={badge.difficulty} />
              <span className="rounded-full border border-zinc-700 px-2 py-0.5 text-[10px] uppercase tracking-wide text-zinc-400">
                {badge.period}
              </span>
              <span className="rounded-full border border-zinc-700 px-2 py-0.5 text-[10px] uppercase tracking-wide text-zinc-400">
                {badge.category === "investor"
                  ? "Legendary investor"
                  : "Strategy"}
              </span>
            </div>

            <div className="rounded-xl border border-zinc-800 bg-zinc-900/40 px-4 py-1 divide-y divide-zinc-800/70">
              <Row
                label="Status"
                value={
                  badge.status === "unlocked"
                    ? "Unlocked"
                    : badge.status === "in_progress"
                      ? "In progress"
                      : "Locked"
                }
              />
              {typeof badge.progressPct === "number" && (
                <Row
                  label="Progress"
                  value={`${Math.round(badge.progressPct)}%`}
                />
              )}
              {typeof badge.historyCoveragePct === "number" && (
                <Row
                  label="History coverage"
                  value={`${Math.round(badge.historyCoveragePct)}%`}
                />
              )}
              <Row label="Benchmark recipe" value={badge.benchmark} />
              <Row label="Unlock rule" value={badge.unlockRule} />
              {badge.status === "unlocked" && badge.evidence ? (
                <>
                  <Row
                    label="Your return"
                    value={pct(badge.evidence.portfolioReturnPct)}
                  />
                  <Row
                    label="Benchmark return"
                    value={pct(badge.evidence.benchmarkReturnPct)}
                  />
                  <Row label="Edge" value={pts(badge.evidence.edgePoints)} />
                  <Row
                    label="Evaluated"
                    value={`${badge.evidence.startDate} → ${badge.evidence.endDate}`}
                  />
                  {badge.awardedAt && (
                    <Row
                      label="Awarded"
                      value={new Date(badge.awardedAt).toLocaleDateString()}
                    />
                  )}
                </>
              ) : (
                <>
                  <Row
                    label="Required edge"
                    value={
                      badge.requiredEdgePoints > 0
                        ? `+${badge.requiredEdgePoints.toFixed(1)} pts`
                        : "Positive return"
                    }
                  />
                  {typeof badge.currentEdgePoints === "number" && (
                    <Row
                      label="Current edge"
                      value={pts(badge.currentEdgePoints)}
                    />
                  )}
                  {typeof badge.portfolioReturnPct === "number" && (
                    <Row
                      label="Your return"
                      value={pct(badge.portfolioReturnPct)}
                    />
                  )}
                  {typeof badge.benchmarkReturnPct === "number" && (
                    <Row
                      label="Benchmark return"
                      value={pct(badge.benchmarkReturnPct)}
                    />
                  )}
                  {badge.trackingStartDate && badge.trackingEndDate && (
                    <Row
                      label="Tracking window"
                      value={`${badge.trackingStartDate} → ${badge.trackingEndDate}`}
                    />
                  )}
                </>
              )}
            </div>

            {badge.status === "unlocked" ? (
              <div className="space-y-1.5">
                <BadgeProgressBar pct={100} status="unlocked" />
                <p className="text-[11px] text-emerald-400">
                  Unlocked — benchmark cleared.
                </p>
              </div>
            ) : typeof badge.progressPct === "number" ? (
              <div className="space-y-1.5">
                <BadgeProgressBar
                  pct={badge.progressPct}
                  status={badge.status}
                />
                <p className="text-[11px] text-sky-300">
                  {badge.unavailableReason ??
                    `${Math.round(badge.progressPct)}% complete`}
                </p>
              </div>
            ) : (
              <p className="rounded-lg border border-zinc-800 bg-zinc-900/40 px-3 py-2 text-[11px] leading-5 text-zinc-500">
                {badge.unavailableReason ??
                  "Benchmark tracking is active and collecting portfolio history."}
              </p>
            )}

            <div className="space-y-2 border-t border-zinc-800/80 pt-3">
              <p className="text-sm leading-6 text-zinc-300">
                {badge.explanation}
              </p>
              <p className="text-xs leading-5 text-zinc-500">
                {DIFFICULTY_PRESTIGE[badge.difficulty]}
              </p>
            </div>
          </>
        )}
      </DialogContent>
    </Dialog>
  );
}
