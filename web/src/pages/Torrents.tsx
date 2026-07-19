import { Fragment, useEffect, useState } from "react";
import { api, formatBytes, Torrent } from "../api";

const BADGE: Record<string, string> = {
  magnet_conversion: "badge-info",
  waiting_files_selection: "badge-warning",
  queued: "badge-neutral",
  downloading: "badge-primary",
  downloaded: "badge-success",
  error: "badge-error",
};

export function availableUntil(t: Torrent): string {
  if (t.progress < 1) return "—";
  if (t.seed_time <= 0) return "Kept";
  const remaining = t.seed_time - t.seeding_time; // seconds
  if (remaining <= 0) return "Removing soon";
  return new Date(Date.now() + remaining * 1000).toLocaleString(undefined, {
    month: "short",
    day: "numeric",
    hour: "2-digit",
    minute: "2-digit",
  });
}

export function Torrents() {
  const [torrents, setTorrents] = useState<Torrent[] | null>(null);
  const [expanded, setExpanded] = useState<string | null>(null);
  const [copiedUrl, setCopiedUrl] = useState("");

  useEffect(() => {
    let inflight = false;
    let stopped = false;
    const refresh = async () => {
      if (inflight) return; // don't overlap a slow request
      inflight = true;
      try {
        const data = await api.torrents();
        if (!stopped) setTorrents(data);
      } catch {
        /* transient; keep last data, next tick retries */
      } finally {
        inflight = false;
      }
    };
    refresh();
    const t = setInterval(refresh, 3000);
    return () => {
      stopped = true;
      clearInterval(t);
    };
  }, []);

  if (!torrents) {
    return <span className="loading loading-spinner loading-lg" />;
  }
  if (torrents.length === 0) {
    return (
      <div className="card bg-base-100 shadow">
        <div className="card-body items-center text-center">
          <h2 className="card-title">No torrents yet</h2>
          <p className="opacity-70">
            Torrents added through the RealDebrid API will appear here.
          </p>
        </div>
      </div>
    );
  }

  async function copy(url: string) {
    await navigator.clipboard.writeText(url);
    setCopiedUrl(url);
    setTimeout(() => setCopiedUrl(""), 1500);
  }

  return (
    <div className="card overflow-x-auto bg-base-100 shadow">
      <table className="table">
        <thead>
          <tr>
            <th>Name</th>
            <th>Status</th>
            <th>Progress</th>
            <th>Speed</th>
            <th>Seeds</th>
            <th>Size</th>
            <th>Uploaded</th>
            <th>Ratio</th>
            <th>Available until</th>
          </tr>
        </thead>
        <tbody>
          {torrents.map((t) => (
            <Fragment key={t.id}>
              <tr
                className="cursor-pointer hover:bg-base-200"
                onClick={() => setExpanded(expanded === t.id ? null : t.id)}
              >
                <td className="max-w-md truncate font-medium">{t.name || t.hash}</td>
                <td>
                  <span className={`badge ${BADGE[t.status] ?? "badge-ghost"}`}>{t.status}</span>
                </td>
                <td className="w-40">
                  <progress
                    className="progress progress-primary w-32"
                    value={t.progress * 100}
                    max={100}
                  />
                </td>
                <td>{t.status === "downloading" ? `${formatBytes(t.speed)}/s` : "—"}</td>
                <td>{t.status === "downloading" ? t.seeders : "—"}</td>
                <td>{formatBytes(t.size)}</td>
                <td>{formatBytes(t.uploaded)}</td>
                <td>
                  <span className={t.ratio >= 1 ? "text-success" : ""}>{t.ratio.toFixed(2)}</span>
                </td>
                <td className="whitespace-nowrap">{availableUntil(t)}</td>
              </tr>
              {expanded === t.id && (
                <tr>
                  <td colSpan={9} className="bg-base-200">
                    {t.error && <div className="alert alert-error mb-2 py-2">{t.error}</div>}
                    {t.links.length === 0 ? (
                      <span className="text-sm opacity-70">
                        No files selected yet (waiting for selectFiles).
                      </span>
                    ) : (
                      <ul className="flex flex-col gap-1">
                        {t.links.map((l) => (
                          <li key={l.url} className="flex items-center gap-2 text-sm">
                            <span className="flex-1 truncate font-mono">{l.path}</span>
                            <span className="opacity-60">{formatBytes(l.bytes)}</span>
                            <button
                              className="btn btn-outline btn-xs"
                              onClick={(e) => {
                                e.stopPropagation();
                                copy(l.url);
                              }}
                            >
                              {copiedUrl === l.url ? "Copied ✓" : "Copy stream URL"}
                            </button>
                          </li>
                        ))}
                      </ul>
                    )}
                  </td>
                </tr>
              )}
            </Fragment>
          ))}
        </tbody>
      </table>
    </div>
  );
}
