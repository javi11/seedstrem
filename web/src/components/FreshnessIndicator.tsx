import { relativeAge } from "../lib/format";

interface FreshnessProps {
  isStale: boolean;
  isOffline: boolean;
  lastUpdated: number | null;
}

// The small "Live / stale" chip shown in page headers. Green pulse when data is
// fresh; amber when the last poll failed and we're showing cached data.
export function FreshnessIndicator({ isStale, isOffline, lastUpdated }: FreshnessProps) {
  const stale = isStale || isOffline;
  return (
    <span
      className={`badge gap-2 whitespace-nowrap ${
        stale ? "badge-warning" : "badge-success"
      } badge-outline`}
      title={lastUpdated ? `Updated ${relativeAge(lastUpdated)}` : undefined}
    >
      <span
        className="pulse-dot inline-block h-2 w-2 rounded-full"
        style={{
          backgroundColor: "currentColor",
          animation: stale ? undefined : "seed-pulse 2s infinite",
        }}
        aria-hidden
      />
      {stale ? "Stale" : "Live"}
    </span>
  );
}
