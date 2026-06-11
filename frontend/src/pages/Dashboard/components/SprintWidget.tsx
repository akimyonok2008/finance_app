import { motion } from "framer-motion";
import { ArrowRight, Timer, Trophy, Zap } from "lucide-react";
import { Link } from "react-router-dom";

import { getDaysRemaining } from "@/pages/Dashboard/utils/dashboardFormatters";
import type { Competition, MyCompetitionStatus } from "@/types/dashboard";
import { formatPercent } from "@/utils/formatPercent";
import { cn } from "@/utils/cn";

type Props = {
  sprint: Competition | null;
  sprintStatus: MyCompetitionStatus | null;
  isLoading: boolean;
};

function StatPill({
  icon,
  label,
  value,
  accent,
}: {
  icon: React.ReactNode;
  label: string;
  value: string;
  accent?: boolean;
}) {
  return (
    <div
      className={cn(
        "flex items-center gap-3 rounded-2xl border border-white/[0.07] bg-white/[0.03] px-4 py-3",
        accent && "border-violet-400/20 bg-violet-400/5",
      )}
    >
      <div className="grid h-8 w-8 shrink-0 place-items-center rounded-xl border border-white/10 bg-slate-900/60 text-violet-300">
        {icon}
      </div>
      <div>
        <div className="text-[11px] uppercase tracking-widest text-slate-500">
          {label}
        </div>
        <div className="tabular-nums text-sm font-semibold text-slate-100">
          {value}
        </div>
      </div>
    </div>
  );
}

export function SprintWidget({ sprint, sprintStatus, isLoading }: Props) {
  const days = getDaysRemaining(sprint?.ends_at);
  const joined = sprintStatus?.joined ?? false;

  return (
    <motion.div
      className="relative flex h-full flex-col overflow-hidden rounded-2xl border border-zinc-800 bg-zinc-900/50 p-5 shadow-sm shadow-black/20 transition-colors hover:border-zinc-700 hover:bg-zinc-900/70"
      whileHover={{ y: -2 }}
      transition={{ duration: 0.18 }}
    >
      <div className="pointer-events-none absolute inset-x-0 top-0 h-px bg-violet-500/30" />

      {/* Header */}
      <div className="mb-4 flex items-center gap-2">
        <div className="grid h-8 w-8 place-items-center rounded-lg border border-zinc-800 bg-zinc-950/50">
          <Zap className="h-4 w-4 text-violet-300" />
        </div>
        <div>
          <div className="text-xs font-medium text-zinc-500">
            Weekly Sprint
          </div>
          <div className="text-sm font-semibold text-slate-100">
            {sprint?.name ?? "No active sprint"}
          </div>
        </div>
      </div>

      {/* Status pill */}
      {sprint ? (
        <div className="mb-4 inline-flex w-fit items-center gap-1.5 rounded-full border border-violet-400/20 bg-violet-400/[0.04] px-3 py-1 text-xs font-medium text-violet-300">
          <span className="h-1.5 w-1.5 rounded-full bg-violet-400" />
          Active
        </div>
      ) : (
        <div className="mb-4 inline-flex w-fit items-center gap-1.5 rounded-full border border-slate-700 bg-slate-800/50 px-3 py-1 text-xs text-slate-500">
          No active sprint
        </div>
      )}

      {/* Stats */}
      {sprint ? (
        <div className="flex flex-col gap-2 flex-1">
          <StatPill
            icon={<Timer className="h-4 w-4" />}
            label="Days remaining"
            value={days !== null ? `${days}d` : "—"}
          />
          {joined ? (
            <StatPill
              icon={<Trophy className="h-4 w-4" />}
              label="Your sprint rank"
              value={
                sprintStatus?.current_rank
                  ? `#${sprintStatus.current_rank}`
                  : "—"
              }
              accent
            />
          ) : (
            <div className="flex flex-1 items-center justify-center rounded-2xl border border-white/[0.06] bg-white/[0.02] px-4 py-4 text-center text-xs text-slate-500">
              Not joined
            </div>
          )}
          {joined && sprintStatus && (
            <StatPill
              icon={<Zap className="h-4 w-4" />}
              label="Sprint return"
              value={formatPercent(sprintStatus.sprint_return_percentage)}
            />
          )}
        </div>
      ) : (
        <div className="flex flex-1 items-center justify-center text-sm text-slate-500">
          The next sprint will appear here.
        </div>
      )}

      {/* CTA */}
      <Link
        to="/arena"
        className={cn(
          "mt-5 group flex h-10 w-full items-center justify-center gap-2 rounded-lg border border-zinc-700 bg-zinc-950/40 text-sm font-medium text-zinc-300 transition hover:bg-zinc-800/70 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-zinc-500",
        )}
      >
        View Sprint
        <ArrowRight className="h-4 w-4 transition-transform group-hover:translate-x-0.5" />
      </Link>

      {isLoading && (
        <div className="absolute inset-0 rounded-2xl bg-zinc-950/60 backdrop-blur-sm" />
      )}
    </motion.div>
  );
}
