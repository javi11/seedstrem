# Adopting label-marked torrents Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let seedstrem discover, stream, and manage torrents a user added by hand in Deluge or qBittorrent, identified by the label/category seedstrem already configures.

**Architecture:** A new `internal/adopt` package runs a background scan loop alongside the existing `syncer` and `cleanup` loops. Each pass asks the download client for torrents carrying seedstrem's label (a new `downloader.Client.TorrentsByLabel` method — the client is still never asked for an unfiltered listing), inserts store rows tagged `origin = 'adopted'` for hashes it does not know, and deletes previously-adopted rows whose label has gone. Adoption never writes to the download client.

**Tech Stack:** Go 1.x, SQLite (`internal/store` with embedded numbered migrations), React + TypeScript web UI under `web/`.

## Global Constraints

- The spec is `docs/superpowers/specs/2026-08-17-adopt-labelled-torrents-design.md`. Read it before starting.
- **Adoption issues no writes to the download client.** No file priorities, no sequential/first-last flags, no label writes, no start/stop. Only reads.
- **Un-adoption may only ever delete rows with `origin = 'adopted'`.** Native rows are unreachable from that path.
- **A failed listing must drop nothing.** Any error from `TorrentsByLabel` — transport, auth, `ErrNotSupported` — aborts the pass before any delete.
- Backends must never be asked for an unfiltered torrent list; the existing guards at `internal/qbit/client.go:119` and `internal/deluge/client.go:248` stay.
- Config key is `downloader.adopt_labelled` (the `downloader` YAML section, which already holds `type`), default `false`.
- Go tests run with `-race`. Every task ends green on `go build ./... && go test -race ./...`.
- Conventional Commits for every commit (`feat:`, `test:`, `refactor:`, scope optional).

---

### Task 1: Store — `origin` column

**Files:**
- Create: `internal/store/migrations/0004_torrent_origin.sql`
- Modify: `internal/store/torrents.go` (phase constants block at :12, `Torrent` struct at :24, `torrentCols` at :42, `scanTorrent` at :43, `InsertTorrent` at :57)
- Test: `internal/store/torrent_origin_test.go` (create)

**Interfaces:**
- Consumes: nothing (first task).
- Produces:
  - `store.OriginNative = "native"`, `store.OriginAdopted = "adopted"` constants
  - `store.Torrent.Origin string` field
  - `func (s *Store) AdoptedTorrents(ctx context.Context) ([]Torrent, error)`

- [ ] **Step 1: Write the failing test**

Create `internal/store/torrent_origin_test.go`. Look at `internal/store/store_test.go` first for how a test store is opened; reuse that helper verbatim rather than inventing a new one.

```go
package store_test

import (
	"context"
	"testing"

	"github.com/javib/seedstrem/internal/store"
)

func TestOriginDefaultsToNative(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t) // helper from store_test.go

	if err := st.InsertTorrent(ctx, store.Torrent{
		ID: "T1", Hash: "aaaa", Name: "native one", Phase: store.PhaseAdded, AddedAt: 100,
	}); err != nil {
		t.Fatalf("insert: %v", err)
	}

	got, err := st.TorrentByHash(ctx, "aaaa")
	if err != nil {
		t.Fatalf("by hash: %v", err)
	}
	if got.Origin != store.OriginNative {
		t.Fatalf("origin = %q, want %q", got.Origin, store.OriginNative)
	}
}

func TestAdoptedTorrentsReturnsOnlyAdopted(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)

	if err := st.InsertTorrent(ctx, store.Torrent{
		ID: "T1", Hash: "aaaa", Phase: store.PhaseAdded, AddedAt: 100, Origin: store.OriginNative,
	}); err != nil {
		t.Fatalf("insert native: %v", err)
	}
	if err := st.InsertTorrent(ctx, store.Torrent{
		ID: "T2", Hash: "bbbb", Phase: store.PhaseSelected, AddedAt: 200, Origin: store.OriginAdopted,
	}); err != nil {
		t.Fatalf("insert adopted: %v", err)
	}

	got, err := st.AdoptedTorrents(ctx)
	if err != nil {
		t.Fatalf("adopted: %v", err)
	}
	if len(got) != 1 || got[0].ID != "T2" {
		t.Fatalf("got %+v, want exactly the adopted row T2", got)
	}
}

func TestInsertTorrentEmptyOriginStoredAsNative(t *testing.T) {
	// Callers that predate this column leave Origin zero; the row must
	// still be a native row, never an empty-string origin that the
	// un-adopt path could not classify.
	ctx := context.Background()
	st := newTestStore(t)

	if err := st.InsertTorrent(ctx, store.Torrent{
		ID: "T1", Hash: "aaaa", Phase: store.PhaseAdded, AddedAt: 100, Origin: "",
	}); err != nil {
		t.Fatalf("insert: %v", err)
	}
	adopted, err := st.AdoptedTorrents(ctx)
	if err != nil {
		t.Fatalf("adopted: %v", err)
	}
	if len(adopted) != 0 {
		t.Fatalf("got %d adopted rows, want 0", len(adopted))
	}
}
```

If `newTestStore` does not exist under that name in `store_test.go`, use whatever the existing helper is called — do not add a second one.

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/store/ -run 'TestOrigin|TestAdoptedTorrents|TestInsertTorrentEmptyOrigin' -v`
Expected: FAIL — compile error, `undefined: store.OriginNative`.

- [ ] **Step 3: Write the migration**

Create `internal/store/migrations/0004_torrent_origin.sql`:

```sql
-- origin records how a torrent entered seedstrem: 'native' when
-- seedstrem added it, 'adopted' when it was discovered in the download
-- client by label. Only adopted rows may be removed when their label
-- disappears, so existing rows default to native and stay untouchable.
ALTER TABLE torrents ADD COLUMN origin TEXT NOT NULL DEFAULT 'native';

CREATE INDEX idx_torrents_origin ON torrents(origin);
```

- [ ] **Step 4: Add the constants and struct field**

In `internal/store/torrents.go`, extend the phase constants block:

```go
// Phase values for Torrent.Phase.
const (
	PhaseAdded    = "added"
	PhaseSelected = "selected"
)

// Origin values for Torrent.Origin: how the torrent entered seedstrem.
// Only OriginAdopted rows are removable by the label scan
// (internal/adopt); OriginNative rows are created by seedstrem itself
// and are never dropped for lacking a label.
const (
	OriginNative  = "native"
	OriginAdopted = "adopted"
)
```

Add to the `Torrent` struct, after `Indexer`:

```go
	// Origin is OriginNative or OriginAdopted; see the Origin constants.
	Origin string
```

- [ ] **Step 5: Thread the column through the queries**

Update `torrentCols`:

```go
const torrentCols = `id, hash, name, phase, added_at, magnet, error, content_source, content_ref, season, episode, indexer, origin`
```

Add `&t.Origin` as the final scan target in `scanTorrent`, matching the new column order.

In `InsertTorrent`, normalize an empty origin and store it. The existing function inserts a fixed column list — add `origin` to both the column list and the values, with this normalization immediately before the `Exec`:

```go
	origin := t.Origin
	if origin == "" {
		origin = OriginNative
	}
```

and pass `origin` (not `t.Origin`) as the bound value.

- [ ] **Step 6: Add `AdoptedTorrents`**

In `internal/store/torrents.go`, model it on the existing `AllTorrents` at :213:

```go
// AdoptedTorrents returns every torrent adopted from the download client
// by label (Origin == OriginAdopted). The label scan uses it to find rows
// whose label has since been removed.
func (s *Store) AdoptedTorrents(ctx context.Context) ([]Torrent, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+torrentCols+` FROM torrents WHERE origin = ? ORDER BY added_at DESC`, OriginAdopted)
	if err != nil {
		return nil, fmt.Errorf("query adopted torrents: %w", err)
	}
	defer rows.Close()

	var out []Torrent
	for rows.Next() {
		t, err := scanTorrent(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}
```

- [ ] **Step 7: Run the tests to verify they pass**

Run: `go test -race ./internal/store/ -v`
Expected: PASS, including the pre-existing store tests (they exercise the migration runner, so a broken migration shows up here).

- [ ] **Step 8: Commit**

```bash
git add internal/store/
git commit -m "feat(store): record how a torrent entered seedstrem"
```

---

### Task 2: Downloader — `TorrentsByLabel` and `TorrentInfo.AddedAt`

**Files:**
- Modify: `internal/downloader/downloader.go` (the `Client` interface), `internal/downloader/types.go` (`TorrentInfo`), `internal/downloader/swap.go`, `internal/downloader/fake/fake.go`, `internal/qbit/client.go`, `internal/deluge/client.go`
- Create: `internal/deluge/delugerpc/labels.go`
- Test: `internal/qbit/client_test.go` (extend), `internal/deluge/client_test.go` (extend)

**Interfaces:**
- Consumes: nothing from Task 1.
- Produces:
  - `downloader.Client.TorrentsByLabel(ctx context.Context, label string) ([]TorrentInfo, error)`
  - `downloader.TorrentInfo.AddedAt time.Time` (zero when the backend does not report it)
  - `fake.Torrent.AddedAt time.Time` (fake torrents are filtered by their existing `Category` field)
  - `delugerpc.Client.TorrentsStatusByLabel(ctx context.Context, label string) (map[string]*TorrentStatus, error)`

- [ ] **Step 1: Write the failing tests**

Add to `internal/qbit/client_test.go`. Read the file first — it already stands up an `httptest` server speaking the qBittorrent WebUI API; extend that harness, do not build a second one.

```go
func TestTorrentsByLabelFiltersByCategory(t *testing.T) {
	// The category must reach qBittorrent as a request parameter: asking
	// for everything and filtering locally would pull unrelated torrents
	// off a shared instance.
	var gotCategory string
	srv := newFakeQbit(t, func(r *http.Request) any {
		gotCategory = r.URL.Query().Get("category")
		return []map[string]any{{
			"hash": "AAAA", "name": "hand added", "state": "stalledUP",
			"added_on": int64(1700000000),
		}}
	})
	c := qbit.New(srv.URL, "u", "p", "seedstrem")

	got, err := c.TorrentsByLabel(context.Background(), "seedstrem")
	if err != nil {
		t.Fatalf("by label: %v", err)
	}
	if gotCategory != "seedstrem" {
		t.Fatalf("category param = %q, want %q", gotCategory, "seedstrem")
	}
	if len(got) != 1 || got[0].Hash != "AAAA" {
		t.Fatalf("got %+v, want one torrent AAAA", got)
	}
	if got[0].AddedAt.Unix() != 1700000000 {
		t.Fatalf("AddedAt = %v, want unix 1700000000", got[0].AddedAt)
	}
}

func TestTorrentsByLabelEmptyLabelReturnsNothing(t *testing.T) {
	// An empty category means "uncategorised" to qBittorrent, not "none",
	// so it must never reach the wire.
	srv := newFakeQbit(t, func(r *http.Request) any {
		t.Fatalf("unexpected request for %s", r.URL)
		return nil
	})
	c := qbit.New(srv.URL, "u", "p", "")

	got, err := c.TorrentsByLabel(context.Background(), "")
	if err != nil {
		t.Fatalf("by label: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("got %d torrents, want 0", len(got))
	}
}
```

Adapt `newFakeQbit(...)` to whatever the existing harness in that file is named and shaped like; the assertions are what matter.

Add to `internal/deluge/client_test.go` — that file already has a fake `api` implementation; extend it with the new method:

```go
func TestTorrentsByLabelRequiresLabelPlugin(t *testing.T) {
	f := &fakeAPI{plugins: []string{"Execute"}} // no Label plugin
	c := deluge.NewWithAPI(f, "seedstrem")      // see note below

	_, err := c.TorrentsByLabel(context.Background(), "seedstrem")
	if !errors.Is(err, downloader.ErrNotSupported) {
		t.Fatalf("err = %v, want ErrNotSupported", err)
	}
}

func TestTorrentsByLabelReturnsLabelledTorrents(t *testing.T) {
	f := &fakeAPI{
		plugins: []string{"Label"},
		byLabel: map[string]map[string]*delugerpc.TorrentStatus{
			"seedstrem": {"aaaa": {Name: "hand added", TimeAdded: 1700000000}},
		},
	}
	c := deluge.NewWithAPI(f, "seedstrem")

	got, err := c.TorrentsByLabel(context.Background(), "seedstrem")
	if err != nil {
		t.Fatalf("by label: %v", err)
	}
	if len(got) != 1 || got[0].Hash != "aaaa" {
		t.Fatalf("got %+v, want one torrent aaaa", got)
	}
	if got[0].AddedAt.Unix() != 1700000000 {
		t.Fatalf("AddedAt = %v, want unix 1700000000", got[0].AddedAt)
	}
}
```

Use whichever constructor the existing Deluge tests use to inject the fake `api` (the interface is declared at `internal/deluge/client.go:22`); `NewWithAPI` above is a placeholder for that existing seam — do not add a new one.

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/qbit/ ./internal/deluge/ -run TorrentsByLabel -v`
Expected: FAIL — compile error, `c.TorrentsByLabel undefined`.

- [ ] **Step 3: Add `AddedAt` to `TorrentInfo`**

In `internal/downloader/types.go`, add to `TorrentInfo` after `SeedingTime`:

```go
	// AddedAt is when the download client took the torrent on. Zero when
	// the backend does not report it.
	AddedAt time.Time
```

- [ ] **Step 4: Extend the `Client` interface**

In `internal/downloader/downloader.go`, add to the `Client` interface, directly under `Torrents`:

```go
	// TorrentsByLabel lists the client's torrents carrying label (a
	// Deluge label, a qBittorrent category). Unlike Torrents it is a
	// server-side filter, so it never returns torrents unrelated to
	// seedstrem on a shared instance. An empty label returns no
	// torrents. Backends that cannot filter by label — Deluge without
	// the Label plugin — return ErrNotSupported.
	TorrentsByLabel(ctx context.Context, label string) ([]TorrentInfo, error)
```

- [ ] **Step 5: Forward it through `Swappable`**

In `internal/downloader/swap.go`, after the `Torrents` method:

```go
func (s *Swappable) TorrentsByLabel(ctx context.Context, label string) ([]TorrentInfo, error) {
	return s.get().TorrentsByLabel(ctx, label)
}
```

- [ ] **Step 6: Implement it in the fake**

In `internal/downloader/fake/fake.go`, add `AddedAt time.Time` to the `Torrent` struct (after `SeedingTime`), map it in `toTorrentInfo` (`AddedAt: t.AddedAt,`), and add:

```go
// TorrentsByLabel returns the fake torrents whose Category matches label.
func (s *Server) TorrentsByLabel(_ context.Context, label string) ([]downloader.TorrentInfo, error) {
	s.record("TorrentsByLabel(%s)", label)
	if label == "" {
		return nil, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []downloader.TorrentInfo
	for _, t := range s.torrents {
		if strings.EqualFold(t.Category, label) {
			out = append(out, toTorrentInfo(t))
		}
	}
	return out, nil
}
```

Also add a way for tests to force a failure, next to the existing `SetFreeSpaceErr` at :94:

```go
// SetTorrentsByLabelErr makes TorrentsByLabel fail, so callers can be
// tested against an unreachable or incapable download client.
func (s *Server) SetTorrentsByLabelErr(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.byLabelErr = err
}
```

with a `byLabelErr error` field on `Server`, returned first thing in `TorrentsByLabel` when non-nil.

- [ ] **Step 7: Implement it in the qBittorrent backend**

In `internal/qbit/client.go`, populate `AddedAt` in `convertTorrent` (:97) by adding `AddedAt: time.Unix(t.AddedOn, 0),`, then add after `Torrents`:

```go
func (c *client) TorrentsByLabel(ctx context.Context, label string) ([]downloader.TorrentInfo, error) {
	if label == "" {
		// An empty category means "uncategorised" to qBittorrent, which
		// is not what a caller with no label configured is asking for.
		return nil, nil
	}
	list, err := c.qb.GetTorrentsCtx(ctx, qbt.TorrentFilterOptions{Category: label})
	if err != nil {
		return nil, fmt.Errorf("qbit list torrents by category: %w", err)
	}
	infos := make([]downloader.TorrentInfo, 0, len(list))
	for _, t := range list {
		infos = append(infos, convertTorrent(t))
	}
	return infos, nil
}
```

- [ ] **Step 8: Add the Deluge RPC call**

Create `internal/deluge/delugerpc/labels.go`. It lives in the vendored package for the same reason as `seedstrem.go` — it needs the unexported rpc plumbing:

```go
// seedstrem: local addition to the vendored go-deluge library, like
// seedstrem.go. Licensed GPL-2.0 with the rest of the package.
package delugerpc

import (
	"context"

	"github.com/gdm85/go-rencode"
)

// TorrentsStatusByLabel returns the status of every torrent carrying
// label. The filter is applied by the daemon (the Label plugin adds a
// "label" key to core.get_torrents_status's filter dict), so torrents
// belonging to other tools never cross the wire.
func (c *Client) TorrentsStatusByLabel(ctx context.Context, label string) (map[string]*TorrentStatus, error) {
	var filterDict rencode.Dictionary
	filterDict.Add("label", label)

	var args rencode.List
	args.Add(filterDict)
	if !c.v2daemon {
		args.Add(statusKeysV1)
	} else {
		args.Add(statusKeysV2)
	}

	rd, err := c.rpcWithDictionaryResult(ctx, "core.get_torrents_status", args, rencode.Dictionary{})
	if err != nil {
		return nil, err
	}
	d, err := rd.Zip()
	if err != nil {
		return nil, err
	}

	result := map[string]*TorrentStatus{}
	for k, rv := range d {
		v, ok := rv.(rencode.Dictionary)
		if !ok {
			return nil, ErrInvalidDictionaryResponse
		}
		var ts TorrentStatus
		if err := v.ToStruct(&ts, c.excludeTag); err != nil {
			return nil, err
		}
		if !c.v2daemon {
			ts.DownloadLocation = ts.SavePath
		}
		result[k] = &ts
	}
	return result, nil
}
```

Cross-check the tail of `TorrentsStatus` at `internal/deluge/delugerpc/torrent_status.go:164` and mirror its result-decoding loop exactly — if it does anything extra after `ToStruct`, do the same here.

- [ ] **Step 9: Implement it in the Deluge backend**

In `internal/deluge/client.go`: add `TorrentsStatusByLabel(ctx context.Context, label string) (map[string]*delugerpc.TorrentStatus, error)` to the `api` interface (:22), populate `AddedAt` in `convert.go`'s `convertTorrent` from `ts.TimeAdded` (`time.Unix(int64(ts.TimeAdded), 0)`), and add:

```go
func (c *client) TorrentsByLabel(ctx context.Context, label string) ([]downloader.TorrentInfo, error) {
	if label == "" {
		return nil, nil
	}
	var infos []downloader.TorrentInfo
	err := c.do(ctx, func(ctx context.Context) error {
		plugins, err := c.rpc.GetEnabledPlugins(ctx)
		if err != nil {
			return fmt.Errorf("deluge enabled plugins: %w", err)
		}
		if !slices.Contains(plugins, "Label") {
			// Without the Label plugin no torrent carries a label at
			// all, so a label query cannot be answered — not even
			// negatively.
			return downloader.ErrNotSupported
		}
		statuses, err := c.rpc.TorrentsStatusByLabel(ctx, strings.ToLower(label))
		if err != nil {
			return fmt.Errorf("deluge list torrents by label: %w", err)
		}
		infos = make([]downloader.TorrentInfo, 0, len(statuses))
		for hash, ts := range statuses {
			info := convertTorrent(hash, ts)
			f := c.cachedFlags(info.Hash)
			info.SequentialDownload, info.FirstLastPiecePrio = f.seq, f.flp
			infos = append(infos, info)
		}
		return nil
	})
	return infos, err
}
```

Check what `c.do` does with a returned error (`internal/deluge/client.go`, around :140) — if it wraps, make sure `errors.Is(err, downloader.ErrNotSupported)` still holds at the call site, and use `%w` wrapping if not.

- [ ] **Step 10: Run the tests to verify they pass**

Run: `go build ./... && go test -race ./internal/qbit/ ./internal/deluge/ ./internal/downloader/... -v`
Expected: PASS. Any other implementer of `downloader.Client` in the tree will fail to compile here — fix those by adding the method; `go build ./...` finds them all.

- [ ] **Step 11: Commit**

```bash
git add internal/downloader/ internal/qbit/ internal/deluge/
git commit -m "feat(downloader): list a client's torrents by label"
```

---

### Task 3: The `adopt` package

**Files:**
- Create: `internal/adopt/adopt.go`, `internal/adopt/adopt_test.go`
- Modify: `internal/torrents/select.go:56` (export `isVideo` as `IsVideo`) and its call sites within `internal/torrents/`

**Interfaces:**
- Consumes: `store.OriginAdopted`, `store.AdoptedTorrents` (Task 1); `downloader.Client.TorrentsByLabel`, `TorrentInfo.AddedAt` (Task 2); the existing `torrents.NewID`, `torrents.NewLinkToken`, `store.InsertTorrent`, `store.InsertLinks`, `store.TorrentsByHashes`, `store.DeleteTorrent`.
- Produces:
  - `func adopt.New(st *store.Store, dc downloader.Client, settings func() adopt.Settings, logger *slog.Logger, interval time.Duration) *adopt.Adopter`
  - `type adopt.Settings struct { Enabled bool; Label string }`
  - `func (a *Adopter) Run(ctx context.Context)`
  - `func (a *Adopter) Scan(ctx context.Context) error`
  - `torrents.IsVideo(name string) bool`

- [ ] **Step 1: Export `IsVideo`**

In `internal/torrents/select.go`, rename `isVideo` to `IsVideo` and update its doc comment:

```go
// IsVideo reports whether name has a known video extension.
func IsVideo(name string) bool {
	return videoExts[strings.ToLower(path.Ext(name))]
}
```

Update every call site inside `internal/torrents/`:

```bash
grep -rn "isVideo(" internal/torrents/
```

Run: `go build ./... && go test -race ./internal/torrents/`
Expected: PASS — a pure rename.

- [ ] **Step 2: Commit the rename separately**

```bash
git add internal/torrents/
git commit -m "refactor(torrents): export IsVideo for reuse"
```

- [ ] **Step 3: Write the failing tests**

Create `internal/adopt/adopt_test.go`. It uses the real store (as `internal/cleanup`'s tests do — read `internal/cleanup/cleanup_test.go` for the store-plus-fake setup pattern) and `internal/downloader/fake`.

```go
package adopt_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/javib/seedstrem/internal/adopt"
	"github.com/javib/seedstrem/internal/downloader"
	"github.com/javib/seedstrem/internal/downloader/fake"
	"github.com/javib/seedstrem/internal/store"
)

func settings() adopt.Settings { return adopt.Settings{Enabled: true, Label: "seedstrem"} }

// newAdopter wires a real store over a temp database to a fake client.
func newAdopter(t *testing.T) (*adopt.Adopter, *store.Store, *fake.Server) {
	t.Helper()
	st := newTestStore(t) // same pattern as internal/cleanup/cleanup_test.go
	dc := fake.New()
	return adopt.New(st, dc, settings, nil, time.Minute), st, dc
}

func TestScanAdoptsLabelledTorrent(t *testing.T) {
	ctx := context.Background()
	a, st, dc := newAdopter(t)
	dc.Put(&fake.Torrent{
		Hash: "aaaa", Name: "Hand Added", Category: "seedstrem",
		AddedAt: time.Unix(1700000000, 0),
		Files: []fake.File{
			{Name: "Hand.Added.S01E01.mkv", Size: 100},
			{Name: "readme.nfo", Size: 5},
			{Name: "Hand.Added.S01E02.mkv", Size: 200},
		},
	})

	if err := a.Scan(ctx); err != nil {
		t.Fatalf("scan: %v", err)
	}

	tor, err := st.TorrentByHash(ctx, "aaaa")
	if err != nil {
		t.Fatalf("by hash: %v", err)
	}
	if tor.Origin != store.OriginAdopted {
		t.Fatalf("origin = %q, want adopted", tor.Origin)
	}
	if tor.Name != "Hand Added" {
		t.Fatalf("name = %q, want %q", tor.Name, "Hand Added")
	}
	if tor.AddedAt != 1700000000 {
		t.Fatalf("added_at = %d, want the client's value 1700000000", tor.AddedAt)
	}
	links, err := st.LinksByTorrent(ctx, tor.ID)
	if err != nil {
		t.Fatalf("links: %v", err)
	}
	if len(links) != 2 {
		t.Fatalf("got %d links, want 2 (video files only)", len(links))
	}
}

func TestScanIssuesNoWritesToTheClient(t *testing.T) {
	ctx := context.Background()
	a, _, dc := newAdopter(t)
	dc.Put(&fake.Torrent{
		Hash: "aaaa", Category: "seedstrem",
		Files: []fake.File{{Name: "a.mkv", Size: 1}},
	})

	if err := a.Scan(ctx); err != nil {
		t.Fatalf("scan: %v", err)
	}

	for _, c := range dc.Calls() {
		if strings.HasPrefix(c, "Set") || strings.HasPrefix(c, "Delete") ||
			strings.HasPrefix(c, "Add") || strings.HasPrefix(c, "Start") ||
			strings.HasPrefix(c, "PrioritizePieces") {
			t.Fatalf("adoption wrote to the download client: %q (all calls: %v)", c, dc.Calls())
		}
	}
}

func TestScanSkipsKnownHash(t *testing.T) {
	ctx := context.Background()
	a, st, dc := newAdopter(t)
	if err := st.InsertTorrent(ctx, store.Torrent{
		ID: "T1", Hash: "aaaa", Name: "native", Phase: store.PhaseSelected,
		AddedAt: 1, Origin: store.OriginNative,
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	dc.Put(&fake.Torrent{Hash: "aaaa", Name: "renamed", Category: "seedstrem",
		Files: []fake.File{{Name: "a.mkv", Size: 1}}})

	if err := a.Scan(ctx); err != nil {
		t.Fatalf("scan: %v", err)
	}

	tor, err := st.TorrentByHash(ctx, "aaaa")
	if err != nil {
		t.Fatalf("by hash: %v", err)
	}
	if tor.ID != "T1" || tor.Origin != store.OriginNative || tor.Name != "native" {
		t.Fatalf("existing row was modified: %+v", tor)
	}
}

func TestScanDefersTorrentWithoutMetadata(t *testing.T) {
	ctx := context.Background()
	a, st, dc := newAdopter(t)
	dc.Put(&fake.Torrent{Hash: "aaaa", Category: "seedstrem"}) // no files yet

	if err := a.Scan(ctx); err != nil {
		t.Fatalf("scan: %v", err)
	}

	if _, err := st.TorrentByHash(ctx, "aaaa"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound (adoption must wait for metadata)", err)
	}
}

func TestScanUnadoptsWhenLabelRemoved(t *testing.T) {
	ctx := context.Background()
	a, st, dc := newAdopter(t)
	dc.Put(&fake.Torrent{Hash: "aaaa", Category: "seedstrem",
		Files: []fake.File{{Name: "a.mkv", Size: 1}}})
	if err := a.Scan(ctx); err != nil {
		t.Fatalf("first scan: %v", err)
	}

	dc.Update("aaaa", func(tr *fake.Torrent) { tr.Category = "something-else" })
	if err := a.Scan(ctx); err != nil {
		t.Fatalf("second scan: %v", err)
	}

	if _, err := st.TorrentByHash(ctx, "aaaa"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound (row should be un-adopted)", err)
	}
}

func TestScanNeverDropsNativeRows(t *testing.T) {
	ctx := context.Background()
	a, st, _ := newAdopter(t)
	if err := st.InsertTorrent(ctx, store.Torrent{
		ID: "T1", Hash: "aaaa", Phase: store.PhaseSelected, AddedAt: 1,
		Origin: store.OriginNative,
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	// The client reports no labelled torrents at all — the situation a
	// Deluge instance without the Label plugin would produce.

	if err := a.Scan(ctx); err != nil {
		t.Fatalf("scan: %v", err)
	}

	if _, err := st.TorrentByHash(ctx, "aaaa"); err != nil {
		t.Fatalf("native row was dropped: %v", err)
	}
}

func TestScanDropsNothingWhenListingFails(t *testing.T) {
	ctx := context.Background()
	a, st, dc := newAdopter(t)
	dc.Put(&fake.Torrent{Hash: "aaaa", Category: "seedstrem",
		Files: []fake.File{{Name: "a.mkv", Size: 1}}})
	if err := a.Scan(ctx); err != nil {
		t.Fatalf("first scan: %v", err)
	}

	dc.SetTorrentsByLabelErr(errors.New("connection refused"))
	if err := a.Scan(ctx); err == nil {
		t.Fatal("scan returned nil, want the listing error")
	}

	if _, err := st.TorrentByHash(ctx, "aaaa"); err != nil {
		t.Fatalf("adopted row dropped on a failed listing: %v", err)
	}
}

func TestScanNotSupportedIsANoop(t *testing.T) {
	ctx := context.Background()
	a, st, dc := newAdopter(t)
	dc.Put(&fake.Torrent{Hash: "aaaa", Category: "seedstrem",
		Files: []fake.File{{Name: "a.mkv", Size: 1}}})
	if err := a.Scan(ctx); err != nil {
		t.Fatalf("first scan: %v", err)
	}

	dc.SetTorrentsByLabelErr(downloader.ErrNotSupported)
	if err := a.Scan(ctx); err != nil {
		t.Fatalf("scan: %v, want nil (unsupported is not a failure)", err)
	}

	if _, err := st.TorrentByHash(ctx, "aaaa"); err != nil {
		t.Fatalf("adopted row dropped by an unsupported backend: %v", err)
	}
}

func TestScanDisabledDoesNothing(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	dc := fake.New()
	dc.Put(&fake.Torrent{Hash: "aaaa", Category: "seedstrem",
		Files: []fake.File{{Name: "a.mkv", Size: 1}}})
	a := adopt.New(st, dc, func() adopt.Settings {
		return adopt.Settings{Enabled: false, Label: "seedstrem"}
	}, nil, time.Minute)

	if err := a.Scan(ctx); err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(dc.Calls()) != 0 {
		t.Fatalf("disabled scan talked to the client: %v", dc.Calls())
	}
	if _, err := st.TorrentByHash(ctx, "aaaa"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}
```

- [ ] **Step 4: Run the tests to verify they fail**

Run: `go test ./internal/adopt/ -v`
Expected: FAIL — the package does not exist.

- [ ] **Step 5: Write the implementation**

Create `internal/adopt/adopt.go`:

```go
// Package adopt discovers torrents added directly in the download client
// and brings them under seedstrem's management. A torrent carrying
// seedstrem's configured label (a Deluge label, a qBittorrent category)
// is adopted: a store row and one streaming link per video file. Removing
// the label un-adopts it again.
//
// Adoption is read-only towards the download client. It never changes
// file priorities, streaming flags, or labels — the user's own selection
// stands, and the torrent is left exactly as they set it up.
package adopt

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/javib/seedstrem/internal/downloader"
	"github.com/javib/seedstrem/internal/store"
	"github.com/javib/seedstrem/internal/torrents"
)

// Settings is the live configuration slice the adopt loop needs.
type Settings struct {
	// Enabled turns label adoption on. Off by default: adopting a
	// torrent subjects it to cleanup's seed-time and ratio rules, which
	// must never happen to an existing setup by surprise.
	Enabled bool
	// Label is the download client's label/category marking a torrent as
	// seedstrem's. Empty disables adoption regardless of Enabled.
	Label string
}

// Adopter periodically reconciles the store against the set of torrents
// carrying seedstrem's label in the download client.
type Adopter struct {
	store    *store.Store
	dc       downloader.Client
	settings func() Settings
	logger   *slog.Logger
	interval time.Duration
}

// New creates an Adopter.
func New(st *store.Store, dc downloader.Client, settings func() Settings, logger *slog.Logger, interval time.Duration) *Adopter {
	if logger == nil {
		logger = slog.Default()
	}
	if interval <= 0 {
		interval = time.Minute
	}
	return &Adopter{store: st, dc: dc, settings: settings, logger: logger, interval: interval}
}

// Run loops until ctx is cancelled.
func (a *Adopter) Run(ctx context.Context) {
	ticker := time.NewTicker(a.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := a.Scan(ctx); err != nil && ctx.Err() == nil {
				a.logger.Warn("adopt scan failed", "error", err)
			}
		}
	}
}

// Scan performs one adoption pass: labelled torrents seedstrem does not
// know are adopted, and previously-adopted torrents that no longer carry
// the label are dropped.
//
// A listing failure aborts the pass before anything is deleted. "The
// client did not answer" and "the label is gone" are indistinguishable
// from the response alone, and only one of them may remove rows.
func (a *Adopter) Scan(ctx context.Context) error {
	s := a.settings()
	if !s.Enabled || s.Label == "" {
		return nil
	}

	live, err := a.dc.TorrentsByLabel(ctx, s.Label)
	if err != nil {
		if errors.Is(err, downloader.ErrNotSupported) {
			// No label support (Deluge without the Label plugin). Not a
			// failure, but nothing can be concluded either — least of
			// all that every adopted torrent lost its label.
			a.logger.Debug("adopt: download client cannot filter by label")
			return nil
		}
		return fmt.Errorf("list torrents by label: %w", err)
	}

	labelled := make(map[string]downloader.TorrentInfo, len(live))
	hashes := make([]string, 0, len(live))
	for _, info := range live {
		h := strings.ToLower(info.Hash)
		labelled[h] = info
		hashes = append(hashes, h)
	}

	known, err := a.store.TorrentsByHashes(ctx, hashes)
	if err != nil {
		return fmt.Errorf("look up known hashes: %w", err)
	}
	for hash, info := range labelled {
		if _, ok := known[hash]; ok {
			continue
		}
		if err := a.adopt(ctx, hash, info); err != nil {
			// One bad torrent must not abort the pass; the next tick
			// retries it.
			a.logger.Warn("adopt: could not adopt torrent", "hash", hash, "error", err)
		}
	}

	return a.unadopt(ctx, labelled)
}

// adopt inserts the store row and one link per video file.
func (a *Adopter) adopt(ctx context.Context, hash string, info downloader.TorrentInfo) error {
	files, err := a.dc.Files(ctx, hash)
	if err != nil {
		return fmt.Errorf("files: %w", err)
	}
	if len(files) == 0 {
		// Metadata has not resolved yet. Adopting now would mint a
		// torrent with no links and never revisit it, so wait.
		a.logger.Debug("adopt: metadata not ready, deferring", "hash", hash)
		return nil
	}

	id, err := torrents.NewID()
	if err != nil {
		return fmt.Errorf("generate id: %w", err)
	}
	addedAt := info.AddedAt.Unix()
	if info.AddedAt.IsZero() {
		addedAt = time.Now().Unix()
	}
	tor := store.Torrent{
		ID:      id,
		Hash:    hash,
		Name:    info.Name,
		Phase:   store.PhaseSelected,
		AddedAt: addedAt,
		Origin:  store.OriginAdopted,
	}
	if err := a.store.InsertTorrent(ctx, tor); err != nil {
		return fmt.Errorf("persist torrent: %w", err)
	}

	var links []store.Link
	for _, f := range files {
		if !torrents.IsVideo(f.Name) {
			continue
		}
		token, err := torrents.NewLinkToken()
		if err != nil {
			return fmt.Errorf("generate link token: %w", err)
		}
		links = append(links, store.Link{
			Token:     token,
			TorrentID: id,
			FileIndex: f.Index,
			Path:      f.Name,
			Bytes:     f.Size,
		})
	}
	if len(links) > 0 {
		if err := a.store.InsertLinks(ctx, links); err != nil {
			return fmt.Errorf("persist links: %w", err)
		}
	}

	a.logger.Info("adopt: adopted torrent from download client",
		"hash", hash, "id", id, "name", info.Name, "links", len(links))
	return nil
}

// unadopt drops adopted rows whose hash is absent from a successful
// listing. Native rows are not considered: seedstrem added them, and a
// missing label — including on a client with no label support at all —
// says nothing about whether they should exist.
func (a *Adopter) unadopt(ctx context.Context, labelled map[string]downloader.TorrentInfo) error {
	adopted, err := a.store.AdoptedTorrents(ctx)
	if err != nil {
		return fmt.Errorf("list adopted torrents: %w", err)
	}
	for _, tor := range adopted {
		if _, ok := labelled[strings.ToLower(tor.Hash)]; ok {
			continue
		}
		if err := a.store.DeleteTorrent(ctx, tor.ID); err != nil {
			a.logger.Warn("adopt: could not un-adopt torrent", "id", tor.ID, "error", err)
			continue
		}
		a.logger.Info("adopt: un-adopted torrent, label no longer set",
			"id", tor.ID, "hash", tor.Hash)
	}
	return nil
}
```

- [ ] **Step 6: Run the tests to verify they pass**

Run: `go test -race ./internal/adopt/ -v`
Expected: PASS, all nine tests.

If `TestScanIssuesNoWritesToTheClient` fails, the fake's `record` names differ from the prefixes asserted — check `dc.Calls()` output and fix the assertion to match the real names, not the other way round.

- [ ] **Step 7: Commit**

```bash
git add internal/adopt/
git commit -m "feat(adopt): adopt label-marked torrents from the download client"
```

---

### Task 4: Config and wiring

**Files:**
- Modify: `internal/config/config.go` (`Downloader` struct at :54, `Default()` at :313, env overrides near :420), `cmd/seedstrem/main.go` (after the cleanup goroutine at :153), `config.example.yaml`, `README.md`
- Test: `internal/config/config_test.go` (extend)

**Interfaces:**
- Consumes: `adopt.New`, `adopt.Settings` (Task 3).
- Produces:
  - `config.Downloader.AdoptLabelled bool` (`yaml:"adopt_labelled"`, env `SEEDSTREM_DOWNLOADER_ADOPT_LABELLED`)
  - `func (c Config) ClientLabel() string`

- [ ] **Step 1: Write the failing test**

Add to `internal/config/config_test.go`:

```go
func TestAdoptLabelledDefaultsOff(t *testing.T) {
	// Adoption subjects a hand-added torrent to cleanup's delete rules,
	// so an upgrade must never switch it on.
	if config.Default().Downloader.AdoptLabelled {
		t.Fatal("adopt_labelled defaults on, want off")
	}
}

func TestClientLabelFollowsDownloaderType(t *testing.T) {
	cfg := config.Default()

	cfg.Downloader.Type = config.DownloaderQBittorrent
	cfg.QBittorrent.Category = "qbit-cat"
	if got := cfg.ClientLabel(); got != "qbit-cat" {
		t.Fatalf("ClientLabel() = %q, want %q", got, "qbit-cat")
	}

	cfg.Downloader.Type = config.DownloaderDeluge
	cfg.Deluge.Label = "deluge-label"
	if got := cfg.ClientLabel(); got != "deluge-label" {
		t.Fatalf("ClientLabel() = %q, want %q", got, "deluge-label")
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/config/ -run 'TestAdoptLabelled|TestClientLabel' -v`
Expected: FAIL — `cfg.ClientLabel undefined`.

- [ ] **Step 3: Add the config field and helper**

In `internal/config/config.go`, extend `Downloader`:

```go
// Downloader selects which download client seedstrem drives.
type Downloader struct {
	// Type is "qbittorrent" (default) or "deluge".
	Type string `yaml:"type"`
	// AdoptLabelled brings torrents added directly in the download
	// client under seedstrem's management when they carry its
	// label/category. Off by default: an adopted torrent is swept by
	// cleanup like any other, so enabling it is a deliberate choice.
	AdoptLabelled bool `yaml:"adopt_labelled"`
}
```

Add the helper next to the `Downloader*` constants:

```go
// ClientLabel returns the label the active download client tags
// seedstrem's torrents with — a Deluge label or a qBittorrent category
// depending on Downloader.Type.
func (c Config) ClientLabel() string {
	if c.Downloader.Type == DownloaderDeluge {
		return c.Deluge.Label
	}
	return c.QBittorrent.Category
}
```

`Default()` needs no change — `false` is the zero value, and being explicit about it there would only invite someone to flip it.

- [ ] **Step 4: Add the env override**

Find how existing bool env overrides are parsed in `internal/config/config.go` (around :420, near the `SEEDSTREM_DELUGE_PORT` integer case) and follow that shape exactly:

```go
	if v := getenv("SEEDSTREM_DOWNLOADER_ADOPT_LABELLED"); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			cfg.Downloader.AdoptLabelled = b
		}
	}
```

If the file already has a bool helper alongside `set(...)`, use it instead of hand-rolling this.

- [ ] **Step 5: Run the tests to verify they pass**

Run: `go test -race ./internal/config/ -v`
Expected: PASS.

- [ ] **Step 6: Wire the loop into main**

In `cmd/seedstrem/main.go`, after the cleanup goroutine block (ends :161):

```go
	adoptCtx, stopAdopt := context.WithCancel(context.Background())
	defer stopAdopt()
	go adopt.New(db, dc, func() adopt.Settings {
		c := cm.Get()
		return adopt.Settings{
			Enabled: c.Downloader.AdoptLabelled,
			Label:   c.ClientLabel(),
		}
	}, logger, time.Minute).Run(adoptCtx)
```

Add `"github.com/javib/seedstrem/internal/adopt"` to the imports. Reading settings through `cm.Get()` on every tick is deliberate: it matches cleanup and rss, so toggling adoption in the admin UI takes effect without a restart.

- [ ] **Step 7: Document the setting**

In `config.example.yaml`, under the `downloader:` section:

```yaml
downloader:
  type: qbittorrent
  # Bring torrents you added directly in the download client under
  # seedstrem's management when they carry the label/category configured
  # below (qbittorrent.category / deluge.label). Adopted torrents are
  # streamable and are swept by the cleanup rules like any other, so
  # labelling a torrent also arms its seed-time deletion. Removing the
  # label un-adopts it. Deluge needs the Label plugin.
  adopt_labelled: false
```

Add a matching paragraph to `README.md` wherever the download-client configuration is described — including the two operational facts: cleanup applies, and removing the label un-adopts.

- [ ] **Step 8: Verify the build and full suite**

Run: `go build ./... && go test -race ./...`
Expected: PASS.

- [ ] **Step 9: Commit**

```bash
git add internal/config/ cmd/seedstrem/main.go config.example.yaml README.md
git commit -m "feat(config): add downloader.adopt_labelled and run the adopt loop"
```

---

### Task 5: Surface `origin` in the admin API and web UI

**Files:**
- Modify: `internal/admin/router.go` (`torrentItem` struct at :670-685, the item construction in `torrents` at :693), `web/src/api.ts` (`Torrent` interface at :167), `web/src/pages/Torrents.tsx` (name cell at :136)
- Test: `internal/admin/admin_test.go` (extend)

**Interfaces:**
- Consumes: `store.Torrent.Origin`, `store.OriginAdopted` (Task 1).
- Produces: `origin` field on the `/api/torrents` item JSON.

- [ ] **Step 1: Write the failing test**

Add to `internal/admin/admin_test.go`, following the existing handler-test setup in that file:

```go
func TestTorrentsExposesOrigin(t *testing.T) {
	// The UI distinguishes torrents seedstrem added from ones adopted
	// out of the download client, so origin has to reach the client.
	ctx := context.Background()
	h, st := newTestHandler(t) // existing helper in this file
	if err := st.InsertTorrent(ctx, store.Torrent{
		ID: "T1", Hash: "aaaa", Name: "adopted one", Phase: store.PhaseSelected,
		AddedAt: 1, Origin: store.OriginAdopted,
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authedRequest(t, "GET", "/api/torrents"))

	var body struct {
		Torrents []struct {
			ID     string `json:"id"`
			Origin string `json:"origin"`
		} `json:"torrents"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Torrents) != 1 {
		t.Fatalf("got %d torrents, want 1", len(body.Torrents))
	}
	if body.Torrents[0].Origin != "adopted" {
		t.Fatalf("origin = %q, want %q", body.Torrents[0].Origin, "adopted")
	}
}
```

Match the response envelope to what `/api/torrents` actually returns — read the `torrents` handler and the existing tests before writing the decode struct; if the payload is a bare array, decode into a slice instead.

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/admin/ -run TestTorrentsExposesOrigin -v`
Expected: FAIL — `origin` decodes as `""`.

- [ ] **Step 3: Add the field to the API**

In `internal/admin/router.go`, add to `torrentItem` after `AddedAt`:

```go
	// Origin is "native" (seedstrem added it) or "adopted" (discovered in
	// the download client by label).
	Origin string `json:"origin"`
```

and set it in the item construction inside `torrents`:

```go
			Origin: tor.Origin,
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test -race ./internal/admin/ -v`
Expected: PASS.

- [ ] **Step 5: Add the field to the web API type**

In `web/src/api.ts`, add to the `Torrent` interface after `indexer`:

```ts
  // "native" when seedstrem added it, "adopted" when it was picked up
  // from the download client by label.
  origin?: string;
```

- [ ] **Step 6: Render the badge**

In `web/src/pages/Torrents.tsx`, in the name cell (the block holding the `t.indexer` hint at :136), add below the indexer line:

```tsx
                        {t.origin === "adopted" && (
                          <div className="truncate text-xs opacity-60">
                            ⬇ adopted from download client
                          </div>
                        )}
```

Check the mobile card layout further down the same file and add the same line there if it also shows the indexer hint — the two views should not disagree.

- [ ] **Step 7: Verify the front end builds**

Run: `cd web && npm run build`
Expected: build succeeds with no TypeScript errors.

If the repo has a lint script, run it too (`npm run lint`).

- [ ] **Step 8: Full verification**

Run: `go build ./... && go test -race ./...`
Expected: PASS.

- [ ] **Step 9: Commit**

```bash
git add internal/admin/ web/
git commit -m "feat(web): mark adopted torrents in the torrents list"
```

---

## Verification

After Task 5, the whole feature is in place. Confirm end to end:

1. `go build ./... && go test -race ./...` — green.
2. `cd web && npm run build` — green.
3. Manual smoke test, if a client is available: set `downloader.adopt_labelled: true`, add a torrent by hand in Deluge/qBittorrent with the `seedstrem` label, wait up to a minute, and confirm it appears in the torrents list marked "adopted" with playable links. Remove the label; within a minute the row disappears while the torrent stays in the client.
