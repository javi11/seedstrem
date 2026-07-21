import { SectionProps } from "../types";
import { SectionCard, NumberField, ToggleField } from "../fields";

export function Seeding({ config, update }: SectionProps) {
  return (
    <SectionCard title="Seeding & cleanup">
      <ToggleField
        label="Download the whole torrent (played file first) so a complete copy seeds — better for ratio on private trackers. Off = download only the played file."
        checked={config.seeding.full}
        onChange={(v) => update((c) => (c.seeding.full = v))}
      />
      <div className="grid gap-4 sm:grid-cols-2">
        <NumberField
          label="Seed time before removal (hours, 0 = never)"
          min={0}
          value={config.cleanup.seed_time_hours}
          onChange={(v) => update((c) => (c.cleanup.seed_time_hours = v))}
        />
        <NumberField
          label="Min progress to keep on abandoned playback (%)"
          min={0}
          max={100}
          value={config.cleanup.min_progress_for_cancel_percent}
          onChange={(v) => update((c) => (c.cleanup.min_progress_for_cancel_percent = v))}
        />
      </div>
    </SectionCard>
  );
}
