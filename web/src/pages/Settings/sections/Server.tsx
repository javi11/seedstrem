import { SectionProps } from "../types";
import { SectionCard, TextField, NumberField, ToggleField } from "../fields";

export function Server({ config, update }: SectionProps) {
  return (
    <SectionCard title="Server">
      <div className="grid gap-4 sm:grid-cols-2">
        <TextField
          label="Listen address"
          value={config.server.listen}
          onChange={(v) => update((c) => (c.server.listen = v))}
        />
        <TextField
          label="External URL (used in stream links)"
          value={config.server.external_url}
          onChange={(v) => update((c) => (c.server.external_url = v))}
        />
      </div>
      <TextField
        label="New admin password (leave blank to keep)"
        type="password"
        placeholder="unchanged"
        value={config.server.admin_password}
        onChange={(v) => update((c) => (c.server.admin_password = v))}
      />
      <div className="grid gap-4 sm:grid-cols-2">
        <TextField
          label="Database path (restart required)"
          placeholder="/config/seedstrem.db"
          value={config.storage.database}
          onChange={(v) => update((c) => (c.storage.database = v))}
        />
        <label className="form-control">
          <span className="label-text mb-1">Log level (restart required)</span>
          <select
            className="select select-bordered"
            value={config.log.level}
            onChange={(e) => update((c) => (c.log.level = e.target.value))}
          >
            <option value="debug">debug</option>
            <option value="info">info</option>
            <option value="warn">warn</option>
            <option value="error">error</option>
          </select>
        </label>
      </div>
      <ToggleField
        label="Delete downloaded files when a torrent is removed"
        checked={config.storage.delete_files_on_remove}
        onChange={(v) => update((c) => (c.storage.delete_files_on_remove = v))}
      />
      <NumberField
        label="Max disk usage before withholding new streams (%)"
        min={0}
        max={100}
        value={config.storage.max_disk_usage_percent}
        onChange={(v) => update((c) => (c.storage.max_disk_usage_percent = v))}
        hint="Once the download disk is this full, no new streams are offered and releases that would push it over are hidden. Already-downloaded and downloading content is unaffected. 0 disables."
      />
    </SectionCard>
  );
}
