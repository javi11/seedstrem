import { SectionProps } from "../types";
import { SectionCard, NumberField, TextField, ToggleField } from "../fields";

// Category lists are edited as comma-separated ints; keyword lists as
// comma-separated strings. Both round-trip through the array config fields.
function parseCategories(s: string): number[] {
  return s
    .split(",")
    .map((p) => Number(p.trim()))
    .filter((n) => Number.isFinite(n) && n > 0);
}

function parseKeywords(s: string): string[] {
  return s
    .split(",")
    .map((p) => p.trim())
    .filter((p) => p.length > 0);
}

export function Rss({ config, update }: SectionProps) {
  return (
    <SectionCard
      title="RSS auto-grab"
      description="Periodically pull the just-released items from your Prowlarr indexers and auto-download a filtered subset — to build seeding ratio and pre-cache content so it plays instantly in Stremio. Off by default. The seeder floor is inherited from Result filters; size, categories, and title keywords below are RSS-specific. Disk use is bounded by the disk-usage gate and the seed-time cleanup."
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
          hint="Caps how many new releases are added each poll. Spread across your indexers (round-robin) so no single tracker dominates."
        />
        <NumberField
          label="Max concurrent downloads (0 = no limit)"
          min={0}
          value={config.rss.max_concurrent_downloads}
          onChange={(v) => update((c) => (c.rss.max_concurrent_downloads = v))}
          hint="Skip (or trim) a grab cycle when this many torrents are already downloading."
        />
        <NumberField
          label="Max active torrents (0 = no limit)"
          min={0}
          value={config.rss.max_active_torrents}
          onChange={(v) => update((c) => (c.rss.max_active_torrents = v))}
          hint="Stop grabbing once the total number of managed torrents reaches this."
        />
      </div>

      <div className="divider text-sm opacity-70">RSS filters</div>

      <div className="grid gap-4 sm:grid-cols-2">
        <NumberField
          label="Min size (MB, 0 = no minimum)"
          min={0}
          value={config.rss.filters.min_size_mb}
          onChange={(v) => update((c) => (c.rss.filters.min_size_mb = v))}
          hint="Skip releases smaller than this."
        />
        <NumberField
          label="Max size (MB, 0 = unbounded)"
          min={0}
          value={config.rss.filters.max_size_mb}
          onChange={(v) => update((c) => (c.rss.filters.max_size_mb = v))}
          hint="Skip releases larger than this."
        />
      </div>
      <TextField
        label="Categories (newznab ids, comma-separated)"
        placeholder="leave empty to poll all enabled content types"
        value={config.rss.filters.categories.join(", ")}
        onChange={(v) => update((c) => (c.rss.filters.categories = parseCategories(v)))}
      />
      <TextField
        label="Include keywords (comma-separated)"
        placeholder="e.g. 1080p, 2160p — empty allows all titles"
        value={config.rss.filters.include_keywords.join(", ")}
        onChange={(v) => update((c) => (c.rss.filters.include_keywords = parseKeywords(v)))}
      />
      <TextField
        label="Exclude keywords (comma-separated)"
        placeholder="e.g. CAM, HDTS — dropped even if included; case-insensitive"
        value={config.rss.filters.exclude_keywords.join(", ")}
        onChange={(v) => update((c) => (c.rss.filters.exclude_keywords = parseKeywords(v)))}
      />
    </SectionCard>
  );
}
