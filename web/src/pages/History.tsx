import { api } from "../api";
import { usePolling } from "../lib/usePolling";
import { deletionEvidence, deletionReasonClass, deletionReasonLabel } from "../lib/deletions";
import { FreshnessIndicator } from "../components/FreshnessIndicator";
import { PageHeader } from "../components/PageHeader";
import { Skeleton } from "../components/Skeleton";

function when(unixSeconds: number): string {
  return new Date(unixSeconds * 1000).toLocaleString(undefined, {
    month: "short",
    day: "numeric",
    hour: "2-digit",
    minute: "2-digit",
  });
}

export function History() {
  const { data, isStale, isOffline, lastUpdated } = usePolling(api.deletions, {
    baseIntervalMs: 10000,
  });

  // First load.
  if (!data) {
    return (
      <div className="flex flex-col gap-6">
        <PageHeader title="History" />
        <div className="surface p-4">
          {Array.from({ length: 5 }).map((_, i) => (
            <Skeleton key={i} className="mb-3 h-10 w-full" />
          ))}
        </div>
      </div>
    );
  }

  return (
    <div className="flex flex-col gap-6">
      <PageHeader
        title="History"
        subtitle={
          data.length > 0
            ? `${data.length} removal${data.length === 1 ? "" : "s"} in the last 48h`
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

      {data.length === 0 ? (
        <div className="surface p-10 text-center">
          <div className="text-4xl">🗒️</div>
          <h2 className="mt-3 text-lg font-bold tracking-brand">Nothing removed recently</h2>
          <p className="mx-auto mt-1 max-w-md text-sm opacity-70">
            Torrents removed in the last 48 hours appear here, with the reason and the
            measurement behind each decision.
          </p>
        </div>
      ) : (
        <>
          {/* Desktop table */}
          <div className="surface hidden overflow-x-auto md:block">
            <table className="table">
              <thead>
                <tr>
                  <th>Removed</th>
                  <th>Name</th>
                  <th>Reason</th>
                  <th>Evidence</th>
                  <th>Indexer</th>
                  <th>Files</th>
                </tr>
              </thead>
              <tbody>
                {data.map((d) => (
                  <tr key={d.id}>
                    <td className="whitespace-nowrap text-sm opacity-70">
                      {when(d.deleted_at)}
                    </td>
                    <td className="max-w-xs">
                      <div className="truncate font-medium">{d.name || d.hash}</div>
                    </td>
                    <td>
                      <span className={`badge ${deletionReasonClass(d.reason)} badge-sm`}>
                        {deletionReasonLabel(d.reason)}
                      </span>
                    </td>
                    <td className="text-sm opacity-80">{deletionEvidence(d)}</td>
                    <td className="text-sm opacity-70">{d.indexer || "—"}</td>
                    <td className="text-sm opacity-70">
                      {d.files_deleted ? "Deleted" : "Kept"}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>

          {/* Mobile cards */}
          <div className="flex flex-col gap-3 md:hidden">
            {data.map((d) => (
              <div key={d.id} className="surface p-4">
                <div className="flex items-start justify-between gap-2">
                  <div className="truncate font-medium">{d.name || d.hash}</div>
                  <span
                    className={`badge ${deletionReasonClass(d.reason)} badge-sm shrink-0`}
                  >
                    {deletionReasonLabel(d.reason)}
                  </span>
                </div>
                <div className="mt-1 text-sm opacity-80">{deletionEvidence(d)}</div>
                <div className="mt-2 flex justify-between text-xs opacity-60">
                  <span>{when(d.deleted_at)}</span>
                  <span>{d.files_deleted ? "Files deleted" : "Files kept"}</span>
                </div>
              </div>
            ))}
          </div>
        </>
      )}
    </div>
  );
}
