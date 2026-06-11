import { Skeleton } from "@/components/ui/skeleton";

const card = "rounded-2xl border border-zinc-800 bg-zinc-900/40 p-5";

export function ArenaSkeleton() {
  return (
    <div className="grid grid-cols-1 gap-8 lg:grid-cols-3">
      <section className="space-y-8 lg:col-span-2">
        <div className={card}>
          <Skeleton className="h-5 w-32" />
          <Skeleton className="mt-3 h-8 w-52" />
          <div className="mt-6 grid grid-cols-2 gap-2 sm:grid-cols-4">
            {[0, 1, 2, 3].map((item) => (
              <Skeleton key={item} className="h-16 rounded-2xl" />
            ))}
          </div>
          <Skeleton className="mt-6 h-28 rounded-2xl" />
        </div>
        <div className={card}>
          <Skeleton className="h-10 w-48" />
          <div className="mt-5 space-y-3">
            {[0, 1, 2, 3, 4].map((item) => (
              <Skeleton key={item} className="h-12 rounded-xl" />
            ))}
          </div>
        </div>
      </section>
      <aside className={card}>
        <Skeleton className="h-10 w-40" />
        <div className="mt-5 space-y-4">
          {[0, 1, 2, 3].map((item) => (
            <Skeleton key={item} className="h-36 rounded-2xl" />
          ))}
        </div>
      </aside>
    </div>
  );
}
