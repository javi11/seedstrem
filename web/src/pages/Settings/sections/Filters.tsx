import { SectionProps } from "../types";
import { SectionCard, NumberField } from "../fields";

export function Filters({ config, update }: SectionProps) {
  return (
    <SectionCard title="Result filters" description="Trim which torrent results seedstrem offers.">
      <div className="grid grid-cols-2 gap-4 md:grid-cols-4">
        <NumberField
          label="Min seeders"
          min={0}
          value={config.filters.min_seeders}
          onChange={(v) => update((c) => (c.filters.min_seeders = v))}
        />
        <NumberField
          label="Min size (MB)"
          min={0}
          value={config.filters.min_size_mb}
          onChange={(v) => update((c) => (c.filters.min_size_mb = v))}
        />
        <NumberField
          label="Max size (MB, 0 = ∞)"
          min={0}
          value={config.filters.max_size_mb}
          onChange={(v) => update((c) => (c.filters.max_size_mb = v))}
        />
        <NumberField
          label="Max results"
          min={1}
          value={config.filters.max_results}
          onChange={(v) => update((c) => (c.filters.max_results = v))}
        />
      </div>
    </SectionCard>
  );
}
