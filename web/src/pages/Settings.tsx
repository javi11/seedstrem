import { FormEvent, useEffect, useState } from "react";
import { api, Config, ProwlarrIndexer } from "../api";

export function Settings() {
  const [config, setConfig] = useState<Config | null>(null);
  const [saving, setSaving] = useState(false);
  const [message, setMessage] = useState<{ kind: "success" | "error" | "warning"; text: string } | null>(null);
  const [testResult, setTestResult] = useState<string>("");
  const [prowlarrTest, setProwlarrTest] = useState<string>("");
  const [indexers, setIndexers] = useState<ProwlarrIndexer[] | null>(null);
  const [indexersMsg, setIndexersMsg] = useState<string>("");

  useEffect(() => {
    api
      .getConfig()
      .then((c) => {
        // Secrets arrive masked; blank the fields so they follow the
        // "empty = keep existing" convention and never submit the mask.
        c.qbittorrent.password = "";
        c.deluge.password = "";
        c.server.admin_password = "";
        c.prowlarr.api_key = "";
        c.meta.tmdb_api_key = "";
        // A nil slice is serialized as JSON null; configs predating the
        // indexer_ids field arrive without it, so normalize to an array.
        c.prowlarr.indexer_ids = c.prowlarr.indexer_ids ?? [];
        setConfig(c);
      })
      .catch(() => {});
  }, []);

  if (!config) {
    return <span className="loading loading-spinner loading-lg" />;
  }

  const update = (fn: (c: Config) => void) => {
    const next = structuredClone(config);
    fn(next);
    setConfig(next);
  };

  async function save(e: FormEvent) {
    e.preventDefault();
    setSaving(true);
    setMessage(null);
    try {
      const res = await api.putConfig(config!);
      // Re-blank secrets: the response re-masks them, and leaving typed
      // values in state would resubmit them on the next unrelated save.
      res.config.qbittorrent.password = "";
      res.config.deluge.password = "";
      res.config.server.admin_password = "";
      res.config.prowlarr.api_key = "";
      res.config.meta.tmdb_api_key = "";
      res.config.prowlarr.indexer_ids = res.config.prowlarr.indexer_ids ?? [];
      setConfig(res.config);
      setMessage(
        res.restart_required
          ? {
              kind: "warning",
              text: "Saved. Some of the changed settings (listen address, database path, log level, metadata sources) only apply after restarting seedstrem.",
            }
          : { kind: "success", text: "Saved." },
      );
    } catch (err) {
      setMessage({ kind: "error", text: String((err as Error).message) });
    } finally {
      setSaving(false);
    }
  }

  async function testQbittorrent() {
    setTestResult("…");
    try {
      const res = await api.testQbittorrent(
        config!.qbittorrent.url,
        config!.qbittorrent.username,
        config!.qbittorrent.password,
        config!.qbittorrent.category,
      );
      setTestResult(res.ok ? `✓ Connected (qBittorrent ${res.version})` : `✗ ${res.error}`);
    } catch (err) {
      setTestResult(`✗ ${(err as Error).message}`);
    }
  }

  async function testDeluge() {
    setTestResult("…");
    try {
      const res = await api.testDeluge(
        config!.deluge.host,
        config!.deluge.port,
        config!.deluge.username,
        config!.deluge.password,
      );
      setTestResult(res.ok ? `✓ Connected (${res.version})` : `✗ ${res.error}`);
    } catch (err) {
      setTestResult(`✗ ${(err as Error).message}`);
    }
  }

  async function testProwlarr() {
    setProwlarrTest("…");
    try {
      const res = await api.testProwlarr(config!.prowlarr.url, config!.prowlarr.api_key);
      setProwlarrTest(res.ok ? "✓ Connected" : `✗ ${res.error}`);
    } catch (err) {
      setProwlarrTest(`✗ ${(err as Error).message}`);
    }
  }

  async function loadIndexers() {
    setIndexersMsg("…");
    try {
      const res = await api.listProwlarrIndexers(config!.prowlarr.url, config!.prowlarr.api_key);
      if (res.ok && res.indexers) {
        setIndexers(res.indexers);
        setIndexersMsg(res.indexers.length ? "" : "No indexers found");
      } else {
        setIndexersMsg(`✗ ${res.error ?? "failed"}`);
      }
    } catch (err) {
      setIndexersMsg(`✗ ${(err as Error).message}`);
    }
  }

  const toggleIndexer = (id: number, checked: boolean) =>
    update((c) => {
      const set = new Set(c.prowlarr.indexer_ids);
      if (checked) set.add(id);
      else set.delete(id);
      c.prowlarr.indexer_ids = [...set].sort((a, b) => a - b);
    });

  // Category lists are edited as comma-separated strings for convenience.
  const parseCategories = (s: string): number[] =>
    s
      .split(",")
      .map((p) => Number(p.trim()))
      .filter((n) => Number.isFinite(n) && n > 0);

  return (
    <form className="flex flex-col gap-4" onSubmit={save}>
      {message && <div className={`alert alert-${message.kind}`}>{message.text}</div>}

      <div className="card bg-base-100 shadow">
        <div className="card-body">
          <h2 className="card-title">Download client</h2>
          <label className="form-control max-w-xs">
            <span className="label-text">Client</span>
            <select
              className="select select-bordered"
              value={config.downloader.type}
              onChange={(e) => {
                setTestResult("");
                update((c) => (c.downloader.type = e.target.value));
              }}
            >
              <option value="qbittorrent">qBittorrent</option>
              <option value="deluge">Deluge</option>
            </select>
          </label>
        </div>
      </div>

      {config.downloader.type !== "deluge" && (
      <div className="card bg-base-100 shadow">
        <div className="card-body">
          <h2 className="card-title">qBittorrent</h2>
          <p className="text-sm opacity-70">
            Connects to the qBittorrent WebUI API. The WebUI must be enabled and
            reachable with the username/password below.
          </p>
          <label className="form-control">
            <span className="label-text">WebUI URL</span>
            <input
              className="input input-bordered"
              placeholder="http://qbittorrent:8080"
              value={config.qbittorrent.url}
              onChange={(e) => update((c) => (c.qbittorrent.url = e.target.value))}
            />
          </label>
          <div className="grid grid-cols-2 gap-2">
            <label className="form-control">
              <span className="label-text">Username</span>
              <input
                className="input input-bordered"
                value={config.qbittorrent.username}
                onChange={(e) => update((c) => (c.qbittorrent.username = e.target.value))}
              />
            </label>
            <label className="form-control">
              <span className="label-text">Password</span>
              <input
                type="password"
                className="input input-bordered"
                placeholder="unchanged"
                value={config.qbittorrent.password}
                onChange={(e) => update((c) => (c.qbittorrent.password = e.target.value))}
              />
            </label>
          </div>
          <label className="form-control">
            <span className="label-text">Category</span>
            <input
              className="input input-bordered"
              placeholder="seedstrem"
              value={config.qbittorrent.category}
              onChange={(e) => update((c) => (c.qbittorrent.category = e.target.value))}
            />
          </label>
          <div className="card-actions items-center">
            <button type="button" className="btn btn-outline" onClick={testQbittorrent}>
              Test connection
            </button>
            {testResult && <span className="text-sm">{testResult}</span>}
          </div>
        </div>
      </div>
      )}

      {config.downloader.type === "deluge" && (
      <div className="card bg-base-100 shadow">
        <div className="card-body">
          <h2 className="card-title">Deluge</h2>
          <p className="text-sm opacity-70">
            Connects to the Deluge 2 daemon RPC port (not the web UI). Enable
            &quot;Allow Remote Connections&quot; in the daemon settings and use an
            account from Deluge&apos;s auth file. Install the bundled Seedstream
            plugin for fast seeking.
          </p>
          <div className="grid grid-cols-2 gap-2">
            <label className="form-control">
              <span className="label-text">Host</span>
              <input
                className="input input-bordered"
                placeholder="deluge"
                value={config.deluge.host}
                onChange={(e) => update((c) => (c.deluge.host = e.target.value))}
              />
            </label>
            <label className="form-control">
              <span className="label-text">Daemon port</span>
              <input
                type="number"
                className="input input-bordered"
                placeholder="58846"
                value={config.deluge.port}
                onChange={(e) => update((c) => (c.deluge.port = Number(e.target.value)))}
              />
            </label>
          </div>
          <div className="grid grid-cols-2 gap-2">
            <label className="form-control">
              <span className="label-text">Username</span>
              <input
                className="input input-bordered"
                placeholder="localclient"
                value={config.deluge.username}
                onChange={(e) => update((c) => (c.deluge.username = e.target.value))}
              />
            </label>
            <label className="form-control">
              <span className="label-text">Password</span>
              <input
                type="password"
                className="input input-bordered"
                placeholder="unchanged"
                value={config.deluge.password}
                onChange={(e) => update((c) => (c.deluge.password = e.target.value))}
              />
            </label>
          </div>
          <label className="form-control">
            <span className="label-text">Label</span>
            <input
              className="input input-bordered"
              placeholder="seedstrem"
              value={config.deluge.label}
              onChange={(e) => update((c) => (c.deluge.label = e.target.value))}
            />
          </label>
          <div className="card-actions items-center">
            <button type="button" className="btn btn-outline" onClick={testDeluge}>
              Test connection
            </button>
            {testResult && <span className="text-sm">{testResult}</span>}
          </div>
        </div>
      </div>
      )}

      <div className="card bg-base-100 shadow">
        <div className="card-body">
          <h2 className="card-title">Prowlarr</h2>
          <p className="text-sm opacity-70">
            seedstrem searches your Prowlarr indexers for torrents. Configure at least one
            indexer inside Prowlarr itself.
          </p>
          <label className="form-control">
            <span className="label-text">Prowlarr URL</span>
            <input
              className="input input-bordered"
              placeholder="http://prowlarr:9696"
              value={config.prowlarr.url}
              onChange={(e) => update((c) => (c.prowlarr.url = e.target.value))}
            />
          </label>
          <label className="form-control">
            <span className="label-text">API key</span>
            <input
              type="password"
              className="input input-bordered"
              placeholder="unchanged"
              value={config.prowlarr.api_key}
              onChange={(e) => update((c) => (c.prowlarr.api_key = e.target.value))}
            />
          </label>
          <div className="grid grid-cols-3 gap-2">
            <label className="form-control">
              <span className="label-text">Movie categories</span>
              <input
                className="input input-bordered"
                value={config.prowlarr.movie_categories.join(", ")}
                onChange={(e) =>
                  update((c) => (c.prowlarr.movie_categories = parseCategories(e.target.value)))
                }
              />
            </label>
            <label className="form-control">
              <span className="label-text">TV categories</span>
              <input
                className="input input-bordered"
                value={config.prowlarr.tv_categories.join(", ")}
                onChange={(e) =>
                  update((c) => (c.prowlarr.tv_categories = parseCategories(e.target.value)))
                }
              />
            </label>
            <label className="form-control">
              <span className="label-text">Anime categories</span>
              <input
                className="input input-bordered"
                value={config.prowlarr.anime_categories.join(", ")}
                onChange={(e) =>
                  update((c) => (c.prowlarr.anime_categories = parseCategories(e.target.value)))
                }
              />
            </label>
          </div>
          <div className="form-control">
            <span className="label-text">Search indexers</span>
            <p className="text-sm opacity-70">
              Restrict searches to specific indexers. Leave all unchecked to search every
              indexer.
            </p>
            <div className="mt-2 flex flex-wrap items-center gap-2">
              <button type="button" className="btn btn-outline btn-sm" onClick={loadIndexers}>
                Load indexers
              </button>
              {indexersMsg && <span className="text-sm">{indexersMsg}</span>}
            </div>
            {indexers && indexers.length > 0 && (
              <div className="mt-2 grid grid-cols-2 gap-1 sm:grid-cols-3">
                {indexers.map((ix) => (
                  <label key={ix.id} className="label cursor-pointer justify-start gap-2">
                    <input
                      type="checkbox"
                      className="checkbox checkbox-sm"
                      checked={(config.prowlarr.indexer_ids ?? []).includes(ix.id)}
                      onChange={(e) => toggleIndexer(ix.id, e.target.checked)}
                    />
                    <span className="label-text">{ix.name}</span>
                  </label>
                ))}
              </div>
            )}
          </div>
          <div className="card-actions items-center">
            <button type="button" className="btn btn-outline" onClick={testProwlarr}>
              Test connection
            </button>
            {prowlarrTest && <span className="text-sm">{prowlarrTest}</span>}
          </div>
        </div>
      </div>

      <div className="card bg-base-100 shadow">
        <div className="card-body">
          <h2 className="card-title">Content types</h2>
          <p className="text-sm opacity-70">Which content the Stremio addon serves.</p>
          <label className="label cursor-pointer justify-start gap-3">
            <input
              type="checkbox"
              className="toggle"
              checked={config.addon.enable_movies}
              onChange={(e) => update((c) => (c.addon.enable_movies = e.target.checked))}
            />
            <span className="label-text">Movies</span>
          </label>
          <label className="label cursor-pointer justify-start gap-3">
            <input
              type="checkbox"
              className="toggle"
              checked={config.addon.enable_series}
              onChange={(e) => update((c) => (c.addon.enable_series = e.target.checked))}
            />
            <span className="label-text">TV series</span>
          </label>
          <label className="label cursor-pointer justify-start gap-3">
            <input
              type="checkbox"
              className="toggle"
              checked={config.addon.enable_anime}
              onChange={(e) => update((c) => (c.addon.enable_anime = e.target.checked))}
            />
            <span className="label-text">Anime (Kitsu / MAL ids)</span>
          </label>
        </div>
      </div>

      <div className="card bg-base-100 shadow">
        <div className="card-body">
          <h2 className="card-title">Result filters</h2>
          <div className="grid grid-cols-2 gap-2 md:grid-cols-4">
            <label className="form-control">
              <span className="label-text">Min seeders</span>
              <input
                type="number"
                min={0}
                className="input input-bordered"
                value={config.filters.min_seeders}
                onChange={(e) => update((c) => (c.filters.min_seeders = Number(e.target.value)))}
              />
            </label>
            <label className="form-control">
              <span className="label-text">Min size (MB)</span>
              <input
                type="number"
                min={0}
                className="input input-bordered"
                value={config.filters.min_size_mb}
                onChange={(e) => update((c) => (c.filters.min_size_mb = Number(e.target.value)))}
              />
            </label>
            <label className="form-control">
              <span className="label-text">Max size (MB, 0 = ∞)</span>
              <input
                type="number"
                min={0}
                className="input input-bordered"
                value={config.filters.max_size_mb}
                onChange={(e) => update((c) => (c.filters.max_size_mb = Number(e.target.value)))}
              />
            </label>
            <label className="form-control">
              <span className="label-text">Max results</span>
              <input
                type="number"
                min={1}
                className="input input-bordered"
                value={config.filters.max_results}
                onChange={(e) => update((c) => (c.filters.max_results = Number(e.target.value)))}
              />
            </label>
          </div>
        </div>
      </div>

      <div className="card bg-base-100 shadow">
        <div className="card-body">
          <h2 className="card-title">Metadata</h2>
          <p className="text-sm opacity-70">
            Cinemeta resolves IMDb ids to titles/years. A TMDb API key (optional, free at
            themoviedb.org) improves matching on indexers that support TMDb-id search.
            Changing these requires a restart.
          </p>
          <div className="grid grid-cols-2 gap-2">
            <label className="form-control">
              <span className="label-text">Cinemeta URL</span>
              <input
                className="input input-bordered"
                placeholder="https://v3-cinemeta.strem.io"
                value={config.meta.cinemeta_url}
                onChange={(e) => update((c) => (c.meta.cinemeta_url = e.target.value))}
              />
            </label>
            <label className="form-control">
              <span className="label-text">Metadata timeout (seconds)</span>
              <input
                type="number"
                min={1}
                className="input input-bordered"
                value={config.meta.metadata_timeout_seconds}
                onChange={(e) =>
                  update((c) => (c.meta.metadata_timeout_seconds = Number(e.target.value)))
                }
              />
            </label>
          </div>
          <label className="form-control">
            <span className="label-text">TMDb API key</span>
            <input
              type="password"
              className="input input-bordered"
              placeholder="unchanged"
              value={config.meta.tmdb_api_key}
              onChange={(e) => update((c) => (c.meta.tmdb_api_key = e.target.value))}
            />
          </label>
        </div>
      </div>

      <div className="card bg-base-100 shadow">
        <div className="card-body">
          <h2 className="card-title">Seeding & cleanup</h2>
          <label className="label cursor-pointer justify-start gap-3">
            <input
              type="checkbox"
              className="toggle"
              checked={config.seeding.full}
              onChange={(e) => update((c) => (c.seeding.full = e.target.checked))}
            />
            <span className="label-text">
              Download the whole torrent (played file first) so a complete copy seeds — better
              for ratio on private trackers. Off = download only the played file.
            </span>
          </label>
          <div className="grid grid-cols-2 gap-2">
            <label className="form-control">
              <span className="label-text">Seed time before removal (hours, 0 = never)</span>
              <input
                type="number"
                min={0}
                className="input input-bordered"
                value={config.cleanup.seed_time_hours}
                onChange={(e) =>
                  update((c) => (c.cleanup.seed_time_hours = Number(e.target.value)))
                }
              />
            </label>
            <label className="form-control">
              <span className="label-text">Min progress to keep on abandoned playback (%)</span>
              <input
                type="number"
                min={0}
                max={100}
                className="input input-bordered"
                value={config.cleanup.min_progress_for_cancel_percent}
                onChange={(e) =>
                  update(
                    (c) => (c.cleanup.min_progress_for_cancel_percent = Number(e.target.value)),
                  )
                }
              />
            </label>
          </div>
        </div>
      </div>

      <div className="card bg-base-100 shadow">
        <div className="card-body">
          <h2 className="card-title">Path mappings</h2>
          <p className="text-sm opacity-70">
            Translate paths as qBittorrent sees them to paths seedstrem can read (e.g. Docker
            volume mounts: qBittorrent <code>/downloads</code> → seedstrem <code>/data</code>).
          </p>
          {config.paths.mappings.map((m, i) => (
            <div className="flex items-end gap-2" key={i}>
              <label className="form-control flex-1">
                <span className="label-text">Remote path</span>
                <input
                  className="input input-bordered"
                  value={m.remote}
                  onChange={(e) => update((c) => (c.paths.mappings[i].remote = e.target.value))}
                />
              </label>
              <label className="form-control flex-1">
                <span className="label-text">Local path</span>
                <input
                  className="input input-bordered"
                  value={m.local}
                  onChange={(e) => update((c) => (c.paths.mappings[i].local = e.target.value))}
                />
              </label>
              <button
                type="button"
                className="btn btn-outline btn-error"
                onClick={() => update((c) => c.paths.mappings.splice(i, 1))}
              >
                ✕
              </button>
            </div>
          ))}
          <div className="card-actions">
            <button
              type="button"
              className="btn btn-outline btn-sm"
              onClick={() => update((c) => c.paths.mappings.push({ remote: "", local: "" }))}
            >
              + Add mapping
            </button>
          </div>
        </div>
      </div>

      <div className="card bg-base-100 shadow">
        <div className="card-body">
          <h2 className="card-title">Server</h2>
          <div className="grid grid-cols-2 gap-2">
            <label className="form-control">
              <span className="label-text">Listen address</span>
              <input
                className="input input-bordered"
                value={config.server.listen}
                onChange={(e) => update((c) => (c.server.listen = e.target.value))}
              />
            </label>
            <label className="form-control">
              <span className="label-text">External URL (used in stream links)</span>
              <input
                className="input input-bordered"
                value={config.server.external_url}
                onChange={(e) => update((c) => (c.server.external_url = e.target.value))}
              />
            </label>
          </div>
          <label className="form-control">
            <span className="label-text">New admin password (leave blank to keep)</span>
            <input
              type="password"
              className="input input-bordered"
              placeholder="unchanged"
              value={config.server.admin_password}
              onChange={(e) =>
                update((c) => (c.server.admin_password = e.target.value))
              }
            />
          </label>
          <div className="grid grid-cols-2 gap-2">
            <label className="form-control">
              <span className="label-text">Database path (restart required)</span>
              <input
                className="input input-bordered"
                placeholder="/config/seedstrem.db"
                value={config.storage.database}
                onChange={(e) => update((c) => (c.storage.database = e.target.value))}
              />
            </label>
            <label className="form-control">
              <span className="label-text">Log level (restart required)</span>
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
          <label className="label cursor-pointer justify-start gap-3">
            <input
              type="checkbox"
              className="toggle"
              checked={config.storage.delete_files_on_remove}
              onChange={(e) =>
                update((c) => (c.storage.delete_files_on_remove = e.target.checked))
              }
            />
            <span className="label-text">Delete downloaded files when a torrent is removed</span>
          </label>
          <label className="form-control">
            <span className="label-text">Max disk usage before withholding new streams (%)</span>
            <input
              type="number"
              className="input input-bordered"
              min={0}
              max={100}
              value={config.storage.max_disk_usage_percent}
              onChange={(e) =>
                update((c) => (c.storage.max_disk_usage_percent = Number(e.target.value)))
              }
            />
            <span className="label-text-alt text-base-content/60">
              Once the download disk is this full, no new streams are offered and releases that
              would push it over are hidden. Already-downloaded and downloading content is
              unaffected. 0 disables.
            </span>
          </label>
        </div>
      </div>

      <div className="card bg-base-100 shadow">
        <div className="card-body">
          <h2 className="card-title">Streaming</h2>
          <div className="grid grid-cols-2 gap-2">
            <label className="form-control">
              <span className="label-text">
                Wait timeout for missing pieces (seconds)
              </span>
              <input
                type="number"
                min={5}
                className="input input-bordered"
                value={config.stream.wait_timeout_seconds}
                onChange={(e) =>
                  update((c) => (c.stream.wait_timeout_seconds = Number(e.target.value)))
                }
              />
            </label>
            <label className="form-control">
              <span className="label-text">Read chunk size (MiB)</span>
              <input
                type="number"
                min={1}
                className="input input-bordered"
                value={Math.max(1, Math.round(config.stream.read_chunk / 1048576))}
                onChange={(e) =>
                  update(
                    (c) =>
                      (c.stream.read_chunk = Math.max(1, Number(e.target.value)) * 1048576),
                  )
                }
              />
            </label>
          </div>
        </div>
      </div>

      <button className="btn btn-primary" disabled={saving}>
        {saving ? <span className="loading loading-spinner loading-sm" /> : "Save settings"}
      </button>
    </form>
  );
}
