import { RefreshCw, Zap } from "lucide-react";

export function ArenaEmptyState({
  error,
  onRetry,
}: {
  error?: boolean;
  onRetry?: () => void;
}) {
  return (
    <div className="flex flex-col items-center rounded-2xl border border-dashed border-zinc-800 bg-zinc-900/30 px-6 py-16 text-center">
      <div className="grid h-12 w-12 place-items-center rounded-xl border border-zinc-800 bg-zinc-900/50 text-zinc-400">
        <Zap className="h-6 w-6" />
      </div>
      <h2 className="mt-5 text-xl font-semibold tracking-tight">
        {error
          ? "Arena data is temporarily unavailable."
          : "No active sprint right now"}
      </h2>
      <p className="mt-2 max-w-md text-sm text-zinc-400">
        {error
          ? "Please try again in a moment."
          : "The next sprint will appear here."}
      </p>
      {onRetry && (
        <button
          type="button"
          onClick={onRetry}
          className="mt-6 inline-flex items-center gap-2 rounded-xl border border-white/10 bg-white/[0.04] px-4 py-2 text-sm font-medium text-zinc-200 transition hover:bg-white/[0.08] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-violet-400/50"
        >
          <RefreshCw className="h-4 w-4" />
          Retry
        </button>
      )}
    </div>
  );
}
