import { Skeleton } from "@/components/ui/skeleton";

export function LeaderboardSkeleton() {
  return (
    <div className="overflow-hidden rounded-xl border border-zinc-800/90 bg-zinc-950/35 p-3">
      <Skeleton className="mb-3 h-8 w-full rounded-lg" />
      <div className="space-y-2">
        {Array.from({ length: 8 }, (_, index) => (
          <Skeleton key={index} className="h-16 w-full rounded-lg" />
        ))}
      </div>
    </div>
  );
}
