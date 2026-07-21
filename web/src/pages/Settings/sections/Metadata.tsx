import { SectionProps } from "../types";
import { SectionCard, TextField, NumberField } from "../fields";

export function Metadata({ config, update }: SectionProps) {
  return (
    <SectionCard
      title="Metadata"
      description="Cinemeta resolves IMDb ids to titles/years. A TMDb API key (optional, free at themoviedb.org) improves matching on indexers that support TMDb-id search. Changing these requires a restart."
    >
      <div className="grid gap-4 sm:grid-cols-2">
        <TextField
          label="Cinemeta URL"
          placeholder="https://v3-cinemeta.strem.io"
          value={config.meta.cinemeta_url}
          onChange={(v) => update((c) => (c.meta.cinemeta_url = v))}
        />
        <NumberField
          label="Metadata timeout (seconds)"
          min={1}
          value={config.meta.metadata_timeout_seconds}
          onChange={(v) => update((c) => (c.meta.metadata_timeout_seconds = v))}
        />
      </div>
      <TextField
        label="TMDb API key"
        type="password"
        placeholder="unchanged"
        value={config.meta.tmdb_api_key}
        onChange={(v) => update((c) => (c.meta.tmdb_api_key = v))}
      />
    </SectionCard>
  );
}
