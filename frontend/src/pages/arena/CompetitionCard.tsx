import { motion } from "framer-motion";
import { ArrowUpRight, CalendarDays, Check, Clock3, ShieldCheck, Users } from "lucide-react";
import { Link } from "react-router-dom";

import { categoryLabel, statusLabel } from "@/pages/arena/arenaUtils";
import type { ArenaCompetitionCard as ArenaCompetitionCardType } from "@/types/arena";
import { cn } from "@/utils/cn";

const STATUS_STYLE: Record<string, { badge: string; accent: string; glow: string }> = {
  active: {
    badge: "border-emerald-300/20 bg-emerald-400/[0.09] text-emerald-200",
    accent: "from-emerald-300 via-teal-400 to-transparent",
    glow: "bg-emerald-400/10",
  },
  registration_open: {
    badge: "border-violet-300/20 bg-violet-400/[0.10] text-violet-200",
    accent: "from-violet-300 via-fuchsia-400 to-transparent",
    glow: "bg-violet-400/10",
  },
  registration_closed: {
    badge: "border-sky-300/20 bg-sky-400/[0.09] text-sky-200",
    accent: "from-sky-300 via-indigo-400 to-transparent",
    glow: "bg-sky-400/10",
  },
  upcoming: {
    badge: "border-sky-300/20 bg-sky-400/[0.09] text-sky-200",
    accent: "from-sky-300 via-indigo-400 to-transparent",
    glow: "bg-sky-400/10",
  },
  published: {
    badge: "border-sky-300/20 bg-sky-400/[0.09] text-sky-200",
    accent: "from-sky-300 via-indigo-400 to-transparent",
    glow: "bg-sky-400/10",
  },
  completed: {
    badge: "border-amber-200/20 bg-amber-300/[0.08] text-amber-100",
    accent: "from-amber-200 via-orange-300 to-transparent",
    glow: "bg-amber-300/10",
  },
};

function formatDate(value: string): string {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return "Date unavailable";
  return new Intl.DateTimeFormat("en-GB", { day: "2-digit", month: "short", year: "numeric" }).format(date);
}

function actionLabel(competition: ArenaCompetitionCardType): string {
  if (competition.status === "completed") return "See results";
  if (competition.joined) return "View standing";
  if (competition.status === "registration_open") return "Join competition";
  return "View competition";
}

function eligibilityLabel(competition: ArenaCompetitionCardType): string {
  if (competition.joined) return competition.entryStatus ? competition.entryStatus.replaceAll("_", " ") : "Entry confirmed";
  if (competition.status === "registration_open") return "Eligibility checked on entry";
  return "Rules available in details";
}

export function CompetitionCard({ competition, index = 0 }: { competition: ArenaCompetitionCardType; index?: number }) {
  const style = STATUS_STYLE[competition.status] ?? {
    badge: "border-zinc-600/50 bg-zinc-700/20 text-zinc-300",
    accent: "from-zinc-400 via-zinc-600 to-transparent",
    glow: "bg-zinc-500/10",
  };

  return (
    <motion.article
      initial={{ opacity: 0, y: 14 }}
      animate={{ opacity: 1, y: 0 }}
      transition={{ duration: 0.28, delay: Math.min(index * 0.045, 0.24) }}
      className="h-full"
    >
      <Link
        to={`/arena/competitions/${competition.id}`}
        aria-label={`${actionLabel(competition)}: ${competition.name}`}
        className="group relative flex h-full min-h-[360px] flex-col overflow-hidden rounded-[1.4rem] border border-white/[0.09] bg-[linear-gradient(145deg,rgba(39,39,42,0.74),rgba(9,9,11,0.9))] p-5 shadow-[0_18px_55px_-30px_rgba(0,0,0,0.9)] backdrop-blur-sm transition duration-300 hover:-translate-y-1 hover:border-white/[0.17] hover:shadow-[0_28px_65px_-32px_rgba(0,0,0,0.95)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-violet-300 focus-visible:ring-offset-2 focus-visible:ring-offset-zinc-950 sm:p-6"
      >
        <div className={cn("pointer-events-none absolute -right-16 -top-20 h-44 w-44 rounded-full blur-3xl transition duration-500 group-hover:scale-125", style.glow)} />
        <div className={cn("absolute inset-x-0 top-0 h-px bg-gradient-to-r", style.accent)} />

        <div className="relative flex items-start justify-between gap-3">
          <div className="min-w-0">
            <div className="flex flex-wrap items-center gap-2">
              <span className="font-mono text-[10px] font-medium uppercase tracking-[0.2em] text-violet-300/90">
                {categoryLabel(competition.category) || "Open field"}
              </span>
              {competition.isLegacy && <span className="text-[10px] uppercase tracking-wider text-amber-200/70">Legacy sprint</span>}
            </div>
            <h3 className="arena-display mt-3 line-clamp-2 text-[1.35rem] font-semibold leading-tight tracking-[-0.02em] text-zinc-50 transition group-hover:text-white">
              {competition.name}
            </h3>
          </div>
          <span className={cn("inline-flex shrink-0 items-center rounded-full border px-2.5 py-1 text-[10px] font-semibold uppercase tracking-[0.1em]", style.badge)}>
            {statusLabel(competition.status)}
          </span>
        </div>

        <p className="relative mt-4 line-clamp-3 min-h-[3.75rem] text-sm leading-5 text-zinc-400">
          {competition.description || "A focused portfolio competition scored under a published, immutable ruleset."}
        </p>

        <div className="relative mt-5 grid grid-cols-2 gap-x-4 gap-y-3 border-y border-white/[0.07] py-4 text-xs">
          <div>
            <span className="flex items-center gap-1.5 text-zinc-600"><CalendarDays className="h-3.5 w-3.5" /> Starts</span>
            <span className="mt-1 block font-mono text-[11px] text-zinc-300">{formatDate(competition.startsAt)}</span>
          </div>
          <div>
            <span className="flex items-center gap-1.5 text-zinc-600"><Clock3 className="h-3.5 w-3.5" /> Ends</span>
            <span className="mt-1 block font-mono text-[11px] text-zinc-300">{formatDate(competition.endsAt)}</span>
          </div>
          <div>
            <span className="flex items-center gap-1.5 text-zinc-600"><Users className="h-3.5 w-3.5" /> Participants</span>
            <span className="mt-1 block font-mono text-[11px] text-zinc-300">{competition.participantCount.toLocaleString()} joined</span>
          </div>
          <div>
            <span className="flex items-center gap-1.5 text-zinc-600"><ShieldCheck className="h-3.5 w-3.5" /> Eligibility</span>
            <span className="mt-1 block truncate text-[11px] capitalize text-zinc-300">{eligibilityLabel(competition)}</span>
          </div>
        </div>

        <div className="relative mt-auto flex items-center justify-between gap-3 pt-5">
          {competition.joined ? (
            <span className="inline-flex items-center gap-1.5 text-xs font-semibold text-emerald-300"><Check className="h-3.5 w-3.5" /> Joined</span>
          ) : (
            <span className="text-xs text-zinc-600">{competition.scoringScope?.replaceAll("_", " ") || "Published rules"}</span>
          )}
          <span className="inline-flex items-center gap-2 rounded-full bg-zinc-50 px-4 py-2 text-xs font-bold text-zinc-950 shadow-[0_8px_22px_-10px_rgba(255,255,255,0.45)] transition group-hover:bg-white group-hover:pr-3.5">
            {actionLabel(competition)}
            <ArrowUpRight className="h-3.5 w-3.5 transition-transform group-hover:-translate-y-0.5 group-hover:translate-x-0.5" />
          </span>
        </div>
      </Link>
    </motion.article>
  );
}
