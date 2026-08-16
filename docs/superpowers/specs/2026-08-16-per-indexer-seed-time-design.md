# Per-indexer seed time

## Problem

`cleanup.seed_time` is a single global duration. Every torrent seeds for the
same time regardless of where it came from. Private trackers impose minimum
seed times (often days or weeks) while public trackers impose none, so one
global value forces a choice between violating tracker rules and wasting disk
on releases nobody needs to keep.

Make the seed time configurable per indexer, with the global value as the
fallback.

## Scope

In scope: per-indexer `seed_time` override, the plumbing needed to attribute a
torrent to an indexer, and editing the overrides in the admin UI.

Out of scope:

- per-indexer `target_ratio` or `delete_policy`
- tracker-host matching as a fallback attribution mechanism
- backfilling indexers for torrents already in the database

## Attribution: persist the indexer on the torrent

The indexer name is known at search time (`prowlarr.Result.Indexer`) and is
already displayed in Stremio stream names, but it is never persisted: neither
`store.Torrent` nor the play URL carries it. Cleanup therefore cannot tell one
torrent's origin from another's.

Persist it at add time. New torrents carry their Prowlarr indexer name; rows
that predate the change keep an empty indexer and use the global default.

### Migration

`internal/store/migrations/0003_torrent_indexer.sql`:

```sql
ALTER TABLE torrents ADD COLUMN indexer TEXT NOT NULL DEFAULT '';
```

`store.Torrent` gains `Indexer string`, added to `torrentCols`, `scanTorrent`,
and `InsertTorrent` — following the pattern of `0002_torrent_content.sql`. No
index: cleanup already loads all rows and matches in memory.

### Plumbing

- **`torrents.Selector`** gains `Indexer string`. The selector is already the
  "what was this torrent added for" carrier and `EnsureAdded` already persists
  its fields onto the new row.
- **`EnsureAdded` backfill.** On the existing-torrent path, if the stored row
  has an empty indexer and the incoming selector carries one, write it through
  `store.SetTorrentIndexer` and update the returned struct. This mirrors the
  existing `ContentRef` backfill. A non-empty stored indexer is never
  overwritten — the first attribution wins, so a re-play through a different
  indexer cannot silently change a torrent's retention.
- **Stremio.** `toStreamItem` sets `ix=<indexer>` on the play URL;
  `play` reads `r.URL.Query().Get("ix")` into the selector.
  `toOwnedStreamItem` sets `ix` from the stored torrent's indexer, so
  re-playing an owned torrent does not clear it.
- **RSS grabber.** Passes `torrents.Selector{Indexer: r.Indexer}` instead of
  the empty selector.

Indexer names are stored verbatim as Prowlarr reports them; normalization
happens only at lookup time.

## Configuration

```yaml
cleanup:
  seed_time: 72h # global default, unchanged
  indexer_seed_times: # per-indexer overrides
    TorrentLeech: 240h
    Nyaa: 12h
```

`config.Cleanup` gains:

```go
// IndexerSeedTimes overrides SeedTime per Prowlarr indexer name.
// Matching is case-insensitive; an indexer with no entry uses SeedTime.
// A value of 0 disables seed-time removal for that indexer.
IndexerSeedTimes map[string]time.Duration `yaml:"indexer_seed_times"`
```

Default: empty (nil map) — behavior is unchanged for existing deployments.

**Env override.** `SEEDSTREM_CLEANUP_INDEXER_SEED_TIMES="TorrentLeech=240h,Nyaa=12h"`
replaces the whole map, consistent with the other env overrides in
`applyEnv`. Entries are split on `,` then on the first `=`; an entry whose
duration fails to parse, or whose name is blank, is skipped — matching the
existing env parsers, which ignore malformed values rather than failing
startup. An empty value leaves the YAML map in place.

**Validation** (`Validate`): any negative duration is an error, naming the
offending indexer, e.g.
`cleanup.indexer_seed_times["Nyaa"] must not be negative (0 disables seed-time cleanup)`.
Blank indexer names are an error. Zero is valid and means "never remove this
indexer's torrents by seed time", the same meaning zero has globally.

## Cleanup

`cleanup.Settings` gains `IndexerSeedTimes map[string]time.Duration`, populated
in `cmd/seedstrem/main.go` alongside `SeedTime`.

Resolution lives in one pure method so it can be tested on its own:

```go
// EffectiveSeedTime returns the seed time governing a torrent from the
// named indexer: the per-indexer override when one exists, otherwise the
// global SeedTime. Matching is case-insensitive and whitespace-trimmed.
func (s Settings) EffectiveSeedTime(indexer string) time.Duration
```

An empty indexer always resolves to the global `SeedTime`.

`selectRemovals` computes the trigger per torrent:

```go
st := s.EffectiveSeedTime(tor.Indexer)
byTime := st > 0 && info.SeedingTime >= st
```

`Sweep`'s early-out must account for overrides. Today it returns immediately
when `SeedTime <= 0 && TargetRatio <= 0`; with overrides in play, a global
`seed_time: 0` plus a per-indexer override must still sweep. The condition
becomes: return early only when `TargetRatio <= 0`, `SeedTime <= 0`, and every
value in `IndexerSeedTimes` is `<= 0`.

Delete ordering (`orderRemovals`) is untouched.

## Admin API and UI

**Config DTO** (`internal/admin/router.go`). `cleanup.indexer_seed_times` is a
JSON list, not an object:

```json
"indexer_seed_times": [{ "indexer": "TorrentLeech", "seed_time_hours": 240 }]
```

A list gives stable ordering for React keys and lets a row's name be edited
without rebuilding an object. Read converts the map to a list sorted by
indexer name; write converts back to a map, trimming names, dropping entries
with a blank name, and rejecting negative hours with a 400. Duplicate names
after trimming and case-folding are rejected with a 400 rather than silently
collapsing.

Hours are the unit here, matching the existing `seed_time_hours` field.

**Torrent listing DTO.** `torrentItem.SeedTime` currently reports the global
`cfg.Cleanup.SeedTime` for every row, which would make the "time left"
countdown in `web/src/lib/format.ts` wrong for any overridden indexer. It
becomes the effective seed time for that torrent. `torrentItem` also gains
`indexer` so the UI can show which rule applies.

**UI.** `web/src/pages/Settings/sections/Seeding.tsx` grows an add/remove row
list under the existing global seed-time field: each row is an indexer name
text input and an hours number input. `web/src/api.ts` types are updated to
match both DTO changes. The torrents table shows the indexer name where it has
room; the existing time-left rendering needs no change once the DTO carries
the effective value.

## Testing

Table-driven, matching the existing test style. `go test -race ./...`.

- `internal/config`: default is an empty map; YAML parses the map; the env
  override replaces it and skips malformed entries; validation rejects
  negative durations and blank names, accepts zero.
- `internal/cleanup`: `EffectiveSeedTime` — exact match, case-insensitive and
  whitespace-trimmed match, unknown indexer falls back to global, empty
  indexer falls back to global, zero override disables, override applies when
  the global is zero. `selectRemovals` with a mix of indexers under one
  settings value. `Sweep` early-out does not fire when the global is zero but
  an override is positive.
- `internal/store`: insert-and-read round trip through the new column;
  `SetTorrentIndexer`; a row written before the migration reads back `""`.
- `internal/torrents`: `EnsureAdded` persists the selector's indexer; the
  backfill fills an empty stored indexer and leaves a non-empty one alone.
- `internal/stremio`: `toStreamItem` and `toOwnedStreamItem` emit `ix`; `play`
  parses it into the selector; a play URL with no `ix` yields an empty indexer.
- `internal/admin`: config DTO round trip (map → sorted list → map), rejection
  of negative hours and duplicate names, blank names dropped; the torrent
  listing reports the effective per-torrent seed time.

## Compatibility

Existing configs are unaffected: no `indexer_seed_times` means an empty map and
today's behavior. Torrents already in the database have an empty indexer and
keep using the global `seed_time` until they are re-added. The migration is
additive with a default, so a rollback to the previous binary leaves a
harmless unused column.
