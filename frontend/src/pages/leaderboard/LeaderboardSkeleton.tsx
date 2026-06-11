import { Skeleton } from "@/components/ui/skeleton";

const card =
  "rounded-2xl border border-zinc-800 bg-zinc-900/40 p-5";

export function LeaderboardSkeleton() {
  return (
    <div className="space-y-6">
      <div className="grid gap-4 md:grid-cols-3">
        {[0, 1, 2].map((item) => (
          <div key={item} className={card}>
            <Skeleton className="h-16 w-16 rounded-2xl" />
            <Skeleton className="mt-4 h-5 w-36" />
            <Skeleton className="mt-2 h-6 w-24 rounded-full" />
            <div className="mt-5 grid grid-cols-2 gap-2">
              <Skeleton className="h-14 rounded-xl" />
              <Skeleton className="h-14 rounded-xl" />
            </div>
          </div>
        ))}
      </div>
      <div className="hidden overflow-hidden rounded-2xl border border-zinc-800 bg-zinc-900/40 p-5 lg:block">
        <Skeleton className="mb-4 h-8 w-full" />
        <div className="space-y-3">
          {Array.from({ length: 8 }, (_, index) => (
            <Skeleton key={index} className="h-16 w-full rounded-xl" />
          ))}
        </div>
      </div>
      <div className="space-y-3 lg:hidden">
        {Array.from({ length: 5 }, (_, index) => (
          <Skeleton key={index} className="h-36 w-full rounded-2xl" />
        ))}
      </div>
    </div>
  );
}
