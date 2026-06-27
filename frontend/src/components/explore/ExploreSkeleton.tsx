import { Skeleton } from "@/components/ui/skeleton";

export function ExploreSkeleton() {
  return (
    <div className="grid gap-6 xl:grid-cols-[1fr_300px]">
      <div className="space-y-6">
        <div className="grid gap-4 md:grid-cols-2">
          {[0, 1, 2, 3].map((item) => <Skeleton key={item} className="h-72 rounded-2xl" />)}
        </div>
        <Skeleton className="h-96 rounded-2xl" />
      </div>
      <Skeleton className="h-96 rounded-2xl" />
    </div>
  );
}
