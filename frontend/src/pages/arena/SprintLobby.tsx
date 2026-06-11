import { motion } from "framer-motion";
import { CheckCircle2, Zap } from "lucide-react";

import { CountdownTimer } from "@/pages/arena/CountdownTimer";
import type { ActiveSprint } from "@/types/arena";

export function SprintLobby({
  sprint,
  isError,
  onJoin,
  isJoining,
}: {
  sprint: ActiveSprint | undefined;
  isLoading?: boolean;
  isError?: boolean;
  onJoin: (sprintId: string) => void;
  isJoining?: boolean;
}) {
  return (
    <motion.section
      initial={{ opacity: 0, y: 14 }}
      animate={{ opacity: 1, y: 0 }}
      transition={{ duration: 0.18 }}
      className="relative overflow-hidden rounded-2xl border border-zinc-800 bg-zinc-900/50 p-5 shadow-sm shadow-black/20"
      aria-labelledby="sprint-lobby-heading"
    >
      <div className="pointer-events-none absolute inset-x-0 top-0 h-px bg-violet-500/30" />
      <div className="relative">
        <div className="flex flex-wrap items-start justify-between gap-4">
          <div>
            <div className="mb-2 flex items-center gap-2 text-xs font-medium text-violet-300">
              <Zap className="h-4 w-4" />
              Live Sprint
            </div>
            <h2 id="sprint-lobby-heading" className="text-xl font-medium tracking-tight">
              {sprint?.name ?? "No active sprint"}
            </h2>
          </div>
          {sprint && (
            <span
              className={
                sprint.isJoined
                  ? "rounded-full border border-emerald-500/20 bg-emerald-500/10 px-3 py-1 text-xs text-emerald-300"
                  : "rounded-full border border-zinc-700 bg-zinc-900 px-3 py-1 text-xs text-zinc-400"
              }
            >
              {sprint.isJoined ? "Joined" : "Not joined"}
            </span>
          )}
        </div>

        {isError ? (
          <p className="mt-6 text-sm text-rose-300">
            Sprint data is temporarily unavailable.
          </p>
        ) : !sprint ? (
          <p className="mt-6 text-sm text-zinc-400">
            The next sprint will appear here.
          </p>
        ) : (
          <>
            <div className="mt-6">
              <div className="mb-3 text-xs font-medium text-zinc-500">
                Time remaining
              </div>
              <CountdownTimer endsAt={sprint.endsAt} />
            </div>

            <div className="mt-6 grid gap-6 md:grid-cols-[1fr_auto] md:items-end">
              <div>
                <div className="mb-3 text-xs font-medium text-zinc-500">
                  Sprint rules
                </div>
                <ul className="space-y-2">
                  {(sprint.rules.length
                    ? sprint.rules
                    : ["Standard sprint rules apply."]
                  ).map((rule) => (
                    <li
                      key={rule}
                      className="flex items-start gap-2 text-sm text-zinc-400"
                    >
                      <CheckCircle2 className="mt-0.5 h-4 w-4 shrink-0 text-violet-400" />
                      {rule}
                    </li>
                  ))}
                </ul>
              </div>

              <div className="md:max-w-xs">
                <p className="mb-3 text-sm leading-relaxed text-zinc-400">
                  {sprint.isJoined
                    ? "Rank updates with performance."
                    : "Baseline locks at entry."}
                </p>
                {!sprint.isJoined && (
                  <button
                    type="button"
                    disabled={isJoining}
                    onClick={() => onJoin(sprint.id)}
                    className="w-full rounded-lg border border-violet-500/20 bg-violet-500/[0.08] px-4 py-2.5 text-sm font-medium text-violet-200 transition hover:bg-violet-500/[0.12] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-violet-400/40 disabled:cursor-not-allowed disabled:opacity-60"
                  >
                    {isJoining ? "Joining..." : "Join Sprint"}
                  </button>
                )}
              </div>
            </div>
          </>
        )}
      </div>
    </motion.section>
  );
}
