// Shimmering placeholder blocks shown during first load instead of a bare
// spinner, so the layout doesn't jump when data arrives.

interface SkeletonProps {
  className?: string;
}

export function Skeleton({ className = "" }: SkeletonProps) {
  return (
    <div
      className={`rounded-lg ${className}`}
      style={{
        background:
          "linear-gradient(90deg, var(--color-base-200), var(--color-base-300), var(--color-base-200))",
        backgroundSize: "200% 100%",
        animation: "seed-shimmer 1.3s linear infinite",
      }}
    />
  );
}

// A row of stat-card skeletons for the dashboard first paint.
export function StatCardSkeleton() {
  return (
    <div className="surface p-4">
      <Skeleton className="h-3 w-20" />
      <Skeleton className="mt-3 h-7 w-16" />
    </div>
  );
}
