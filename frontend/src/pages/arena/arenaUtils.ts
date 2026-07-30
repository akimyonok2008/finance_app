import { decimalToChartNumber, type DecimalString } from "@/utils/decimal";
import { formatPercent } from "@/utils/formatPercent";
import type { Achievement } from "@/types/arena";

export { formatPercent as formatSignedPercent };

/** Tailwind color class for a decimal-string percentage, without ever coercing it for math. */
export function getPercentClassName(value: DecimalString | null | undefined): string {
  const n = decimalToChartNumber(value);
  if (n === null) return "text-zinc-400";
  if (n > 0) return "text-emerald-500";
  if (n < 0) return "text-rose-500";
  return "text-zinc-400";
}

export function calculateProgressPercent(
  currentProgress: number,
  targetProgress: number,
): number {
  if (targetProgress <= 0) return 0;
  return Math.min(100, Math.round((currentProgress / targetProgress) * 100));
}

export function formatUnlockedDate(value?: string): string {
  if (!value) return "";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return "";
  return `Unlocked ${new Intl.DateTimeFormat("en-GB", {
    day: "2-digit",
    month: "short",
    year: "numeric",
  }).format(date)}`;
}

export function sortAchievements(achievements: Achievement[]): Achievement[] {
  return [...achievements].sort((a, b) => {
    if (a.isUnlocked !== b.isUnlocked) return a.isUnlocked ? -1 : 1;
    if (a.isUnlocked && b.isUnlocked) {
      return (
        new Date(b.unlockedAt ?? 0).getTime() - new Date(a.unlockedAt ?? 0).getTime()
      );
    }
    return (
      calculateProgressPercent(b.currentProgress, b.targetProgress) -
      calculateProgressPercent(a.currentProgress, a.targetProgress)
    );
  });
}

const CATEGORY_LABELS: Record<string, string> = {
  crypto: "Crypto",
  etf: "ETF",
  regional: "Regional",
  open: "Open",
};

export function categoryLabel(category: string | undefined): string {
  if (!category) return "";
  return CATEGORY_LABELS[category] ?? category;
}

/** Buckets a raw backend status (legacy or engine lifecycle) into one of the four Arena sections. */
export function statusBucket(
  status: string,
  joined: boolean,
): "live" | "upcoming" | "completed" | "cancelled" {
  if (status === "completed") return "completed";
  if (status === "cancelled") return "cancelled";
  if (status === "active") return "live";
  if (joined && (status === "registration_open" || status === "registration_closed")) {
    return "upcoming";
  }
  return "upcoming";
}

export function statusLabel(status: string): string {
  switch (status) {
    case "upcoming":
      return "Upcoming";
    case "draft":
      return "Draft";
    case "published":
      return "Published";
    case "registration_open":
      return "Registration open";
    case "registration_closed":
      return "Registration closed";
    case "active":
      return "Live";
    case "finalizing":
      return "Finalizing";
    case "completed":
      return "Completed";
    case "cancelled":
      return "Cancelled";
    default:
      return status;
  }
}
