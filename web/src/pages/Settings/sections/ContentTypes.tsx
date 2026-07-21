import { SectionProps } from "../types";
import { SectionCard, ToggleField } from "../fields";

export function ContentTypes({ config, update }: SectionProps) {
  return (
    <SectionCard title="Content types" description="Which content the Stremio addon serves.">
      <ToggleField
        label="Movies"
        checked={config.addon.enable_movies}
        onChange={(v) => update((c) => (c.addon.enable_movies = v))}
      />
      <ToggleField
        label="TV series"
        checked={config.addon.enable_series}
        onChange={(v) => update((c) => (c.addon.enable_series = v))}
      />
      <ToggleField
        label="Anime (Kitsu / MAL ids)"
        checked={config.addon.enable_anime}
        onChange={(v) => update((c) => (c.addon.enable_anime = v))}
      />
    </SectionCard>
  );
}
