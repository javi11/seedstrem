# Adopting torrents added directly to the download client

## Goal

Let seedstrem discover, stream, and manage torrents a user added by hand
in Deluge or qBittorrent, identified by the label/category seedstrem
already configures — without ever touching torrents the user has not
marked.

## Motivation

Today a torrent only enters seedstrem's store through `EnsureAdded`
(`internal/torrents/service.go`), reached from the Stremio play flow and
the RSS grabber. Everything downstream is store-driven:

- `internal/syncer` reads `store.AllTorrents`, then asks the client about
  *those hashes only*.
- `internal/admin` lists `store.ListTorrents` and merges live info by hash.

Both backends deliberately refuse an unfiltered listing
(`internal/qbit/client.go:119`, `internal/deluge/client.go:248`) so that a
shared client instance never leaks unrelated torrents into seedstrem.

The consequence is that a torrent added directly in the client is
invisible: not listed, not streamable, not swept by cleanup. A user who
grabs a release by hand has no way to play it through seedstrem short of
re-adding it through Stremio.

## Decisions

Five choices shape the design:

1. **The label is the whole interface.** A torrent carrying seedstrem's
   configured Deluge label / qBittorrent category is adopted. Nothing
   else is. There is no per-torrent import button and no new UI verb —
   adopting is something the user does in their own client.
2. **The existing label is reused**, not a second `adopt_label` setting.
   Seedstrem already applies it to everything it adds
   (`internal/deluge/client.go:166`, `internal/qbit/client.go:57`), so
   there is one concept to learn, not two.
3. **Adopted torrents are managed exactly like native ones**, cleanup
   included. No exemption, no `include_adopted` flag.
4. **Adoption never writes to the client.** No file-priority changes, no
   sequential/first-last flag changes, no label writes. Seedstrem reads
   the torrent and links its video files; the user's own selection stands.
5. **Un-adoption is scoped to adopted rows.** Removing the label drops the
   row and its links, but only for rows adoption itself created. Native
   rows are unreachable from this path.

### Accepted consequence

Because adopted torrents are swept like native ones, **applying the label
in Deluge or qBittorrent arms seed-time/ratio auto-deletion, with files,
on that torrent.** This is intended: the label means "this is seedstrem's
now", and seedstrem deletes its own torrents when they have seeded
enough. Two mitigations keep it from surprising anyone:

- The feature is off by default (`adopt_labelled: false`), so upgrading
  cannot retroactively arm deletion on torrents that already carry the
  label.
- Removing the label un-adopts, which disarms cleanup for that torrent.
  It is a complete escape hatch and does not touch the torrent itself.

### Why native rows are protected

Deluge labelling is best-effort: without the Label plugin installed,
`applyLabel` silently no-ops (`internal/deluge/client.go:174`) and *no*
torrent carries a label. A rule of "no label ⇒ drop the row" would
therefore delete every row on the first scan of such an instance. Scoping
un-adoption to `origin = 'adopted'` makes that class of accident
structurally impossible, and additionally means a manual label edit can
never destroy an in-flight stream.

## Design

### Configuration

```yaml
downloader:
  type: qbittorrent
  adopt_labelled: false   # new

qbittorrent:
  category: seedstrem     # existing
deluge:
  label: seedstrem        # existing
```

`adopt_labelled` lives in the backend-neutral `downloader` section; the
label itself is read from whichever backend `downloader.type` selects, via
a new `Config.ClientLabel()` helper. Adoption is disabled when
`adopt_labelled` is false **or** the resolved label is empty.

### New `downloader.Client` method

```go
// TorrentsByLabel lists the client's torrents carrying label. Backends
// that cannot filter by label return ErrNotSupported.
TorrentsByLabel(ctx context.Context, label string) ([]TorrentInfo, error)
```

- **qBittorrent**: `GetTorrentsCtx` with `TorrentFilterOptions{Category: label}`.
- **Deluge**: `core.get_torrents_status` with a `{"label": <lowercased>}`
  filter dict; returns `ErrNotSupported` when the Label plugin is not in
  `GetEnabledPlugins`.
- `Swappable` (`internal/downloader/swap.go`) forwards it; the fake client
  implements it.

The filter is always explicit, so the "never list everything blindly"
invariant is preserved: seedstrem still cannot enumerate an unrelated
torrent.

### Store

Migration `0004_torrent_origin.sql`:

```sql
ALTER TABLE torrents ADD COLUMN origin TEXT NOT NULL DEFAULT 'native';
```

Existing rows become `native`, so no torrent already in the database is
droppable by the un-adopt path. New constants `store.OriginNative` and
`store.OriginAdopted`, an `Origin` field on `store.Torrent`, and an
`AdoptedTorrents(ctx)` query.

### New package `internal/adopt`

An `Adopter` with a `Run(ctx)` ticker loop and a `Scan(ctx)` pass,
matching the shape of `internal/syncer` and `internal/cleanup`. Default
interval 60s.

`Scan` does, in order:

1. Call `TorrentsByLabel`. On **any** error — transport failure,
   `ErrNotSupported`, auth — log and return immediately. Nothing is
   inserted and, critically, nothing is dropped. A client that cannot
   answer must never be read as "the label is gone from everything".
2. Look up the listed hashes with `store.TorrentsByHashes`. Hashes with no
   row are adoption candidates.
3. For each candidate, call `Files(hash)`. An empty file list means
   metadata has not resolved yet; skip and retry on the next tick.
   Otherwise:
   - Insert a torrent row: `origin = adopted`, `phase = selected`, `name`
     and `added_at` from the client, empty `magnet`, empty `indexer`,
     empty content identity.
   - Insert one link row, with a fresh token, per **video** file.
     `torrents.isVideo` is currently unexported; it is promoted to
     `torrents.IsVideo` so `adopt` can share the one extension list
     rather than duplicating it.
4. Un-adopt: every row from `AdoptedTorrents` whose hash is absent from
   the (successful) listing gets `DeleteTorrent`; link rows cascade. The
   torrent is left alone in the client.

`Scan` issues no writes to the download client at any point.

### Interaction with existing loops

Unchanged, all of them:

- **syncer** gives adopted rows the same treatment as native ones —
  sticky error when the torrent vanishes from the client, name backfill
  once metadata resolves. A torrent deleted in the client disappears from
  both the label listing and the by-hash query, so whichever loop ticks
  first wins: syncer may briefly show the sticky error before the next
  `Scan` drops the row, or the row may simply vanish. Both outcomes are
  acceptable; the row does not survive either way.
- **cleanup** sweeps adopted rows under the global seed time. With no
  indexer recorded, `EffectiveSeedTime("")` falls through to the global
  value, which is the correct behaviour for a hand-added torrent.
- **`EnsureAdded`** already reuses an existing row when the hash is known
  (`internal/torrents/service.go:102`), so a later play of the same
  release adopts the content identity onto the row and streams it. The
  `origin` stays `adopted`: anything that entered by label continues to
  be governed by the label.

### HTTP and UI

`origin` is added to the admin torrents item JSON
(`internal/admin/router.go`). The web table renders a small "adopted"
badge. Nothing else changes — there is no import action to expose.

## Error handling

| Condition | Behaviour |
| --- | --- |
| `adopt_labelled` false or label empty | Loop does not start |
| Listing call fails | Log at warn, return; no inserts, no drops |
| Deluge Label plugin missing | `ErrNotSupported`, logged once, treated as a failed listing |
| Metadata not yet resolved | Candidate skipped, retried next tick |
| Torrent has no video files | Row inserted with zero links; visible, not streamable |
| Torrent vanishes from client entirely | Absent from listing ⇒ un-adopted (row dropped) |

## Testing

**`internal/adopt`** — table-driven `Scan` tests against
`downloader/fake`:

- unknown labelled hash is adopted, with one link per video file
- already-known hash is skipped, and its row is not modified
- candidate with no file list is deferred, not inserted
- adopted row missing from the listing is dropped, links cascade
- listing error drops nothing, even when adopted rows exist
- `ErrNotSupported` is a clean no-op
- native rows are never dropped, whatever the listing contains
- no client writes are issued (fake records calls and asserts none)

**`internal/store`** — migration applies to a pre-0004 database, `origin`
round-trips, `AdoptedTorrents` returns only adopted rows.

**`internal/qbit` / `internal/deluge`** — `TorrentsByLabel` against the
existing per-backend test harnesses, including the Deluge
plugin-missing path returning `ErrNotSupported`.

## Out of scope

- Matching adopted torrents to Stremio catalog entries. Adopted torrents
  have no content identity until a play resolves onto them.
- Per-torrent import/eject actions in the web UI.
- Adopting by any signal other than the label (save path, tracker, age).
