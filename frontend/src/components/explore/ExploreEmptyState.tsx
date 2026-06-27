export function ExploreEmptyState({ filtered }: { filtered: boolean }) {
  return (
    <div className="rounded-2xl border border-dashed border-zinc-800 bg-zinc-900/20 px-6 py-14 text-center">
      <h2 className="text-base font-semibold text-zinc-100">
        {filtered ? "No public strategies found." : "No public strategies yet."}
      </h2>
      <p className="mt-2 text-sm text-zinc-500">
        {filtered
          ? "Try a different search or symbol filter."
          : "Profiles appear here when users create a strategy baseline and make their profile public."}
      </p>
    </div>
  );
}
