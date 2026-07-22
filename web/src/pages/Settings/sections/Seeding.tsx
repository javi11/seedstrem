import { SectionProps } from "../types";
import { SectionCard, NumberField, SelectField, ToggleField } from "../fields";

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
          hint="A completed torrent is removed once it has seeded this long."
        />
        <NumberField
          label="Target seeding ratio (0 = disabled)"
          min={0}
          step={0.1}
          value={config.cleanup.target_ratio}
          onChange={(v) => update((c) => (c.cleanup.target_ratio = v))}
          hint="A completed torrent also becomes removal-eligible once it reaches this upload/download ratio — whichever of ratio or seed time comes first."
        />
      </div>
      <div className="grid gap-4 sm:grid-cols-2">
        <SelectField
          label="Delete order when cleaning up"
          value={config.cleanup.delete_policy || "oldest_first"}
          options={[
            { value: "oldest_first", label: "Oldest first (by add time)" },
            { value: "lowest_upload", label: "Lowest upload activity" },
          ]}
          onChange={(v) => update((c) => (c.cleanup.delete_policy = v))}
          hint="Order in which removal-eligible torrents are cleaned up."
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
