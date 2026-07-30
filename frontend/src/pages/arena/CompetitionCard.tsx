import { motion } from "framer-motion";
import { ArrowRight, CheckCircle2, Clock, Users, Zap } from "lucide-react";
import { Link } from "react-router-dom";

import { categoryLabel, statusLabel } from "@/pages/arena/arenaUtils";
import type { ArenaCompetitionCard as ArenaCompetitionCardType } from "@/types/arena";
import { cn } from "@/utils/cn";

const STATUS_STYLE: Record<string, { icon: typeof Zap; className: string }> = {
  active: { icon: Zap, className: "border-violet-500/20 bg-violet-500/10 text-violet-300" },
  registration_open: { icon: Clock, className: "border-sky-500/20 bg-sky-500/10 text-sky-300" },
  registration_closed: { icon: Clock, className: "border-sky-500/20 bg-sky-500/10 text-sky-300" },
  upcoming: { icon: Clock, className: "border-sky-500/20 bg-sky-500/10 text-sky-300" },
  completed: {
    icon: CheckCircle2,
    className: "border-emerald-500/20 bg-emerald-500/10 text-emerald-300",
  },
};

export function CompetitionCard({
  competition,
  index = 0,
}: {
  competition: ArenaCompetitionCardType;
  index?: number;
}) {
  const style = STATUS_STYLE[competition.status] ?? {
    icon: Clock,
    className: "border-zinc-700 bg-zinc-900 text-zinc-400",
  };
  const StatusIcon = style.icon;

  return (
    <motion.div
      initial={{ opacity: 0, y: 10 }}
      animate={{ opacity: 1, y: 0 }}
      transition={{ duration: 0.18, delay: Math.min(index * 0.03, 0.24) }}
    >
      <Link
        to={`/arena/competitions/${competition.id}`}
        className="group block rounded-2xl border border-zinc-800 bg-zinc-900/50 p-5 shadow-sm shadow-black/20 transition hover:border-zinc-700 hover:bg-zinc-900/70 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-violet-400/40"
      >
        <div className="flex items-start justify-between gap-3">
          <div className="min-w-0">
            {competition.category && (
              <div className="mb-2 text-[10px] font-medium uppercase tracking-[0.16em] text-zinc-500">
                {categoryLabel(competition.category)}
              </div>
            )}
            <h3 className="truncate text-base font-medium text-zinc-100">
              {competition.name}
            </h3>
          </div>
          <span
            className={cn(
              "inline-flex shrink-0 items-center gap-1 rounded-full border px-2.5 py-1 text-[11px] font-medium",
              style.className,
            )}
          >
            <StatusIcon className="h-3 w-3" />
            {statusLabel(competition.status)}
          </span>
        </div>

        {competition.description && (
          <p className="mt-3 line-clamp-2 text-sm text-zinc-400">{competition.description}</p>
        )}

        <div className="mt-5 flex items-center justify-between text-xs text-zinc-500">
          <span className="inline-flex items-center gap-1.5">
            <Users className="h-3.5 w-3.5" />
            {competition.participantCount} joined
          </span>
          <span className="inline-flex items-center gap-1 font-medium text-zinc-400 transition group-hover:text-zinc-200">
            {competition.joined ? "View standing" : "View details"}
            <ArrowRight className="h-3.5 w-3.5 transition group-hover:translate-x-0.5" />
          </span>
        </div>

        {competition.joined && (
          <div className="mt-3 inline-flex rounded-full border border-emerald-500/20 bg-emerald-500/10 px-2.5 py-1 text-[11px] text-emerald-300">
            Joined
          </div>
        )}
      </Link>
    </motion.div>
  );
}
