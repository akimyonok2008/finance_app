export function StrategyTagBadge({
  strategy,
}: {
  strategy?: string | null;
}) {
  return (
    <span className="inline-flex rounded-full border border-zinc-700 bg-zinc-900/70 px-2.5 py-1 text-xs text-zinc-300">
      {strategy || "Balanced"}
    </span>
  );
}
