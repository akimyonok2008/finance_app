import { Skeleton } from "@/components/ui/skeleton";

const cardBase =
  "relative overflow-hidden rounded-2xl border border-zinc-800 bg-zinc-900/40 p-5";

export function DashboardSkeleton() {
  return (
    <div className="grid grid-cols-1 gap-5 lg:grid-cols-12">
      {/* Performance chart card */}
      <div className={`${cardBase} lg:col-span-8`}>
        <Skeleton className="mb-3 h-4 w-32" />
        <Skeleton className="mb-1 h-10 w-40" />
        <Skeleton className="mb-6 h-4 w-24" />
        <Skeleton className="h-52 w-full rounded-2xl" />
      </div>

      {/* Sprint widget */}
      <div className={`${cardBase} lg:col-span-4`}>
        <Skeleton className="mb-3 h-4 w-20" />
        <Skeleton className="mb-4 h-8 w-28" />
        <div className="space-y-3">
          <Skeleton className="h-14 w-full rounded-2xl" />
          <Skeleton className="h-14 w-full rounded-2xl" />
        </div>
        <Skeleton className="mt-5 h-10 w-full rounded-xl" />
      </div>

      {/* Ranking widget */}
      <div className={`${cardBase} lg:col-span-4`}>
        <Skeleton className="mb-3 h-4 w-28" />
        <Skeleton className="mb-1 h-12 w-20" />
        <Skeleton className="mb-4 h-4 w-32" />
        <Skeleton className="h-8 w-24 rounded-full" />
      </div>

      {/* Trophy case */}
      <div className={`${cardBase} lg:col-span-8`}>
        <Skeleton className="mb-4 h-4 w-28" />
        <div className="flex gap-3">
          {[0, 1, 2, 3].map((i) => (
            <Skeleton key={i} className="h-20 w-20 rounded-2xl" />
          ))}
        </div>
      </div>
    </div>
  );
}
