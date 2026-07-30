import { motion } from "framer-motion";
import { CheckCircle2, ChevronLeft, ShieldCheck, ShieldX, Users, XCircle } from "lucide-react";
import { Link, useParams } from "react-router-dom";

import { AppNav } from "@/components/layout/AppNav";
import { useCompetitionDetail } from "@/hooks/useArena";
import { ArenaEmptyState } from "@/pages/arena/ArenaEmptyState";
import { CompetitionLeaderboardTable } from "@/pages/arena/CompetitionLeaderboardTable";
import { CountdownTimer } from "@/pages/arena/CountdownTimer";
import { categoryLabel, formatSignedPercent, statusLabel } from "@/pages/arena/arenaUtils";
import { cn } from "@/utils/cn";

function ScoringExplanation({ scope, joined }: { scope: string | undefined; joined: boolean }) {
  const label =
    scope === "matching_assets"
      ? "Only positions matching this competition's rules are scored."
      : scope === "custom_universe"
        ? "Only positions in this competition's configured universe are scored."
        : "Your complete portfolio composition is scored.";
  return (
    <div className="flex items-start gap-2 text-sm text-zinc-400">
      <ShieldCheck className="mt-0.5 h-4 w-4 shrink-0 text-violet-400" />
      <span>
        {label}
        {joined ? " Your composition was frozen when you joined." : ""}
      </span>
    </div>
  );
}

export function CompetitionDetailPage() {
  const { competitionId } = useParams<{ competitionId: string }>();
  const detail = useCompetitionDetail(competitionId);

  if (detail.isLoading) {
    return (
      <div className="min-h-screen bg-zinc-950 text-zinc-50">
        <main className="mx-auto w-full max-w-5xl px-4 py-4 sm:px-6 lg:px-8">
          <AppNav />
          <p className="mt-10 text-sm text-zinc-500">Loading competition…</p>
        </main>
      </div>
    );
  }

  if (detail.isError || !detail.card) {
    return (
      <div className="min-h-screen bg-zinc-950 text-zinc-50">
        <main className="mx-auto w-full max-w-5xl px-4 py-4 sm:px-6 lg:px-8">
          <AppNav />
          <ArenaEmptyState error onRetry={detail.refetch} />
        </main>
      </div>
    );
  }

  const { card, eligibility, status, leaderboard, results } = detail;
  const isCompleted = card.status === "completed";
  const canWithdraw =
    card.joined && status?.entryStatus === "admitted" && card.status === "registration_open";
  const canJoin = !card.joined && card.status === "registration_open";

  return (
    <div className="min-h-screen bg-zinc-950 text-zinc-50">
      <main className="mx-auto w-full max-w-5xl px-4 py-4 sm:px-6 lg:px-8">
        <AppNav />

        <Link
          to="/arena"
          className="mb-6 inline-flex items-center gap-1.5 text-sm text-zinc-400 transition hover:text-zinc-200"
        >
          <ChevronLeft className="h-4 w-4" />
          Back to Arena
        </Link>

        <motion.header
          initial={{ opacity: 0, y: -8 }}
          animate={{ opacity: 1, y: 0 }}
          transition={{ duration: 0.25 }}
          className="mb-8 rounded-2xl border border-zinc-800 bg-zinc-900/50 p-6 shadow-sm shadow-black/20"
        >
          <div className="flex flex-wrap items-start justify-between gap-4">
            <div>
              {card.category && (
                <div className="mb-2 text-xs font-medium uppercase tracking-[0.16em] text-violet-300">
                  {categoryLabel(card.category)}
                </div>
              )}
              <h1 className="text-2xl font-medium tracking-tight">{card.name}</h1>
              {card.description && (
                <p className="mt-2 max-w-2xl text-sm text-zinc-400">{card.description}</p>
              )}
            </div>
            <span className="rounded-full border border-zinc-700 bg-zinc-900 px-3 py-1 text-xs text-zinc-300">
              {statusLabel(card.status)}
            </span>
          </div>

          <div className="mt-6 flex flex-wrap items-center gap-6 text-sm text-zinc-400">
            <span className="inline-flex items-center gap-1.5">
              <Users className="h-4 w-4" />
              {card.participantCount} joined
            </span>
            <span>
              {new Date(card.startsAt).toLocaleDateString()} –{" "}
              {new Date(card.endsAt).toLocaleDateString()}
            </span>
          </div>

          {!isCompleted && card.status === "registration_open" && card.joinClosesAt && (
            <div className="mt-6">
              <div className="mb-3 text-xs font-medium text-zinc-500">Join window closes in</div>
              <CountdownTimer endsAt={card.joinClosesAt} />
            </div>
          )}
          {card.status === "active" && (
            <div className="mt-6">
              <div className="mb-3 text-xs font-medium text-zinc-500">Competition ends in</div>
              <CountdownTimer endsAt={card.endsAt} />
            </div>
          )}

          <div className="mt-6 space-y-2">
            <ScoringExplanation scope={card.scoringScope} joined={card.joined} />
          </div>

          <div className="mt-6 flex flex-wrap items-center gap-3">
            {card.joined ? (
              <span className="inline-flex items-center gap-1.5 rounded-full border border-emerald-500/20 bg-emerald-500/10 px-3 py-1.5 text-sm text-emerald-300">
                <CheckCircle2 className="h-4 w-4" />
                Joined
                {status?.entryStatus && ` · ${status.entryStatus}`}
              </span>
            ) : (
              canJoin && (
                <button
                  type="button"
                  disabled={detail.isJoining || (eligibility && !eligibility.eligible)}
                  onClick={() => detail.join()}
                  className="rounded-lg border border-violet-500/20 bg-violet-500/[0.08] px-4 py-2.5 text-sm font-medium text-violet-200 transition hover:bg-violet-500/[0.12] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-violet-400/40 disabled:cursor-not-allowed disabled:opacity-60"
                >
                  {detail.isJoining ? "Joining…" : "Join competition"}
                </button>
              )
            )}
            {canWithdraw && (
              <button
                type="button"
                disabled={detail.isWithdrawing}
                onClick={() => detail.withdraw()}
                className="rounded-lg border border-zinc-700 bg-zinc-900 px-4 py-2.5 text-sm font-medium text-zinc-300 transition hover:bg-zinc-800 disabled:cursor-not-allowed disabled:opacity-60"
              >
                {detail.isWithdrawing ? "Withdrawing…" : "Withdraw"}
              </button>
            )}
          </div>

          {status && card.joined && status.currentRank > 0 && (
            <div className="mt-6 grid grid-cols-2 gap-3 sm:grid-cols-3">
              <div className="rounded-xl border border-zinc-800 bg-zinc-950/50 px-4 py-3">
                <div className="text-xs text-zinc-500">Your rank</div>
                <div className="font-mono text-lg tabular-nums text-zinc-50">
                  #{status.currentRank}
                </div>
              </div>
              <div className="rounded-xl border border-zinc-800 bg-zinc-950/50 px-4 py-3">
                <div className="text-xs text-zinc-500">Return</div>
                <div className="font-mono text-lg tabular-nums text-zinc-50">
                  {formatSignedPercent(status.returnPercentage)}
                </div>
              </div>
            </div>
          )}
        </motion.header>

        {!card.joined && eligibility && (
          <section className="mb-8 rounded-2xl border border-zinc-800 bg-zinc-900/50 p-5">
            <h2 className="mb-4 text-sm font-semibold text-zinc-100">Eligibility</h2>
            <ul className="space-y-2">
              {eligibility.rules.map((rule) => (
                <li key={rule.code} className="flex items-start gap-2 text-sm">
                  {rule.passed ? (
                    <ShieldCheck className="mt-0.5 h-4 w-4 shrink-0 text-emerald-400" />
                  ) : (
                    <ShieldX className="mt-0.5 h-4 w-4 shrink-0 text-rose-400" />
                  )}
                  <span className={cn(rule.passed ? "text-zinc-300" : "text-zinc-400")}>
                    {rule.label}
                    <span className="ml-2 font-mono text-xs text-zinc-500">
                      {rule.actual} / {rule.required}
                    </span>
                    {rule.reason && (
                      <span className="ml-2 text-xs text-zinc-500">{rule.reason}</span>
                    )}
                  </span>
                </li>
              ))}
            </ul>
            {!eligibility.eligible && (
              <p className="mt-4 flex items-center gap-1.5 text-xs text-rose-300">
                <XCircle className="h-3.5 w-3.5" />
                You don't currently meet every requirement to join.
              </p>
            )}
          </section>
        )}

        {isCompleted ? (
          <CompetitionLeaderboardTable
            title="Final results"
            subtitle="Immutable — never repriced with current market data"
            rows={results}
            isError={Boolean(detail.error)}
            emptyLabel="No participants scored"
          />
        ) : (
          <>
            <CompetitionLeaderboardTable
              title="Leaderboard"
              subtitle={
                leaderboard?.unavailable
                  ? "Rankings are being prepared"
                  : leaderboard?.valuedAt
                    ? `Updated ${new Date(leaderboard.valuedAt).toLocaleTimeString()}`
                    : undefined
              }
              rows={leaderboard?.unavailable ? [] : leaderboard?.entries}
              isLoading={detail.isLoadingLeaderboard}
              emptyLabel={
                leaderboard?.unavailable ? "Rankings are being prepared" : "No rankings yet"
              }
              emptyHint={
                leaderboard?.unavailable ? "Check back shortly." : "Join to appear here."
              }
            />
            {(detail.hasNextLeaderboardPage || detail.leaderboardCursor !== undefined) && (
              <div className="mt-4 flex justify-center gap-2">
                {detail.leaderboardCursor !== undefined && (
                  <button
                    type="button"
                    onClick={detail.resetLeaderboardPage}
                    className="rounded-lg border border-zinc-700 px-3 py-1.5 text-xs text-zinc-300 hover:bg-zinc-800"
                  >
                    Back to top
                  </button>
                )}
                {detail.hasNextLeaderboardPage && (
                  <button
                    type="button"
                    onClick={detail.nextLeaderboardPage}
                    className="rounded-lg border border-zinc-700 px-3 py-1.5 text-xs text-zinc-300 hover:bg-zinc-800"
                  >
                    Next page
                  </button>
                )}
              </div>
            )}
          </>
        )}
      </main>
    </div>
  );
}
