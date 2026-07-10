import { useCallback, useEffect, useState } from "react";
import { api, Status } from "../api";

const STATUS_LABELS: Record<string, string> = {
  magnet_conversion: "Resolving",
  waiting_files_selection: "Waiting selection",
  queued: "Queued",
  downloading: "Downloading",
  downloaded: "Downloaded",
  error: "Error",
};

export function Dashboard() {
  const [status, setStatus] = useState<Status | null>(null);
  const [loadError, setLoadError] = useState(false);
  const [copied, setCopied] = useState(false);

  const refresh = useCallback(async () => {
    try {
      setStatus(await api.status());
      setLoadError(false);
    } catch {
      setLoadError(true);
    }
  }, []);

  useEffect(() => {
    let inflight = false;
    const tick = async () => {
      if (inflight) return;
      inflight = true;
      await refresh();
      inflight = false;
    };
    tick();
    const t = setInterval(tick, 5000);
    return () => clearInterval(t);
  }, [refresh]);

  if (!status) {
    if (loadError) {
      return (
        <div className="alert alert-error">
          <span>Could not load status. Retrying…</span>
        </div>
      );
    }
    return <span className="loading loading-spinner loading-lg" />;
  }

  async function copyManifest() {
    await navigator.clipboard.writeText(status!.manifest_url);
    setCopied(true);
    setTimeout(() => setCopied(false), 1500);
  }

  const externalHostMismatch =
    new URL(status.external_url).host !== window.location.host;

  // stremio:// deep link installs the addon directly in the Stremio app.
  const stremioDeepLink = status.manifest_url.replace(/^https?:\/\//, "stremio://");

  return (
    <div className="flex flex-col gap-4">
      <div className="stats stats-vertical shadow sm:stats-horizontal">
        <div className="stat">
          <div className="stat-title">qBittorrent</div>
          <div className={`stat-value text-lg ${status.qbittorrent.connected ? "text-success" : "text-error"}`}>
            {status.qbittorrent.connected ? `Connected (${status.qbittorrent.version})` : "Disconnected"}
          </div>
          {status.qbittorrent.error && (
            <div className="stat-desc text-error">{status.qbittorrent.error}</div>
          )}
        </div>
        {Object.entries(STATUS_LABELS).map(([key, label]) => (
          <div className="stat" key={key}>
            <div className="stat-title">{label}</div>
            <div className="stat-value text-lg">{status.torrents[key] ?? 0}</div>
          </div>
        ))}
      </div>

      {externalHostMismatch && (
        <div className="alert alert-warning">
          <span>
            The configured external URL <code>{status.external_url}</code> does not match the
            address you are browsing from. Generated stream links may not be reachable by
            players — check Settings.
          </span>
        </div>
      )}

      <div className="card bg-base-100 shadow">
        <div className="card-body">
          <h2 className="card-title">Stremio addon</h2>
          <p className="text-sm opacity-70">
            Install this addon in Stremio, then search for movies or shows — seedstrem finds
            torrents via Prowlarr and streams them through qBittorrent. Manifest URL:
          </p>
          <div className="flex items-center gap-2">
            <input
              readOnly
              className="input input-bordered flex-1 font-mono text-sm"
              value={status.manifest_url}
            />
            <button className="btn" onClick={copyManifest}>
              {copied ? "Copied ✓" : "Copy"}
            </button>
            <a className="btn btn-primary" href={stremioDeepLink}>
              Install in Stremio
            </a>
          </div>
          <p className="text-xs opacity-50">seedstrem {status.version}</p>
        </div>
      </div>
    </div>
  );
}
