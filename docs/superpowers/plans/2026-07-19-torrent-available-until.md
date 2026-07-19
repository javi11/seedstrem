# "Available until" Column Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Show, per downloaded/seeding torrent in the admin Torrents table, the absolute local time it will be removed by seed-time cleanup.

**Architecture:** The backend `/api/torrents` handler already has the configured seed-time limit and each torrent's live qBittorrent seeding time; expose both as seconds on the JSON item. The React Torrents page computes and renders the removal deadline client-side from those two numbers, so it stays fresh against the browser clock on each 3s poll.

**Tech Stack:** Go 1.25 (`net/http`, standard `testing`), React 19 + TypeScript + Vite, daisyUI/Tailwind.

## Global Constraints

- Go formatting via `gofmt`/`goimports` is mandatory.
- No new API endpoint, no config fetch on the Torrents page, no changes to `internal/cleanup`, no changes to the Stremio placeholder.
- Times exposed over JSON are integer **seconds**.
- The web package has **no** working test runner (`vitest` is scripted but not installed, zero existing web tests). Do not add one. Verify the frontend with `npm run build` (which runs `tsc -b`).

---

### Task 1: Backend — expose `seed_time` and `seeding_time`

**Files:**
- Modify: `internal/admin/router.go` (`torrentItem` struct ~362-377; population block ~413-427)
- Modify: `internal/admin/admin_test.go` (`env` struct ~21-27; `newEnv` ~29-47; add test after `TestTorrentsListing` ~337)
- Test: `internal/admin/admin_test.go`

**Interfaces:**
- Consumes: `store.Store.InsertTorrent(ctx, store.Torrent)`, `fake.Server.Put(*fake.Torrent)`, both already existing; `cfg.Cleanup.SeedTime` (`time.Duration`), `info.SeedingTime` (`time.Duration`).
- Produces: two new JSON fields on each `/api/torrents` array item — `seed_time` (int64 seconds) and `seeding_time` (int64 seconds). The React `Torrent` type in Task 2 relies on these exact names.

- [ ] **Step 1: Expose the store on the test env**

The new test needs to insert a torrent row. Add a `store` field to `env` and populate it in `newEnv`.

In `internal/admin/admin_test.go`, change the `env` struct:

```go
type env struct {
	handler http.Handler
	config  *config.Manager
	fake    *fake.Server
	store   *store.Store
	cookie  *http.Cookie
	t       *testing.T
}
```

And the return in `newEnv` (currently `return &env{handler: h.Router(), config: cm, fake: f, t: t}`):

```go
	return &env{handler: h.Router(), config: cm, fake: f, store: st, t: t}
```

- [ ] **Step 2: Write the failing test**

Add to `internal/admin/admin_test.go` (after `TestTorrentsListing`):

```go
func TestTorrentsListingExposesSeedTimes(t *testing.T) {
	e := newEnv(t)
	e.login(t)

	const hash = "aa11bb22cc33dd44"
	if err := e.store.InsertTorrent(context.Background(), store.Torrent{
		ID: "tt1", Hash: hash, Name: "Example", Phase: store.PhaseSelected, AddedAt: 1,
	}); err != nil {
		t.Fatal(err)
	}
	// Completed torrent that has been seeding for 2h; default SeedTime is 72h.
	e.fake.Put(&fake.Torrent{Hash: hash, Progress: 1, SeedingTime: 2 * time.Hour})

	w := e.do(t, http.MethodGet, "/torrents", "")
	if w.Code != http.StatusOK {
		t.Fatalf("torrents = %d", w.Code)
	}
	var items []map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &items); err != nil {
		t.Fatalf("decode: %v body=%s", err, w.Body.String())
	}
	if len(items) != 1 {
		t.Fatalf("want 1 item, got %d: %v", len(items), items)
	}
	if got := items[0]["seed_time"]; got != float64(72*3600) {
		t.Errorf("seed_time = %v, want %d", got, 72*3600)
	}
	if got := items[0]["seeding_time"]; got != float64(2*3600) {
		t.Errorf("seeding_time = %v, want %d", got, 2*3600)
	}
}
```

Add `"context"` to the import block of `internal/admin/admin_test.go` (currently absent).

- [ ] **Step 3: Run the test to verify it fails**

Run: `go test ./internal/admin/ -run TestTorrentsListingExposesSeedTimes -v`
Expected: FAIL — `seed_time = <nil>, want 259200` (fields not yet serialized).

- [ ] **Step 4: Add the struct fields and populate them**

In `internal/admin/router.go`, add two fields to `torrentItem` (place them right after `Ratio`):

```go
type torrentItem struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	Hash        string     `json:"hash"`
	Status      string     `json:"status"`
	Progress    float64    `json:"progress"`
	Speed       int64      `json:"speed"`
	Seeders     int64      `json:"seeders"`
	Size        int64      `json:"size"`
	Uploaded    int64      `json:"uploaded"`
	Ratio       float64    `json:"ratio"`
	SeedTime    int64      `json:"seed_time"`
	SeedingTime int64      `json:"seeding_time"`
	AddedAt     int64      `json:"added_at"`
	Error       string     `json:"error,omitempty"`
	Links       []linkItem `json:"links"`
}
```

In the population block inside `torrents()`, add the two fields (right after `Ratio: info.Ratio,`):

```go
			Ratio:       info.Ratio,
			SeedTime:    int64(cfg.Cleanup.SeedTime / time.Second),
			SeedingTime: int64(info.SeedingTime / time.Second),
			AddedAt:     tor.AddedAt,
```

(`cfg` and `time` are already in scope in this function.)

- [ ] **Step 5: Run the test to verify it passes**

Run: `go test ./internal/admin/ -run TestTorrentsListingExposesSeedTimes -v`
Expected: PASS.

- [ ] **Step 6: Run gofmt and the package tests**

Run: `gofmt -w internal/admin/router.go internal/admin/admin_test.go && go test ./internal/admin/`
Expected: PASS, no diff from gofmt.

- [ ] **Step 7: Commit**

```bash
git add internal/admin/router.go internal/admin/admin_test.go
git commit -m "feat(admin): expose seed_time and seeding_time on torrents API"
```

---

### Task 2: Frontend — "Available until" column

**Files:**
- Modify: `web/src/api.ts` (`Torrent` interface ~112-127)
- Modify: `web/src/pages/Torrents.tsx` (imports, add `availableUntil` helper, table header, cell, `colSpan`)
- Test: none (no runner; verified via `npm run build`)

**Interfaces:**
- Consumes: `Torrent.seed_time` and `Torrent.seeding_time` (numbers, seconds) from Task 1.
- Produces: exported pure function `availableUntil(t: Torrent): string` for future unit testing.

- [ ] **Step 1: Add the fields to the `Torrent` type**

In `web/src/api.ts`, in the `Torrent` interface, add the two fields right after `ratio: number;`:

```ts
  ratio: number;
  seed_time: number;
  seeding_time: number;
  added_at: number;
```

(`added_at: number;` already exists — do not duplicate it; add only `seed_time` and `seeding_time`.)

- [ ] **Step 2: Add the `availableUntil` helper**

In `web/src/pages/Torrents.tsx`, add this exported function at module scope (e.g. directly below the `BADGE` constant):

```ts
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
```

- [ ] **Step 3: Add the table header**

In the `<thead>` row, add a header after the `Ratio` one:

```tsx
            <th>Uploaded</th>
            <th>Ratio</th>
            <th>Available until</th>
```

- [ ] **Step 4: Add the cell**

In the `<tbody>` row, add a cell after the ratio `<td>`:

```tsx
                <td>
                  <span className={t.ratio >= 1 ? "text-success" : ""}>{t.ratio.toFixed(2)}</span>
                </td>
                <td className="whitespace-nowrap">{availableUntil(t)}</td>
```

- [ ] **Step 5: Widen the expanded-row colSpan**

The expanded details row spans all columns. There are now 9 columns, so change `colSpan={8}` to `colSpan={9}`:

```tsx
                  <td colSpan={9} className="bg-base-200">
```

- [ ] **Step 6: Type-check / build the web app**

Run: `cd web && npm run build`
Expected: build succeeds (`tsc -b` reports no type errors, vite emits `dist/`).

- [ ] **Step 7: Commit**

```bash
git add web/src/api.ts web/src/pages/Torrents.tsx web/dist
git commit -m "feat(web): show 'available until' deadline in torrents table"
```

(If `web/dist` is git-ignored, drop it from the `git add`; commit only the two source files.)

---

## Notes for the implementer

- **Branch matrix for `availableUntil`:** not-seeding (`progress < 1`) → `—`; cleanup disabled (`seed_time <= 0`) → `Kept`; past limit (`remaining <= 0`) → `Removing soon`; otherwise absolute local time like `Jul 22, 14:00`.
- A torrent that fell out of qBittorrent reports `progress === 0`, so it correctly lands in the `—` branch.
- The deadline is recomputed from `Date.now()` on every render; the page already polls every 3s, so it stays current without extra work.
