import { Skeleton } from "@/components/ui/skeleton";

export function ExploreSkeleton() {
  return (
    <div className="grid items-start gap-6 lg:grid-cols-[minmax(0,1fr)_280px]">
      <div className="space-y-7">
        {[0, 1].map((section) => (
          <div key={section}>
            <Skeleton className="h-8 w-52 rounded-lg" />
            <div className="mt-3 grid gap-3 sm:grid-cols-2 xl:grid-cols-3">
              {[0, 1, 2, 3, 4].map((item) => <Skeleton key={item} className="h-40 rounded-xl" />)}
            </div>
          </div>
        ))}
      </div>
      <div className="space-y-4">
        <Skeleton className="h-96 rounded-2xl" />
        <Skeleton className="h-32 rounded-2xl" />
      </div>
    </div>
  );
}
