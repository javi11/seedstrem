# Show "Available until" in the Torrents table

**Date:** 2026-07-19
**Status:** Approved

## Problem

Completed torrents seed for a configured window (`Cleanup.SeedTime`, default 72h)
and are then removed by the cleanup loop (`internal/cleanup/cleanup.go`) once their
qBittorrent `SeedingTime` exceeds that limit. The admin Torrents table gives no
indication of when an item will be removed, so users cannot tell how much longer a
stream will remain available.

## Goal

Show, per downloaded/seeding torrent in the admin Torrents table, the absolute local
time at which it will be removed by seed-time cleanup.

## Non-goals (YAGNI)

- No new API endpoint.
- No config fetch on the Torrents page.
- No changes to cleanup logic.
- No changes to the Stremio "still downloading" placeholder.
- No countdown format (absolute deadline was chosen).

## Design

### Backend — expose the inputs

The `torrents()` handler in `internal/admin/router.go` already loads the config
(`cfg := h.config.Get()`) and per-torrent live qBittorrent info (`info`). Add two
fields to the `torrentItem` struct and populate them:

- `seeding_time` (`int64`, seconds) — from `info.SeedingTime` (a `time.Duration`),
  converted with `int64(info.SeedingTime / time.Second)`.
- `seed_time` (`int64`, seconds) — the configured limit, from
  `int64(cfg.Cleanup.SeedTime / time.Second)`.

The frontend owns the presentation so the deadline recomputes against the browser
clock on each 3s poll, rather than baking the server clock into a timestamp at fetch
time. This also avoids a sentinel value to distinguish "cleanup disabled" from
"not seeding yet".

### Frontend — new column

In `web/src/api.ts`, extend the `Torrent` interface with `seeding_time: number` and
`seed_time: number`.

In `web/src/pages/Torrents.tsx`, add an "Available until" column (header + cell).
A formatting helper `availableUntil(t: Torrent): string` decides the cell content:

1. **Not seeding yet** — `t.progress < 1` → `"—"`.
   (Not-in-qBittorrent already surfaces as `progress === 0`, so the same branch
   covers it.)
2. **Cleanup disabled** — `t.seed_time <= 0` → `"Kept"`.
3. **Deadline** — `remaining = t.seed_time - t.seeding_time` (seconds):
   - `remaining <= 0` → `"Removing soon"`.
   - otherwise → `new Date(Date.now() + remaining * 1000)` formatted as local
     short date + time, e.g. `"Jul 22, 14:00"`, via
     `toLocaleString(undefined, { month: "short", day: "numeric", hour: "2-digit", minute: "2-digit" })`.

The column is added after "Ratio". `colSpan` on the expanded-row `<td>` increases
from `8` to `9`.

## Testing

- **Backend:** extend the existing admin handler test (`internal/admin/admin_test.go`)
  so the `/api/torrents` response asserts `seed_time` and `seeding_time` are present
  and correct for a seeded torrent (fake qBittorrent already supports `SeedingTime`).
- **Frontend:** the web package declares a `test` script (`vitest run`) but vitest
  is **not** installed and there are currently **no** web tests. Installing a runner
  is out of scope for this change. Instead, keep `availableUntil` a **pure exported
  function** (module-level, not a closure) so it is trivially testable later, and
  verify its three branches manually via `npm run build` + a visual check of the
  table. If/when a runner is added, the helper is ready to unit-test without
  refactoring.

## Files touched

- `internal/admin/router.go` — two struct fields + population.
- `internal/admin/admin_test.go` — assert new fields.
- `web/src/api.ts` — two interface fields.
- `web/src/pages/Torrents.tsx` — column + `availableUntil` helper.
