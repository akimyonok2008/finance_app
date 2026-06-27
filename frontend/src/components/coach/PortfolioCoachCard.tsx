import { useState } from "react";
import { Loader2, Sparkles } from "lucide-react";

import { ApiError } from "@/api/client";
import { CoachModeButton } from "@/components/coach/CoachModeButton";
import { CoachResult } from "@/components/coach/CoachResult";
import { COACH_MODES, type CoachMode } from "@/types/coach";
import { useCoach } from "@/hooks/useCoach";

/** Maps a thrown error to a calm, specific message. */
function friendlyError(err: unknown): string {
  if (err instanceof ApiError) {
    const msg = err.message.toLowerCase();
    if (err.status === 400 && msg.includes("no positions")) {
      return "Add positions before asking Portfolio Coach to analyze your portfolio.";
    }
    if (err.status === 400 && msg.includes("mode")) {
      return "This coach mode is not available yet.";
    }
    if (err.status === 404) {
      return "Portfolio Coach is not available on this server yet.";
    }
    if (err.status === 0) {
      return "Portfolio Coach could not reach the server. Try again.";
    }
    return "Portfolio Coach could not generate analysis. Try again.";
  }
  return "Portfolio Coach could not generate analysis. Try again.";
}

export function PortfolioCoachCard() {
  const coach = useCoach();
  const [activeMode, setActiveMode] = useState<CoachMode | null>(null);

  const run = (mode: CoachMode) => {
    setActiveMode(mode);
    coach.mutate(mode);
  };

  const isPending = coach.isPending;

  return (
    <section className="rounded-2xl border border-zinc-800 bg-zinc-900/50 p-5">
      {/* Header */}
      <div className="mb-1 flex items-center gap-2">
        <span className="grid h-8 w-8 place-items-center rounded-lg border border-violet-500/20 bg-violet-500/10">
          <Sparkles className="h-4 w-4 text-violet-300" />
        </span>
        <h2 className="text-base font-medium text-zinc-100">Portfolio Coach</h2>
      </div>
      <p className="text-sm leading-relaxed text-zinc-400">
        Private portfolio, technical, fundamental, and top-10 analysis.
      </p>
      <p className="mt-1.5 text-xs text-zinc-500">
        Private to you. Top-10 comparisons use public symbols and weights only.
      </p>

      {/* Mode buttons */}
      <div className="mt-4 grid grid-cols-1 gap-2 sm:grid-cols-2">
        {COACH_MODES.map((meta) => (
          <CoachModeButton
            key={meta.mode}
            meta={meta}
            active={activeMode === meta.mode}
            disabled={isPending}
            onSelect={() => run(meta.mode)}
          />
        ))}
      </div>

      {/* Result area */}
      <div className="mt-4">
        {isPending && (
          <div className="flex items-center gap-2 rounded-lg border border-zinc-800 bg-zinc-900/40 px-3 py-3 text-sm text-zinc-400">
            <Loader2 className="h-4 w-4 animate-spin text-violet-300" />
            Building private portfolio readout…
          </div>
        )}

        {!isPending && coach.isError && (
          <div className="space-y-2 rounded-lg border border-rose-500/20 bg-rose-500/5 px-3 py-3">
            <p className="text-sm text-rose-200">{friendlyError(coach.error)}</p>
            {activeMode && (
              <button
                type="button"
                onClick={() => run(activeMode)}
                className="text-xs font-medium text-zinc-300 underline underline-offset-2 hover:text-zinc-100 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-zinc-500"
              >
                Try again
              </button>
            )}
          </div>
        )}

        {!isPending && !coach.isError && coach.data && (
          <CoachResult data={coach.data} />
        )}

        {!isPending && !coach.isError && !coach.data && (
          <p className="rounded-lg border border-dashed border-zinc-800 px-3 py-3 text-sm text-zinc-500">
            Pick a mode above to generate a private, educational readout.
            Educational analysis only — not financial advice.
          </p>
        )}
      </div>
    </section>
  );
}
