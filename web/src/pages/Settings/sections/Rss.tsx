import { SectionProps } from "../types";
import { SectionCard, NumberField, ToggleField } from "../fields";

export function Rss({ config, update }: SectionProps) {
  return (
    <SectionCard
      title="RSS auto-grab"
      description="Periodically pull the just-released items from your Prowlarr indexers and auto-download a filtered subset — to build seeding ratio and pre-cache content so it plays instantly in Stremio. Off by default. Which indexers/categories are polled comes from the Prowlarr section, and the seeder/size limits from Result filters. Disk use is bounded by the disk-usage gate and the seed-time cleanup."
    >
      <ToggleField
        label="Enable background RSS grabbing"
        checked={config.rss.enabled}
        onChange={(v) => update((c) => (c.rss.enabled = v))}
      />
      <ToggleField
        label="Only grab freeleech releases (their download doesn't count against ratio)"
        checked={config.rss.freeleech_only}
        onChange={(v) => update((c) => (c.rss.freeleech_only = v))}
      />
      <div className="grid gap-4 sm:grid-cols-2">
        <NumberField
          label="Poll interval (minutes)"
          min={1}
          value={config.rss.interval_minutes}
          onChange={(v) => update((c) => (c.rss.interval_minutes = v))}
          hint="How often to check Prowlarr for new releases. Applied on restart."
        />
        <NumberField
          label="Max grabs per cycle (0 disables)"
          min={0}
          value={config.rss.max_grabs_per_cycle}
          onChange={(v) => update((c) => (c.rss.max_grabs_per_cycle = v))}
          hint="Caps how many new releases are added each poll."
        />
      </div>
    </SectionCard>
  );
}
