import { api } from "../api";
import { formatBytes } from "../lib/format";
import { DASHBOARD_STATUSES } from "../lib/status";
import { usePolling } from "../lib/usePolling";
import { useToast } from "../components/Toast";
import { StatCard } from "../components/StatCard";
import { StatCardSkeleton } from "../components/Skeleton";
import { FreshnessIndicator } from "../components/FreshnessIndicator";
import { PageHeader } from "../components/PageHeader";

export function Dashboard() {
  const toast = useToast();
  const { data: status, error, isStale, isOffline, lastUpdated } = usePolling(
    api.status,
    { baseIntervalMs: 5000 },
  );

  // First load, nothing yet.
  if (!status) {
    if (error) {
      return (
        <div className="alert alert-error">
          <span>Couldn&rsquo;t load status. Retrying automatically…</span>
        </div>
      );
    }
    return (
      <div className="flex flex-col gap-6">
        <PageHeader title="Dashboard" />
        <div className="grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-4">
          {Array.from({ length: 8 }).map((_, i) => (
            <StatCardSkeleton key={i} />
          ))}
        </div>
      </div>
    );
  }

  async function copyManifest() {
    try {
      await navigator.clipboard.writeText(status!.manifest_url);
      toast.info("Manifest URL copied");
    } catch {
      toast.error("Couldn't copy to clipboard");
    }
  }

  const externalHostMismatch =
    new URL(status.external_url).host !== window.location.host;
  // stremio:// deep link installs the addon directly in the Stremio app.
  const stremioDeepLink = status.manifest_url.replace(/^https?:\/\//, "stremio://");
  // Older backends only report the qbittorrent key.
  const downloaderStatus = status.downloader ?? status.qbittorrent;
  const downloaderName = status.downloader?.type === "deluge" ? "Deluge" : "qBittorrent";

  return (
    <div className="flex flex-col gap-6">
      <PageHeader
        title="Dashboard"
        actions={
          <FreshnessIndicator
            isStale={isStale}
            isOffline={isOffline}
            lastUpdated={lastUpdated}
          />
        }
      />

      <div className="grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-4">
        <StatCard
          label={downloaderName}
          value={
            <span className={downloaderStatus.connected ? "text-success" : "text-error"}>
              {downloaderStatus.connected ? "Connected" : "Disconnected"}
            </span>
          }
          hint={
            downloaderStatus.connected
              ? `v${downloaderStatus.version}`
              : downloaderStatus.error
          }
          accent="default"
        />
        {DASHBOARD_STATUSES.map(({ key, label }) => (
          <StatCard
            key={key}
            label={label}
            value={status.torrents[key] ?? 0}
            accent={key === "downloading" ? "primary" : key === "error" ? "error" : "default"}
          />
        ))}
        <StatCard
          label="Uploaded"
          value={formatBytes(status.total_uploaded ?? 0)}
          hint="total seeded"
          accent="success"
        />
      </div>

      {externalHostMismatch && (
        <div className="alert alert-warning">
          <span>
            The configured external URL <code>{status.external_url}</code> doesn&rsquo;t
            match the address you&rsquo;re browsing from. Generated stream links may not be
            reachable by players — check Settings.
          </span>
        </div>
      )}

      <div className="surface p-6">
        <h2 className="text-lg font-bold tracking-brand">Stremio addon</h2>
        <p className="mt-1 text-sm opacity-70">
          Install this addon in Stremio, then search for movies or shows — seedstrem finds
          torrents via Prowlarr and streams them through your download client.
        </p>
        <div className="mt-4 flex flex-col gap-2 sm:flex-row sm:items-center">
          <input
            readOnly
            className="input input-bordered flex-1 font-mono text-sm"
            value={status.manifest_url}
            onFocus={(e) => e.currentTarget.select()}
          />
          <div className="flex gap-2">
            <button className="btn flex-1 sm:flex-none" onClick={copyManifest}>
              Copy
            </button>
            <a className="btn btn-primary flex-1 sm:flex-none" href={stremioDeepLink}>
              Install in Stremio
            </a>
          </div>
        </div>
        <p className="mt-3 text-xs opacity-50">seedstrem {status.version}</p>
      </div>
    </div>
  );
}
