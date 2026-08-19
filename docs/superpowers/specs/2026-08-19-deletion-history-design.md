# Torrent Deletion History (48h) — Design

Date: 2026-08-19
Status: Approved

## Problem

Torrents disappear from seedstrem and there is no way, after the fact, to
tell which code path removed one or why. Four independent paths delete a
torrent, and only one of them is seed-time related:

| Path | Site | Trigger |
|---|---|---|
| Cleanup sweep | `internal/cleanup/cleanup.go:130` | seed time OR target ratio |
| Abandoned stream | `internal/stream/handler.go:539` | progress below threshold, no viewers |
| Manual delete | `internal/admin/router.go:769` | operator clicked delete |
| Un-adopt | `internal/adopt/adopt.go:198` | label vanished; store-only row drop |

Un-adopt is the most confusing: it deletes the store row without touching
the download client, so a torrent vanishes from seedstrem while still
seeding in Deluge. That is easily misread as a seed-time bug.

Investigation confirmed seed-time config is applied live — `cmd/seedstrem/main.go:154`
passes cleanup a closure calling `cm.Get()`, and `Sweep` re-reads it every
pass (`internal/cleanup/cleanup.go:89`). Raising or lowering the seed time
takes effect on the next sweep for all current torrents. No per-torrent
snapshot exists. So the bug, if any, is elsewhere — hence this audit log.

## Goal

Record every completed torrent deletion for 48 hours, with the reason and
the evidence behind the decision, and surface it in the admin UI.

## Data model

New migration `internal/store/migrations/0005_torrent_deletions.sql`.
Standalone table with no foreign key to `torrents`: the row it describes is
being deleted and the record must outlive it.

```sql
CREATE TABLE torrent_deletions (
  id            TEXT PRIMARY KEY,   -- own id; a re-added torrent can be deleted twice
  torrent_id    TEXT NOT NULL,
  hash          TEXT NOT NULL,
  name          TEXT NOT NULL DEFAULT '',
  indexer       TEXT NOT NULL DEFAULT '',
  origin        TEXT NOT NULL DEFAULT '',
  deleted_at    INTEGER NOT NULL,           -- unix seconds
  reason        TEXT NOT NULL,              -- seed_time|ratio|manual|abandoned|unadopted
  seeding_time  INTEGER NOT NULL DEFAULT 0, -- observed, seconds
  seed_limit    INTEGER NOT NULL DEFAULT 0, -- effective limit at decision, seconds
  ratio         REAL    NOT NULL DEFAULT 0,
  ratio_limit   REAL    NOT NULL DEFAULT 0,
  progress      REAL    NOT NULL DEFAULT 0,
  files_deleted INTEGER NOT NULL DEFAULT 0  -- 0/1: was data removed from disk
);
CREATE INDEX idx_torrent_deletions_at ON torrent_deletions(deleted_at DESC);
```

Reason constants live in `internal/store` alongside the existing `Origin`
constants: `DeleteReasonSeedTime`, `DeleteReasonRatio`, `DeleteReasonManual`,
`DeleteReasonAbandoned`, `DeleteReasonUnadopted`.

`id` is generated inside `internal/store` by a small local helper mirroring
`internal/torrents/ids.go`. It cannot reuse `torrents.NewID` because
`internal/torrents` already imports `internal/store`, and the reverse
direction would be an import cycle.

The evidence columns are the point of the feature. A row reading
`seed_time, seeding_time=49h, seed_limit=48h, indexer=X` either confirms or
exonerates the seed-time path immediately.

Cleanup can satisfy both triggers in the same pass
(`internal/cleanup/cleanup.go:153`). Record a single primary `reason` with
seed time taking precedence over ratio, but populate BOTH pairs of evidence
columns so the decision is reconstructible either way.

`files_deleted` is taken from `DeleteFilesOnRemove` at
`internal/torrents/service.go:468`, so the history distinguishes "row gone,
data intact" from "data wiped" — a distinction currently unrecoverable.

## Capture points

A `store.RecordDeletion(ctx, DeletionEvent)` writer, called from exactly two
places:

1. **`Service.Remove`** gains a `DeletionEvent` parameter carrying reason and
   evidence. Its three callers fill in what only they know:
   - cleanup: effective seed time, observed seeding time, ratio, ratio limit
   - stream: progress and threshold, reason `abandoned`
   - admin: reason `manual`

   The signature change makes it a compile error to add a removal path
   without stating a reason.

2. **`adopt.unadopt`** calls the writer directly with reason `unadopted` and
   `files_deleted=0`, since it bypasses `Service` entirely.

Recording happens only after the removal succeeds. `Service.Remove` returns
early on download-client failure, so no record is written for a failed
attempt.

## Retention

48 hours, enforced two complementary ways:

- **Reads** filter `deleted_at >= now-48h`, guaranteeing correct display even
  on an idle instance where no write has triggered a prune.
- **Writes** prune rows older than the window, bounding table growth.

The window is a named constant in `internal/store`, not a config field.

## API

`GET /api/deletions` in the authenticated group of
`internal/admin/router.go`, newest first, last 48h, mirroring the existing
`/torrents` DTO style. Durations serialize as seconds, matching
`seeding_time` at `internal/admin/router.go:738`.

No filters or pagination in v1: 48h of deletions is a small, scannable set.

## UI

A fourth sidebar entry, **History** (`/history`), alongside
Dashboard/Torrents/Settings in `web/src/components/Sidebar.tsx:28`.

A single table: time, name, reason badge, evidence, indexer, files-deleted
indicator. The evidence column renders per reason:

- `seed_time` → `seeded 49h of 48h`
- `ratio` → `ratio 2.1 of 2.0`
- `abandoned` → `progress 12% of 50%`
- `manual` → `removed from the admin UI`
- `unadopted` → `label disappeared — still in client`

Reason gets a colour-coded badge so a burst of one cause is visible at a
glance.

A separate page rather than a tab on Torrents: it answers a different
question ("what happened?" vs "what is here now?") and is easier to reach
while actively debugging.

## Testing

- Store: write/read round-trip; retention boundary with a 47h row (visible)
  and a 49h row (hidden); prune-on-write.
- One test per capture path asserting the correct reason and evidence land,
  including `unadopted`, which no existing test covers.
- Router: endpoint shape and auth.
- Existing `Service.Remove` callers updated for the new signature; their
  current tests must keep passing.

## Non-goals

- Failed removal attempts are not recorded, only completed ones.
- No configurable retention, no filtering, pagination, or export.
- The seed-time-on-save immediate sweep discussed during brainstorming is
  explicitly parked and not part of this work.
