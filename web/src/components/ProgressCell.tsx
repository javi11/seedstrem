// A progress bar plus its numeric percentage, so completion is legible at a
// glance in the torrents table and mobile cards.
export function ProgressCell({ progress }: { progress: number }) {
  const pct = Math.round(Math.min(1, Math.max(0, progress)) * 100);
  const done = pct >= 100;
  return (
    <div className="flex flex-col gap-1">
      <progress
        className={`progress w-full ${done ? "progress-success" : "progress-primary"}`}
        value={pct}
        max={100}
      />
      <span className="text-xs opacity-60 tabular-nums">{pct}%</span>
    </div>
  );
}
