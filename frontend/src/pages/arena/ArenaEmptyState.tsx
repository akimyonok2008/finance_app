export function ArenaEmptyState({ error, onRetry }: { error?: boolean; onRetry?: () => void }) {
  return (
    <div className="py-16 text-center sm:py-24">
      <h2 className="arena-display text-2xl font-semibold text-zinc-200">
        {error ? "Arena data is temporarily unavailable" : "No competitions available"}
      </h2>
      <p className="mx-auto mt-2 max-w-md text-sm leading-6 text-zinc-500">
        {error ? "Please try again in a moment." : "New competitions will appear here when registration opens."}
      </p>
      {onRetry && (
        <button
          type="button"
          onClick={onRetry}
          className="mt-5 rounded-full border border-white/15 px-5 py-2 text-sm font-semibold text-zinc-200 transition hover:border-white/25 hover:bg-white/[0.05] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-violet-300"
        >
          Try again
        </button>
      )}
    </div>
  );
}
