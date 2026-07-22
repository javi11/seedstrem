import { SectionProps } from "../types";
import { SectionCard, NumberField, ToggleField } from "../fields";

export function Storage({ config, update }: SectionProps) {
  return (
    <SectionCard
      title="Storage & disk limits"
      description="Bound how much download storage seedstrem uses. On-demand streams are never blocked by these limits — they gate the background RSS grabber and hide releases that would push usage over."
    >
      <NumberField
        label="Max download storage (GB, 0 = no cap)"
        min={0}
        value={config.storage.max_download_storage_gb}
        onChange={(v) => update((c) => (c.storage.max_download_storage_gb = v))}
        hint="Absolute cap on used download storage. The RSS grabber stops adding torrents once usage reaches it (e.g. 3072 for a 3 TB cap). Complements the percentage gate below — whichever is more restrictive wins."
      />
      <NumberField
        label="Max disk usage before withholding new streams (%)"
        min={0}
        max={100}
        value={config.storage.max_disk_usage_percent}
        onChange={(v) => update((c) => (c.storage.max_disk_usage_percent = v))}
        hint="Once the download disk is this full, no new streams are offered and releases that would push it over are hidden. Already-downloaded and downloading content is unaffected. 0 disables."
      />
      <ToggleField
        label="Delete downloaded files when a torrent is removed"
        checked={config.storage.delete_files_on_remove}
        onChange={(v) => update((c) => (c.storage.delete_files_on_remove = v))}
      />
    </SectionCard>
  );
}
