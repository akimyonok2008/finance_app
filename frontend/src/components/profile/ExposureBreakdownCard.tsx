import type { Exposure } from "@/types/profile";

function ExposureList({ title, items }: { title: string; items: Exposure[] }) {
  return (
    <div>
      <h3 className="text-xs font-medium uppercase tracking-widest text-zinc-500">{title}</h3>
      <div className="mt-3 space-y-3">
        {items.length === 0 ? (
          <p className="text-sm text-zinc-600">No exposure data shared.</p>
        ) : (
          items.map((item) => (
            <div key={item.name}>
              <div className="mb-1.5 flex items-center justify-between gap-3 text-xs">
                <span className="min-w-0 truncate capitalize text-zinc-300">{item.name.replaceAll("_", " ")}</span>
                <span className="font-mono tabular-nums text-zinc-400">{item.weight_percentage.toFixed(2)}%</span>
              </div>
              <div className="h-1.5 overflow-hidden rounded-full bg-zinc-800">
                <div className="h-full rounded-full bg-zinc-500" style={{ width: `${Math.min(100, Math.max(0, item.weight_percentage))}%` }} />
              </div>
            </div>
          ))
        )}
      </div>
    </div>
  );
}

export function ExposureBreakdownCard({ assetTypes, currencies }: { assetTypes: Exposure[]; currencies: Exposure[] }) {
  return (
    <section className="rounded-2xl border border-zinc-800 bg-zinc-900/50 p-5">
      <h2 className="text-sm font-semibold text-zinc-100">Exposure breakdown</h2>
      <div className="mt-5 grid gap-7 sm:grid-cols-2">
        <ExposureList title="Asset type" items={assetTypes} />
        <ExposureList title="Currency" items={currencies} />
      </div>
    </section>
  );
}
