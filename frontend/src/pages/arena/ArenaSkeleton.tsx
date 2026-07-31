import { Skeleton } from "@/components/ui/skeleton";

const card = "rounded-2xl border border-zinc-800 bg-zinc-900/40 p-5";

function CardGridSkeleton({ count }: { count: number }) {
  return (
    <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3">
      {Array.from({ length: count }, (_, i) => i).map((i) => (
        <div key={i} className={card}>
          <Skeleton className="h-4 w-16" />
          <Skeleton className="mt-3 h-5 w-40" />
          <Skeleton className="mt-3 h-8 w-full" />
          <Skeleton className="mt-5 h-4 w-24" />
        </div>
      ))}
    </div>
  );
}

export function ArenaSkeleton() {
  return (
    <CardGridSkeleton count={3} />
  );
}
