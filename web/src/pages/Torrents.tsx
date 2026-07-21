import { Fragment, useState } from "react";
import { api, Torrent } from "../api";
import { formatBytes, formatSpeed, availableUntil } from "../lib/format";
import { usePolling } from "../lib/usePolling";
import { useToast } from "../components/Toast";
import { StatusBadge } from "../components/StatusBadge";
import { ProgressCell } from "../components/ProgressCell";
import { FreshnessIndicator } from "../components/FreshnessIndicator";
import { PageHeader } from "../components/PageHeader";
import { ConfirmDialog } from "../components/ConfirmDialog";
import { Skeleton } from "../components/Skeleton";

export function Torrents() {
  const toast = useToast();
  const { data, isStale, isOffline, lastUpdated } = usePolling(api.torrents, {
    baseIntervalMs: 3000,
  });

  const [expanded, setExpanded] = useState<string | null>(null);
  const [copiedUrl, setCopiedUrl] = useState("");
  const [removed, setRemoved] = useState<Set<string>>(new Set());
  const [deleteTarget, setDeleteTarget] = useState<Torrent | null>(null);
  const [deleting, setDeleting] = useState(false);

  async function copy(url: string) {
    try {
      await navigator.clipboard.writeText(url);
      setCopiedUrl(url);
      setTimeout(() => setCopiedUrl(""), 1500);
      toast.info("Stream URL copied");
    } catch {
      toast.error("Couldn't copy to clipboard");
    }
  }

  async function confirmDelete() {
    if (!deleteTarget) return;
    const id = deleteTarget.id;
    setDeleting(true);
    // Optimistic hide; restore on failure.
    setRemoved((s) => new Set(s).add(id));
    try {
      await api.deleteTorrent(id);
      toast.success("Torrent removed");
      setDeleteTarget(null);
    } catch (err) {
      setRemoved((s) => {
        const next = new Set(s);
        next.delete(id);
        return next;
      });
      toast.error(`Couldn't remove torrent — ${(err as Error).message}`);
    } finally {
      setDeleting(false);
    }
  }

  // First load.
  if (!data) {
    return (
      <div className="flex flex-col gap-6">
        <PageHeader title="Torrents" />
        <div className="surface p-4">
          {Array.from({ length: 5 }).map((_, i) => (
            <Skeleton key={i} className="mb-3 h-10 w-full" />
          ))}
        </div>
      </div>
    );
  }

  const torrents = data.filter((t) => !removed.has(t.id));
  const active = torrents.filter((t) => t.status === "downloading").length;

  return (
    <div className="flex flex-col gap-6">
      <PageHeader
        title="Torrents"
        subtitle={
          torrents.length > 0
            ? `${torrents.length} item${torrents.length === 1 ? "" : "s"} · ${active} active`
            : undefined
        }
        actions={
          <FreshnessIndicator
            isStale={isStale}
            isOffline={isOffline}
            lastUpdated={lastUpdated}
          />
        }
      />

      {(isStale || isOffline) && (
        <div className="alert alert-warning py-2">
          <span className="loading loading-spinner loading-xs" />
          <span className="text-sm">
            Can&rsquo;t reach the server — showing the last known data. Retrying…
          </span>
        </div>
      )}

      {torrents.length === 0 ? (
        <div className="surface p-10 text-center">
          <div className="text-4xl">🌱</div>
          <h2 className="mt-3 text-lg font-bold tracking-brand">No torrents yet</h2>
          <p className="mx-auto mt-1 max-w-md text-sm opacity-70">
            When you play something through the seedstrem Stremio addon, it appears here —
            found via Prowlarr and downloaded through your torrent client.
          </p>
        </div>
      ) : (
        <>
          {/* Desktop table */}
          <div className="surface hidden overflow-x-auto md:block">
            <table className="table">
              <thead>
                <tr>
                  <th>Name</th>
                  <th>Status</th>
                  <th className="w-40">Progress</th>
                  <th>Speed</th>
                  <th>Seeds</th>
                  <th>Size</th>
                  <th>Ratio</th>
                  <th>Available until</th>
                  <th></th>
                </tr>
              </thead>
              <tbody>
                {torrents.map((t) => (
                  <Fragment key={t.id}>
                    <tr
                      className="cursor-pointer hover:bg-base-300/50"
                      onClick={() => setExpanded(expanded === t.id ? null : t.id)}
                    >
                      <td className="max-w-xs truncate font-medium">{t.name || t.hash}</td>
                      <td>
                        <StatusBadge status={t.status} />
                      </td>
                      <td>
                        <ProgressCell progress={t.progress} />
                      </td>
                      <td className="tabular-nums">
                        {t.status === "downloading" ? formatSpeed(t.speed) : "—"}
                      </td>
                      <td className="tabular-nums">
                        {t.status === "downloading" ? t.seeders : "—"}
                      </td>
                      <td className="tabular-nums">{formatBytes(t.size)}</td>
                      <td className="tabular-nums">
                        <span className={t.ratio >= 1 ? "text-success" : ""}>
                          {t.ratio.toFixed(2)}
                        </span>
                      </td>
                      <td className="whitespace-nowrap">{availableUntil(t)}</td>
                      <td>
                        <button
                          className="btn btn-ghost btn-xs text-error"
                          aria-label="Delete torrent"
                          onClick={(e) => {
                            e.stopPropagation();
                            setDeleteTarget(t);
                          }}
                        >
                          🗑
                        </button>
                      </td>
                    </tr>
                    {expanded === t.id && (
                      <tr>
                        <td colSpan={9} className="bg-base-200">
                          <TorrentDetail t={t} copiedUrl={copiedUrl} onCopy={copy} />
                        </td>
                      </tr>
                    )}
                  </Fragment>
                ))}
              </tbody>
            </table>
          </div>

          {/* Mobile cards */}
          <div className="flex flex-col gap-3 md:hidden">
            {torrents.map((t) => (
              <div key={t.id} className="surface p-4">
                <div className="flex items-start justify-between gap-2">
                  <span className="min-w-0 flex-1 truncate font-medium">
                    {t.name || t.hash}
                  </span>
                  <button
                    className="btn btn-ghost btn-xs text-error"
                    aria-label="Delete torrent"
                    onClick={() => setDeleteTarget(t)}
                  >
                    🗑
                  </button>
                </div>
                <div className="mt-2 flex items-center gap-2">
                  <StatusBadge status={t.status} />
                  <span className="text-xs opacity-60 tabular-nums">
                    {formatBytes(t.size)}
                  </span>
                </div>
                <div className="mt-3">
                  <ProgressCell progress={t.progress} />
                </div>
                <div className="mt-3 grid grid-cols-2 gap-y-1 text-xs opacity-70">
                  <span>Speed: {t.status === "downloading" ? formatSpeed(t.speed) : "—"}</span>
                  <span>Seeds: {t.status === "downloading" ? t.seeders : "—"}</span>
                  <span>Ratio: {t.ratio.toFixed(2)}</span>
                  <span>Until: {availableUntil(t)}</span>
                </div>
                <button
                  className="btn btn-ghost btn-xs mt-2"
                  onClick={() => setExpanded(expanded === t.id ? null : t.id)}
                >
                  {expanded === t.id ? "Hide files" : "Show files"}
                </button>
                {expanded === t.id && (
                  <div className="mt-2 rounded-box bg-base-200 p-3">
                    <TorrentDetail t={t} copiedUrl={copiedUrl} onCopy={copy} />
                  </div>
                )}
              </div>
            ))}
          </div>
        </>
      )}

      <ConfirmDialog
        open={deleteTarget !== null}
        title="Remove torrent?"
        confirmLabel="Remove"
        danger
        busy={deleting}
        onCancel={() => !deleting && setDeleteTarget(null)}
        onConfirm={confirmDelete}
      >
        <span className="break-words">
          Remove <strong>{deleteTarget?.name || deleteTarget?.hash}</strong> from seedstrem?
          Downloaded files are deleted only if that option is enabled in Settings.
        </span>
      </ConfirmDialog>
    </div>
  );
}

interface DetailProps {
  t: Torrent;
  copiedUrl: string;
  onCopy: (url: string) => void;
}

function TorrentDetail({ t, copiedUrl, onCopy }: DetailProps) {
  return (
    <div className="flex flex-col gap-2">
      {t.error && (
        <div className="alert alert-error py-2 text-sm">
          <span className="break-words">{t.error}</span>
        </div>
      )}
      {t.links.length === 0 ? (
        <span className="text-sm opacity-70">No files selected yet.</span>
      ) : (
        <ul className="flex flex-col gap-1">
          {t.links.map((l) => (
            <li key={l.url} className="flex items-center gap-2 text-sm">
              <span className="min-w-0 flex-1 truncate font-mono text-xs">{l.path}</span>
              <span className="whitespace-nowrap text-xs opacity-60">
                {formatBytes(l.bytes)}
              </span>
              <button
                className="btn btn-outline btn-xs"
                onClick={(e) => {
                  e.stopPropagation();
                  onCopy(l.url);
                }}
              >
                {copiedUrl === l.url ? "Copied ✓" : "Copy stream URL"}
              </button>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}
