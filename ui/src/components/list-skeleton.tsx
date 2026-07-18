import { Skeleton } from "@/components/ui/skeleton";

/** Placeholder rows shown while list/table data loads, preventing layout shift. */
export function ListSkeleton({ rows = 4 }: { rows?: number }) {
  return (
    <div className="space-y-2" aria-busy="true">
      <Skeleton className="h-8 w-1/3" />
      {Array.from({ length: rows }).map((_, index) => (
        <Skeleton key={index} className="h-8 w-full" />
      ))}
    </div>
  );
}
