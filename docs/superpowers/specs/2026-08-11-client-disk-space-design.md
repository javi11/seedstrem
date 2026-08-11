# Download-client disk space on the Dashboard

## Goal

Show, on the web UI Dashboard, how much disk space seedstrem's downloads
occupy and how much space remains available to the download client.

## Motivation

seedstrem already computes disk used/total in two places — the Stremio
stream gate (`internal/stremio/stream.go`) and the RSS grabber's
disk-management limits (`internal/rss/grabber.go`) — but never exposes
either over HTTP. A user whose streams are being silently refused by the
disk gate, or whose RSS grabs are being skipped, has no way to see why
short of reading logs.

## Decisions

Two choices shape the whole design:

1. **Free space comes from the download client**, not from a local
   `statfs`. qBittorrent and Deluge can each report free space on their
   own filesystem, which stays correct when the client runs on a
   different host than seedstrem. A local `statfs` would only be right
   under the same-filesystem assumption of the bundled compose setup.
2. **"Used" means the footprint of seedstrem's own torrents** — the sum
   of completed bytes across the torrents seedstrem tracks — not total
   disk usage and not every torrent in the client.

### Accepted consequence

Neither client API exposes disk *total*. Therefore:

- `used + free` does not equal disk total.
- No percentage or progress bar is possible.
- `used` is seedstrem's footprint; `free` is the whole filesystem's
  headroom. They are two independent figures, and the UI must not imply
  otherwise.

A percent-of-disk display would require the `statfs` total, which is the
assumption decision 1 deliberately moved away from. Out of scope.

## Design

### 1. Downloader interface

`downloader.Client` (`internal/downloader/downloader.go`) gains:

```go
// FreeSpace reports the bytes available on the client's download
// filesystem. Backends without the concept return ErrNotSupported.
FreeSpace(ctx context.Context) (int64, error)
```

`Swappable` (`internal/downloader/swap.go`) forwards it, as it does for
every other interface method — the hot-swap wrapper breaks otherwise.

Implementations:

- **qBittorrent** (`internal/qbit/client.go`): `SyncMainDataCtx(ctx, 0)`,
  returning `ServerState.FreeSpaceOnDisk`.
- **Deluge** (`internal/deluge/client.go`): `delugerpc.GetFreeSpace(ctx, "")`.
  The path argument is optional; empty means the daemon's default
  download directory.

`TorrentInfo` (`internal/downloader/types.go`) gains:

```go
Completed int64 // bytes of wanted data already on disk
```

Today `TorrentInfo` carries only `Size` (wanted) and `Progress` (0..1), so
used bytes would be a `Size × Progress` estimate. Both clients report the
real figure — qBittorrent `Completed`, Deluge `total_done` — so it is read
rather than multiplied.

### 2. Status endpoint

`GET /api/status` (`internal/admin/router.go`) gains a `disk` object:

```json
"disk": { "used": 0, "free": 0, "free_source": "client" }
```

- `used` — sum of `Completed` over the store's torrents joined with live
  client state. The handler's existing loop already performs that join,
  so this adds no client calls.
- `free` — `dc.FreeSpace(ctx)` first. On any error, including
  `ErrNotSupported`, fall back to `diskusage.Stat(diskPath)` and report
  `free = total - used_statfs` with `free_source: "local"`.
- `free_source` — `"client"` or `"local"`, so the UI can say which
  filesystem the figure describes.
- If the client call and the `statfs` fallback both fail, `free` and
  `free_source` are omitted; `used` is still reported, since it derives
  from data the handler already holds.

`Handler` gains a `diskPath string` field and an injectable
`diskUsage func(path string) (used, total int64, err error)`, defaulting
to `diskusage.Stat` — the same shape and wiring already used by
`stremio.Handler` and `rss.Grabber`, fed from `firstLocalMapping` in
`cmd/seedstrem/main.go`.

Response keys stay `snake_case` and carry raw `int64` byte counts,
matching the existing convention: the backend has no byte formatter and
the frontend formats.

### 3. Frontend

`Status` in `web/src/api.ts` gains a matching optional field. These types
are hand-written with no codegen, so the Go map keys and this interface
are kept in sync manually:

```ts
disk?: {
  used: number;
  free?: number;
  free_source?: "client" | "local";
};
```

Two `StatCard`s join the Dashboard tile grid (`web/src/pages/Dashboard.tsx`),
alongside the existing `Uploaded` tile, formatted with the existing
`formatBytes` from `web/src/lib/format.ts`:

- **Used** — hint `across N torrents`.
- **Free space** — hint `on <downloader name>` when `free_source` is
  `"client"`, `on host` when `"local"`. Surfacing the fallback makes a
  path-mapping mismatch visible instead of silent.

When `free` is absent, the tile renders a dash rather than disappearing.
The grid is already responsive (`grid-cols-2 sm:grid-cols-3 lg:grid-cols-4`),
so two more tiles need no layout change.

## Error handling

| Condition | Behaviour |
|---|---|
| Client reports free space | `free_source: "client"` |
| Client errors or returns `ErrNotSupported` | `statfs` fallback, `free_source: "local"` |
| Client and `statfs` both fail | `free`/`free_source` omitted; tile shows a dash |
| Store or client torrent list unavailable | `used` is `0`, consistent with the existing counts/uploaded behaviour in the same handler |

Nothing here fails the request: `/api/status` must keep serving version
and connectivity even when disk figures are unavailable.

## Testing

Go tests only. `web/src` contains no test files today, and two tiles do
not justify standing up a frontend suite.

- Fake `downloader.Client` in the admin handler tests:
  - client answers → `free_source: "client"`, value passed through
  - `ErrNotSupported` → `statfs` fallback, `free_source: "local"`
  - both fail → `free`/`free_source` absent, `used` still present
  - `used` sums `Completed` across tracked torrents
- qBittorrent and Deluge `FreeSpace` mapping tests, plus `Completed`
  population in their respective conversion tests.

The `diskUsage` injection follows the existing pattern in
`internal/rss/grabber_test.go`.

## Out of scope

- Disk-usage percentage or progress bar (requires a disk total).
- Showing the configured `max_disk_usage_percent` /
  `max_download_storage_gb` budget as the denominator.
- Per-torrent disk figures on the Torrents page.
- Historical or graphed disk usage.
