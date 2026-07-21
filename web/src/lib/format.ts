// Pure formatting helpers shared across pages. Kept dependency-free so they
// are trivial to unit-test.

export function formatBytes(n: number): string {
  if (n <= 0) return "0 B";
  const units = ["B", "KiB", "MiB", "GiB", "TiB"];
  const i = Math.min(Math.floor(Math.log2(n) / 10), units.length - 1);
  return `${(n / 2 ** (10 * i)).toFixed(i === 0 ? 0 : 1)} ${units[i]}`;
}

export function formatSpeed(n: number): string {
  return `${formatBytes(n)}/s`;
}

// How long a completed torrent stays available before cleanup removes it.
// `now` is injectable for deterministic tests.
export function availableUntil(
  t: { progress: number; seed_time: number; seeding_time: number },
  now: number = Date.now(),
): string {
  if (t.progress < 1) return "—";
  if (t.seed_time <= 0) return "Kept";
  const remaining = t.seed_time - t.seeding_time; // seconds
  if (remaining <= 0) return "Removing soon";
  return new Date(now + remaining * 1000).toLocaleString(undefined, {
    month: "short",
    day: "numeric",
    hour: "2-digit",
    minute: "2-digit",
  });
}

// "just now" / "12s ago" / "3m ago" — used by the freshness indicators.
export function relativeAge(fromMs: number, now: number = Date.now()): string {
  const secs = Math.max(0, Math.round((now - fromMs) / 1000));
  if (secs < 5) return "just now";
  if (secs < 60) return `${secs}s ago`;
  const mins = Math.round(secs / 60);
  if (mins < 60) return `${mins}m ago`;
  const hrs = Math.round(mins / 60);
  return `${hrs}h ago`;
}
