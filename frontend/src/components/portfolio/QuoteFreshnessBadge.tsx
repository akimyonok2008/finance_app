import { cn } from "@/utils/cn";

type Props = {
  provider?: string;
  status?: string;
  isStale?: boolean;
  fetchedAt?: string;
  className?: string;
};

export function QuoteFreshnessBadge({
  provider,
  status,
  isStale,
  fetchedAt,
  className,
}: Props) {
  const label = quoteFreshnessLabel({ provider, status, isStale, fetchedAt });
  const stale = isStale || status === "provider_unavailable" || status === "rate_limited";
  return (
    <span
      className={cn(
        "inline-flex items-center rounded-full border px-2 py-0.5 text-[10px] font-medium uppercase tracking-wide",
        stale
          ? "border-amber-400/25 bg-amber-400/10 text-amber-200"
          : "border-zinc-800 bg-zinc-900/70 text-zinc-400",
        className,
      )}
    >
      {label}
    </span>
  );
}

function quoteFreshnessLabel({
  provider,
  status,
  isStale,
  fetchedAt,
}: {
  provider?: string;
  status?: string;
  isStale?: boolean;
  fetchedAt?: string;
}) {
  if (provider === "mock") return "Mock data";
  if (status === "provider_unavailable") return "Provider unavailable";
  if (status === "rate_limited") return "Cached";
  if (isStale) return "Stale";
  if (fetchedAt) return `Updated ${relativeMinutes(fetchedAt)}`;
  return provider ? provider : "Market data";
}

function relativeMinutes(value: string) {
  const then = new Date(value).getTime();
  if (Number.isNaN(then)) return "recently";
  const minutes = Math.max(0, Math.round((Date.now() - then) / 60_000));
  if (minutes < 1) return "just now";
  if (minutes === 1) return "1m ago";
  return `${minutes}m ago`;
}
