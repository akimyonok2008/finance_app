import { gainLossColor } from "@/utils/gainLoss";
import { formatPercent } from "@/utils/formatPercent";
import type { PublicProfile } from "@/types/profile";

function Metric({ label, value, className = "text-zinc-100" }: { label: string; value: string; className?: string }) {
  return (
    <div className="rounded-xl border border-zinc-800 bg-zinc-900/40 p-4">
      <p className="text-[10px] font-medium uppercase tracking-widest text-zinc-500">{label}</p>
      <p className={`mt-2 font-mono text-xl font-semibold tabular-nums ${className}`}>{value}</p>
    </div>
  );
}

export function ProfilePerformanceCards({ profile }: { profile: PublicProfile }) {
  return (
    <section className="grid grid-cols-2 gap-3 lg:grid-cols-4" aria-label="Profile performance">
      <Metric label="Portfolio index" value={profile.portfolio_index?.toFixed(2) ?? "—"} />
      <Metric label="Return" value={formatPercent(profile.return_percentage)} className={gainLossColor(profile.return_percentage)} />
      <Metric label="Global rank" value={profile.global_rank ? `#${profile.global_rank}` : "—"} />
      <Metric label="Sprint rank" value={profile.sprint_rank ? `#${profile.sprint_rank}` : "—"} />
    </section>
  );
}
