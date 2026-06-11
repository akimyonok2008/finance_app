import { motion } from "framer-motion";
import {
  ArrowRight,
  Award,
  BarChart2,
  Flame,
  Star,
  Target,
  Trophy,
  Zap,
} from "lucide-react";
import type { LucideIcon } from "lucide-react";
import { Link } from "react-router-dom";

import type { Achievement } from "@/types/dashboard";
import { cn } from "@/utils/cn";

/** Map backend icon_key values to Lucide icons. */
const ICON_MAP: Record<string, LucideIcon> = {
  trophy: Trophy,
  star: Star,
  zap: Zap,
  fire: Flame,
  flame: Flame,
  target: Target,
  award: Award,
  chart: BarChart2,
};

function BadgeIcon({ iconKey }: { iconKey: string }) {
  const Icon = ICON_MAP[iconKey?.toLowerCase()] ?? Award;
  return <Icon className="h-6 w-6" />;
}

type Props = {
  achievements: Achievement[] | null;
  isLoading: boolean;
  isError: boolean;
};

export function TrophyCaseWidget({ achievements, isLoading, isError }: Props) {
  const unlocked = (achievements ?? [])
    .filter((a) => a.unlocked && a.unlocked_at)
    .sort(
      (a, b) =>
        new Date(b.unlocked_at!).getTime() - new Date(a.unlocked_at!).getTime(),
    )
    .slice(0, 4);

  return (
    <motion.div
      className="relative overflow-hidden rounded-2xl border border-zinc-800 bg-zinc-900/50 p-5 shadow-sm shadow-black/20 transition-colors hover:border-zinc-700 hover:bg-zinc-900/70"
      whileHover={{ y: -2 }}
      transition={{ duration: 0.18 }}
    >
      {/* Header */}
      <div className="mb-4 flex items-center justify-between">
        <div className="flex items-center gap-2">
          <div className="grid h-8 w-8 place-items-center rounded-xl border border-amber-400/20 bg-amber-400/10">
            <Trophy className="h-4 w-4 text-amber-300" />
          </div>
          <span className="text-xs font-medium uppercase tracking-widest text-slate-500">
            Trophy Case
          </span>
        </div>
        <Link
          to="/arena"
          className="group flex items-center gap-1 text-xs text-slate-500 transition hover:text-slate-300 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-indigo-400/40"
        >
          View all
          <ArrowRight className="h-3.5 w-3.5 transition-transform group-hover:translate-x-0.5" />
        </Link>
      </div>

      {isError ? (
        <p className="text-sm text-slate-500">Badges could not be loaded.</p>
      ) : unlocked.length === 0 && !isLoading ? (
        <div className="flex flex-col items-center gap-3 py-8 text-center">
          <div className="grid h-12 w-12 place-items-center rounded-2xl border border-white/[0.07] bg-white/[0.03] text-slate-500">
            <Trophy className="h-5 w-5" />
          </div>
          <div>
            <p className="text-sm font-medium text-slate-300">
              No badges unlocked yet
            </p>
            <p className="mt-0.5 text-xs text-slate-500">
              Add a position or join a sprint.
            </p>
          </div>
        </div>
      ) : (
        <div className="flex gap-3 overflow-x-auto pb-1">
          {unlocked.map((badge, i) => {
            const isLatest = i === 0;
            return (
              <motion.div
                key={badge.key}
                whileHover={{ y: -2 }}
                transition={{ duration: 0.18 }}
                title={badge.name}
                className={cn(
                  "flex w-20 shrink-0 flex-col items-center gap-2 rounded-2xl border border-white/[0.07] bg-white/[0.03] px-2 py-4 text-center",
                  isLatest && "border-violet-400/20 bg-violet-400/[0.035]",
                )}
              >
                <div
                  className={cn(
                    "grid h-10 w-10 place-items-center rounded-xl border text-slate-300",
                    isLatest
                      ? "border-violet-400/30 bg-violet-400/10 text-violet-300"
                      : "border-white/10 bg-white/[0.04]",
                  )}
                >
                  <BadgeIcon iconKey={badge.icon_key} />
                </div>
                <span className="line-clamp-2 text-[10px] leading-snug text-slate-400">
                  {badge.name}
                </span>
              </motion.div>
            );
          })}
        </div>
      )}
    </motion.div>
  );
}
