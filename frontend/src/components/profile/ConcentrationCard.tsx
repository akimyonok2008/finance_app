import type { Concentration } from "@/types/profile";

function Value({ label, value }: { label: string; value: string }) {
  return (
    <div>
      <div className="font-mono text-lg font-semibold tabular-nums text-zinc-100">{value}</div>
      <div className="mt-1 text-[11px] text-zinc-500">{label}</div>
    </div>
  );
}

export function ConcentrationCard({ concentration }: { concentration?: Concentration }) {
  return (
    <section className="rounded-2xl border border-zinc-800 bg-zinc-900/50 p-5">
      <h2 className="text-sm font-semibold text-zinc-100">Concentration</h2>
      <p className="mt-1 text-xs leading-5 text-zinc-500">How much public portfolio weight sits in the largest positions.</p>
      <div className="mt-5 grid grid-cols-3 gap-4">
        <Value label="Holdings" value={concentration?.position_count?.toString() ?? "—"} />
        <Value label="Largest" value={concentration?.largest_weight_percentage !== undefined ? `${concentration.largest_weight_percentage.toFixed(2)}%` : "—"} />
        <Value label="Top 3" value={concentration?.top3_weight_percentage !== undefined ? `${concentration.top3_weight_percentage.toFixed(2)}%` : "—"} />
      </div>
    </section>
  );
}
