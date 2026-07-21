import { useEffect, useRef, useState } from "react";

// A resilient polling primitive:
//   - keeps the last good value when a fetch fails (never blanks the screen)
//   - exposes `isStale` so the UI can flag "you're looking at old data"
//   - backs off exponentially on repeated failures, then resumes at the base
//     interval on the next success
//   - tracks browser offline state and refetches immediately on reconnect
//
// Requests never overlap: a slow tick is skipped rather than stacked.

export interface PollingState<T> {
  data: T | null;
  error: boolean;
  isStale: boolean; // we have data, but the most recent fetch failed
  isOffline: boolean;
  lastUpdated: number | null; // epoch ms of the last successful fetch
  refresh: () => void; // force an immediate fetch
}

export interface PollingOptions {
  baseIntervalMs?: number;
  maxIntervalMs?: number;
}

export function usePolling<T>(
  fetcher: () => Promise<T>,
  { baseIntervalMs = 5000, maxIntervalMs = 30000 }: PollingOptions = {},
): PollingState<T> {
  const [data, setData] = useState<T | null>(null);
  const [error, setError] = useState(false);
  const [lastUpdated, setLastUpdated] = useState<number | null>(null);
  const [isOffline, setIsOffline] = useState(
    typeof navigator !== "undefined" ? !navigator.onLine : false,
  );

  const fetcherRef = useRef(fetcher);
  fetcherRef.current = fetcher;

  const timer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const inflight = useRef(false);
  const failures = useRef(0);
  const stopped = useRef(false);
  // Bumped to trigger an out-of-band immediate refresh.
  const [nudge, setNudge] = useState(0);

  useEffect(() => {
    stopped.current = false;

    const backoff = () =>
      Math.min(maxIntervalMs, baseIntervalMs * 2 ** Math.min(failures.current, 5));

    const schedule = (delay: number) => {
      if (stopped.current) return;
      if (timer.current) clearTimeout(timer.current);
      timer.current = setTimeout(tick, delay);
    };

    const tick = async () => {
      if (inflight.current || stopped.current) return;
      inflight.current = true;
      try {
        const next = await fetcherRef.current();
        if (stopped.current) return;
        setData(next);
        setError(false);
        setLastUpdated(Date.now());
        failures.current = 0;
        schedule(baseIntervalMs);
      } catch {
        if (stopped.current) return;
        setError(true);
        failures.current += 1;
        schedule(backoff());
      } finally {
        inflight.current = false;
      }
    };

    const onOnline = () => {
      setIsOffline(false);
      failures.current = 0;
      schedule(0); // reconnected — refetch now
    };
    const onOffline = () => setIsOffline(true);

    window.addEventListener("online", onOnline);
    window.addEventListener("offline", onOffline);

    tick();

    return () => {
      stopped.current = true;
      if (timer.current) clearTimeout(timer.current);
      window.removeEventListener("online", onOnline);
      window.removeEventListener("offline", onOffline);
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [baseIntervalMs, maxIntervalMs, nudge]);

  return {
    data,
    error,
    isStale: error && data !== null,
    isOffline,
    lastUpdated,
    refresh: () => setNudge((n) => n + 1),
  };
}
