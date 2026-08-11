# Download-Client Disk Space Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Show seedstrem's on-disk footprint and the download client's remaining free space as two tiles on the web UI Dashboard.

**Architecture:** The `downloader.Client` interface gains a `FreeSpace` method that both backends implement from their native APIs (qBittorrent `sync/maindata`, Deluge `core.get_free_space`). `TorrentInfo` gains a `Completed` field so seedstrem's footprint is summed from real completed-byte counts rather than estimated. `GET /api/status` grows a `disk` object — `used` from the torrents it already joins, `free` from the client with a local `statfs` fallback — which the Dashboard renders as two `StatCard`s.

**Tech Stack:** Go 1.x (chi router, `golang.org/x/sys`-free `syscall.Statfs` via `internal/diskusage`), React 19 + TypeScript + Tailwind 4/daisyUI 5 (Vite), `github.com/autobrr/go-qbittorrent` v1.16.0, vendored `internal/deluge/delugerpc`.

**Spec:** `docs/superpowers/specs/2026-08-11-client-disk-space-design.md`

## Global Constraints

- Backend emits raw `int64` byte counts; **all** byte formatting happens in the frontend via the existing `formatBytes`. The Go side has no byte formatter and must not gain one.
- JSON response keys are `snake_case`, matching every existing key in `/api/status`.
- `web/src/api.ts` types are hand-written — there is no codegen. Any Go response-shape change must be mirrored there manually in the same task.
- Every method added to `downloader.Client` MUST also be added to `Swappable` (`internal/downloader/swap.go`) and to the fake (`internal/downloader/fake/fake.go`), or the build breaks. `Swappable` forwards all interface methods one-for-one.
- `/api/status` must never fail because disk figures are unavailable — it still has to serve version and connectivity.
- `used + free` deliberately does **not** equal disk total, and no percentage is computed. Neither client API exposes a disk total. Do not "fix" this by reaching for `statfs` totals on the happy path.
- Run `gofmt -w .` before every commit; the repo is gofmt-clean.
- Test command is `go test -race -cover ./...` (`make test`). Frontend has **no** test suite and this plan does not add one.

---

### Task 1: `TorrentInfo.Completed` through both backends and the fake

Adds the completed-bytes field seedstrem's footprint is summed from. Today `TorrentInfo` carries only `Size` (wanted) and `Progress` (0..1), so a footprint would be a `Size × Progress` estimate; both clients report the real number.

**Files:**
- Modify: `internal/downloader/types.go:6-29` (the `TorrentInfo` struct)
- Modify: `internal/qbit/client.go:97-115` (`convertTorrent`)
- Modify: `internal/deluge/convert.go:41-63` (`convertTorrent`)
- Modify: `internal/downloader/fake/fake.go:31-49` (`Torrent` struct) and `:213-230` (`toTorrentInfo`)
- Test: `internal/qbit/client_test.go:34-59` (extend `TestConvertTorrent`)
- Test: `internal/deluge/convert_test.go:37+` (extend `TestConvertTorrent`)

**Interfaces:**
- Consumes: nothing (first task).
- Produces: `downloader.TorrentInfo.Completed int64` — bytes of wanted data already on disk. Read by Task 4. Also `fake.Torrent.Completed int64`, the field tests set to drive it.

- [ ] **Step 1: Write the failing tests**

In `internal/qbit/client_test.go`, add `Completed: 420` to the `qbt.Torrent` literal inside `TestConvertTorrent` (it currently sets `Hash`, `Name`, `State`, `Progress`, `Size`, `DlSpeed`, `NumSeeds`, `SavePath`, `SeedingTime`), then add this assertion at the end of the function:

```go
	if info.Completed != 420 {
		t.Errorf("completed = %d, want 420", info.Completed)
	}
```

In `internal/deluge/convert_test.go`, add `TotalDone: 420` to the `&delugerpc.TorrentStatus{...}` literal inside `TestConvertTorrent`, then add this assertion after the existing ones in that function:

```go
	if info.Completed != 420 {
		t.Errorf("completed = %d, want 420 (from TotalDone)", info.Completed)
	}
```

- [ ] **Step 2: Run the tests to verify they fail**

```bash
go test ./internal/qbit/ ./internal/deluge/ -run TestConvertTorrent -v
```

Expected: FAIL to **compile**, with `unknown field Completed in struct literal of type downloader.TorrentInfo`-style errors (`info.Completed` undefined). A compile failure is the correct RED here — the field does not exist yet.

- [ ] **Step 3: Add the field to `TorrentInfo`**

In `internal/downloader/types.go`, inside `TorrentInfo`, add `Completed` directly below `Size` so the two size-ish fields sit together:

```go
	Size     int64 // wanted (selected) size
	// Completed is the bytes of wanted data already on disk. Together
	// across torrents it is seedstrem's on-disk footprint, which the
	// admin status endpoint reports.
	Completed int64
	DlSpeed   int64
```

- [ ] **Step 4: Map it in the qBittorrent backend**

In `internal/qbit/client.go`, in `convertTorrent`, add the field below `Size`:

```go
		Size:        t.Size,
		Completed:   t.Completed,
		DlSpeed:     t.DlSpeed,
```

`qbt.Torrent.Completed int64` already exists (`go-qbittorrent@v1.16.0/domain.go:64`), populated from the `completed` JSON field.

- [ ] **Step 5: Map it in the Deluge backend**

In `internal/deluge/convert.go`, in `convertTorrent`, add the field below `Size`:

```go
		Size:      wantedSize(ts),
		Completed: ts.TotalDone,
		DlSpeed:   ts.DownloadPayloadRate,
```

`delugerpc.TorrentStatus.TotalDone int64` already exists (`internal/deluge/delugerpc/torrent_status.go:53`).

- [ ] **Step 6: Map it in the fake**

In `internal/downloader/fake/fake.go`, add to the `Torrent` struct below `Progress`:

```go
	Progress    float64
	Completed   int64
	DlSpeed     int64
```

and in `toTorrentInfo` add the corresponding line below `Progress`:

```go
		Progress:    t.Progress,
		Completed:   t.Completed,
		DlSpeed:     t.DlSpeed,
```

Note: `toTorrentInfo` does not map `Size` today. Leave that alone — it is pre-existing and out of scope.

- [ ] **Step 7: Run the tests to verify they pass**

```bash
gofmt -w . && go test -race ./internal/qbit/ ./internal/deluge/ ./internal/downloader/... -v -run TestConvertTorrent
```

Expected: PASS for both `TestConvertTorrent` functions.

- [ ] **Step 8: Verify nothing else broke**

```bash
go build ./... && go test -race ./...
```

Expected: build succeeds, all tests pass. Adding a struct field breaks no existing call site.

- [ ] **Step 9: Commit**

```bash
git add internal/downloader/types.go internal/qbit/client.go internal/qbit/client_test.go internal/deluge/convert.go internal/deluge/convert_test.go internal/downloader/fake/fake.go
git commit -m "feat(downloader): report completed bytes per torrent"
```

---

### Task 2: `FreeSpace` on the downloader interface and both backends

**Files:**
- Modify: `internal/downloader/downloader.go:36-70` (the `Client` interface)
- Modify: `internal/downloader/swap.go:94+` (append the forwarder)
- Modify: `internal/qbit/client.go` (append a method near `Version`, `:296-302`)
- Modify: `internal/deluge/client.go:22-38` (the `api` interface) and append a method near `Version`, `:468-479`
- Modify: `internal/downloader/fake/fake.go` (fields, setters, method)
- Test: `internal/deluge/client_test.go:16-30` (`fakeAPI` gains the method) and a new test function
- Test: `internal/downloader/fake/` — no test file exists; the fake is exercised through consumers

**Interfaces:**
- Consumes: nothing from Task 1.
- Produces: `downloader.Client.FreeSpace(ctx context.Context) (int64, error)` — bytes available on the client's download filesystem, `downloader.ErrNotSupported` when the backend has no such concept. Called by Task 4. Also `fake.Server.SetFreeSpace(n int64)` and `fake.Server.SetFreeSpaceErr(err error)`, used by Task 4's tests.

**Note on qBittorrent test coverage:** `internal/qbit` has no transport-level tests — its test file covers only pure functions (`convertTorrent`, `normalizeState`, `addOptionsMap`, `isNotFound`). Standing up an httptest server implementing `/api/v2/auth/login` + `/api/v2/sync/maindata` to cover a three-line delegation would break that convention for little gain. The qBittorrent path is verified by compilation plus Task 4's handler tests against the fake. This is a deliberate, stated gap, not an oversight.

- [ ] **Step 1: Write the failing test**

Create the Deluge test. Append to `internal/deluge/client_test.go`:

```go
func TestFreeSpace(t *testing.T) {
	f := newFakeAPI()
	f.freeSpace = 4096
	c := &client{rpc: f, flagCache: map[string]flags{}, now: time.Now}

	got, err := c.FreeSpace(context.Background())
	if err != nil {
		t.Fatalf("FreeSpace: %v", err)
	}
	if got != 4096 {
		t.Errorf("free space = %d, want 4096", got)
	}
	// The default download location is requested, i.e. an empty path.
	if !slices.Contains(f.calls, "getFreeSpace path=") {
		t.Errorf("calls = %v, want a getFreeSpace with an empty path", f.calls)
	}
}

func TestFreeSpaceError(t *testing.T) {
	f := newFakeAPI()
	f.freeSpaceErr = errors.New("boom")
	c := &client{rpc: f, flagCache: map[string]flags{}, now: time.Now}

	if _, err := c.FreeSpace(context.Background()); err == nil {
		t.Fatal("FreeSpace: want error, got nil")
	}
}
```

Check `internal/deluge/client_test.go`'s import block and add whichever of `context`, `errors`, `slices`, `testing`, `time` are missing.

- [ ] **Step 2: Run the test to verify it fails**

```bash
go test ./internal/deluge/ -run TestFreeSpace -v
```

Expected: FAIL to compile — `f.freeSpace undefined` and `c.FreeSpace undefined`.

- [ ] **Step 3: Add the method to the interface**

In `internal/downloader/downloader.go`, inside the `Client` interface, add above `Version`:

```go
	// FreeSpace reports the bytes still available on the download
	// client's own filesystem. It is the client's view, not seedstrem's:
	// the two differ whenever the client runs on another host. Backends
	// without the concept return ErrNotSupported. Neither supported
	// backend can report the filesystem *total*, so callers get headroom
	// only — not a percentage.
	FreeSpace(ctx context.Context) (int64, error)

	Version(ctx context.Context) (string, error)
```

- [ ] **Step 4: Forward it through `Swappable`**

In `internal/downloader/swap.go`, add above the `Version` forwarder:

```go
func (s *Swappable) FreeSpace(ctx context.Context) (int64, error) {
	return s.get().FreeSpace(ctx)
}
```

- [ ] **Step 5: Implement it for qBittorrent**

In `internal/qbit/client.go`, add above `func (c *client) Version`:

```go
// FreeSpace reads qBittorrent's own free-space figure out of the sync
// endpoint's server state. rid 0 asks for a full update, which is the
// only variant that reliably carries server_state.
func (c *client) FreeSpace(ctx context.Context) (int64, error) {
	main, err := c.qb.SyncMainDataCtx(ctx, 0)
	if err != nil {
		return 0, fmt.Errorf("qbit sync maindata: %w", err)
	}
	return main.ServerState.FreeSpaceOnDisk, nil
}
```

`fmt` is already imported. Library reference: `SyncMainDataCtx(ctx, rid int64) (*MainData, error)` and `MainData.ServerState.FreeSpaceOnDisk int64`.

- [ ] **Step 6: Implement it for Deluge**

In `internal/deluge/client.go`, add to the `api` interface, next to `DaemonVersion`:

```go
	DaemonVersion(ctx context.Context) (string, error)
	GetFreeSpace(ctx context.Context, path string) (int64, error)
```

`*delugerpc.ClientV2` already satisfies this — `GetFreeSpace` is declared on the vendored `DelugeClient` interface (`internal/deluge/delugerpc/delugeclient.go:71`) and implemented at `internal/deluge/delugerpc/methods.go:25`.

Then add above `func (c *client) Version`:

```go
// FreeSpace asks the daemon for free space at its default download
// location; core.get_free_space treats an empty path as "the default".
func (c *client) FreeSpace(ctx context.Context) (int64, error) {
	var free int64
	err := c.do(ctx, func(ctx context.Context) error {
		n, err := c.rpc.GetFreeSpace(ctx, "")
		if err != nil {
			return fmt.Errorf("deluge free space: %w", err)
		}
		free = n
		return nil
	})
	return free, err
}
```

- [ ] **Step 7: Add the method to the Deluge test fake**

In `internal/deluge/client_test.go`, add two fields to `fakeAPI`:

```go
	setFilePriorities map[string][]int
	torrentOptions    []string

	freeSpace    int64
	freeSpaceErr error
```

and add the method next to `DaemonVersion`:

```go
func (f *fakeAPI) GetFreeSpace(_ context.Context, path string) (int64, error) {
	f.record("getFreeSpace path=%s", path)
	return f.freeSpace, f.freeSpaceErr
}
```

- [ ] **Step 8: Implement it in the fake client**

In `internal/downloader/fake/fake.go`, add to the `Server` struct:

```go
	// prioritizeErr is returned by PrioritizePieces; defaults to
	// downloader.ErrNotSupported like the qBittorrent backend.
	prioritizeErr error
	// freeSpace/freeSpaceErr back FreeSpace. The error defaults to
	// ErrNotSupported so tests opt in to a capable backend explicitly.
	freeSpace    int64
	freeSpaceErr error
```

In `New`, set the default:

```go
func New() *Server {
	return &Server{
		torrents:      map[string]*Torrent{},
		prioritizeErr: downloader.ErrNotSupported,
		freeSpaceErr:  downloader.ErrNotSupported,
	}
}
```

Add the setters next to `SetPrioritizeErr`:

```go
// SetFreeSpace makes FreeSpace report n bytes and clears any error.
func (s *Server) SetFreeSpace(n int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.freeSpace = n
	s.freeSpaceErr = nil
}

// SetFreeSpaceErr sets the error returned by FreeSpace.
func (s *Server) SetFreeSpaceErr(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.freeSpaceErr = err
}
```

Add the method next to `Version`:

```go
func (s *Server) FreeSpace(context.Context) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.freeSpace, s.freeSpaceErr
}
```

- [ ] **Step 9: Run the tests to verify they pass**

```bash
gofmt -w . && go test -race ./internal/deluge/ -run TestFreeSpace -v
```

Expected: PASS for `TestFreeSpace` and `TestFreeSpaceError`.

- [ ] **Step 10: Verify the whole tree compiles and passes**

```bash
go build ./... && go vet ./... && go test -race ./...
```

Expected: all green. If a compile error names a type that does not implement `FreeSpace`, that type is a `downloader.Client` implementation the plan missed — add the method there rather than removing it from the interface.

- [ ] **Step 11: Commit**

```bash
git add internal/downloader/downloader.go internal/downloader/swap.go internal/downloader/fake/fake.go internal/qbit/client.go internal/deluge/client.go internal/deluge/client_test.go
git commit -m "feat(downloader): report client free space"
```

---

### Task 3: `config.Paths.FirstLocal()` helper

The admin handler needs the downloads path for its `statfs` fallback. `cmd/seedstrem/main.go` already has an unexported `firstLocalMapping` for exactly this, used three times. Moving the logic onto `config.Paths` lets the admin handler reach it from the `*config.Manager` it already holds, with no new constructor parameter and no copy-pasted loop.

**Files:**
- Modify: `internal/config/config.go:132-134` (the `Paths` struct — append a method)
- Modify: `cmd/seedstrem/main.go:257-267` (`firstLocalMapping` delegates)
- Test: `internal/config/paths_test.go` (create)

**Interfaces:**
- Consumes: nothing.
- Produces: `func (p Paths) FirstLocal() string` — the first non-empty `Mappings[].Local`, `""` when none. Called by Task 4.

- [ ] **Step 1: Write the failing test**

Create `internal/config/paths_test.go`:

```go
package config

import "testing"

func TestPathsFirstLocal(t *testing.T) {
	tests := []struct {
		name string
		in   Paths
		want string
	}{
		{"no mappings", Paths{}, ""},
		{
			"single mapping",
			Paths{Mappings: []Mapping{{Remote: "/downloads", Local: "/data"}}},
			"/data",
		},
		{
			"first non-empty local wins",
			Paths{Mappings: []Mapping{
				{Remote: "/downloads", Local: ""},
				{Remote: "/media", Local: "/data2"},
				{Remote: "/other", Local: "/data3"},
			}},
			"/data2",
		},
		{
			"all locals empty",
			Paths{Mappings: []Mapping{{Remote: "/downloads"}, {Remote: "/media"}}},
			"",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.in.FirstLocal(); got != tt.want {
				t.Errorf("FirstLocal() = %q, want %q", got, tt.want)
			}
		})
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

```bash
go test ./internal/config/ -run TestPathsFirstLocal -v
```

Expected: FAIL to compile — `tt.in.FirstLocal undefined (type Paths has no field or method FirstLocal)`.

- [ ] **Step 3: Add the method**

In `internal/config/config.go`, directly below the `Paths` struct:

```go
// FirstLocal returns the first configured local download root — the path
// whose filesystem usage gates streams and RSS grabs, and whose free
// space the admin API falls back to when the download client cannot
// report its own. Empty when no mapping is configured.
func (p Paths) FirstLocal() string {
	for _, m := range p.Mappings {
		if m.Local != "" {
			return m.Local
		}
	}
	return ""
}
```

- [ ] **Step 4: Run the test to verify it passes**

```bash
go test -race ./internal/config/ -run TestPathsFirstLocal -v
```

Expected: PASS, all four subtests.

- [ ] **Step 5: Delegate from main's helper**

In `cmd/seedstrem/main.go`, replace the body of `firstLocalMapping` (keeping the function so its three call sites at `:128`, `:182` and elsewhere stay untouched):

```go
// firstLocalMapping returns the first configured local download root, used
// as the path whose disk usage gates new streams. Empty when no mapping is
// configured, which disables the gate.
func firstLocalMapping(mappings []config.Mapping) string {
	return config.Paths{Mappings: mappings}.FirstLocal()
}
```

- [ ] **Step 6: Verify the tree still builds and passes**

```bash
gofmt -w . && go build ./... && go test -race ./...
```

Expected: all green.

- [ ] **Step 7: Commit**

```bash
git add internal/config/config.go internal/config/paths_test.go cmd/seedstrem/main.go
git commit -m "refactor(config): expose first local download root on Paths"
```

---

### Task 4: `disk` object on `GET /api/status`

**Files:**
- Modify: `internal/admin/router.go:29-49` (`Handler` struct + `New`), `:515-555` (`status`)
- Modify: `internal/admin/admin_test.go:23-57` (`env` exposes the `*Handler` so tests can inject `diskUsage`)
- Test: `internal/admin/disk_test.go` (create)

**Interfaces:**
- Consumes: `downloader.TorrentInfo.Completed` (Task 1), `downloader.Client.FreeSpace` + `fake.Server.SetFreeSpace`/`SetFreeSpaceErr` (Task 2), `config.Paths.FirstLocal()` (Task 3).
- Produces: the `disk` key of the `/api/status` JSON object — `{"used": int64, "free": int64 (optional), "free_source": "client"|"local" (optional)}`. Consumed by Task 5.

- [ ] **Step 1: Expose the handler on the test env**

This is test scaffolding the failing test needs, so it comes first. In `internal/admin/admin_test.go`, add a field to `env`:

```go
type env struct {
	handler   http.Handler
	h         *Handler
	config    *config.Manager
	fake      *fake.Server
	swappable *downloader.Swappable
	store     *store.Store
	cookie    *http.Cookie
	t         *testing.T
}
```

and populate it in `newEnv`'s return (the `h := New(...)` line already exists directly above):

```go
	h := New(cm, st, dc, svc, newClient, "test", nil)
	return &env{handler: h.Router(), h: h, config: cm, fake: f, swappable: dc, store: st, t: t}
```

- [ ] **Step 2: Write the failing test**

Create `internal/admin/disk_test.go`:

```go
package admin

import (
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	"github.com/javib/seedstrem/internal/config"
	"github.com/javib/seedstrem/internal/downloader"
	"github.com/javib/seedstrem/internal/downloader/fake"
)

// diskOf logs in, fetches /status, and returns its "disk" object.
func diskOf(t *testing.T, e *env) map[string]any {
	t.Helper()
	w := e.do(t, http.MethodGet, "/status", "")
	if w.Code != http.StatusOK {
		t.Fatalf("status: got %d %s, want 200", w.Code, w.Body.String())
	}
	var body struct {
		Disk map[string]any `json:"disk"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode status: %v", err)
	}
	if body.Disk == nil {
		t.Fatal("status response has no disk object")
	}
	return body.Disk
}

// seedCompleted puts a torrent in the store and the fake client with the
// given completed byte count, so the handler can sum it.
func seedCompleted(t *testing.T, e *env, id, hash string, completed int64) {
	t.Helper()
	seedTorrent(t, e, id, hash)
	if !e.fake.Update(hash, func(tor *fake.Torrent) { tor.Completed = completed }) {
		t.Fatalf("fake has no torrent %s", hash)
	}
}

func TestStatusDiskUsedSumsCompleted(t *testing.T) {
	e := newEnv(t)
	e.login(t)
	seedCompleted(t, e, "id-1", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", 100)
	seedCompleted(t, e, "id-2", "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", 250)

	if got := diskOf(t, e)["used"]; got != float64(350) {
		t.Errorf("used = %v, want 350", got)
	}
}

func TestStatusDiskFreeFromClient(t *testing.T) {
	e := newEnv(t)
	e.login(t)
	e.fake.SetFreeSpace(8192)
	// A statfs fallback would be wrong here; make it loud if it is used.
	e.h.diskUsage = func(string) (int64, int64, error) {
		t.Error("diskUsage called even though the client answered")
		return 0, 0, nil
	}

	disk := diskOf(t, e)
	if disk["free"] != float64(8192) {
		t.Errorf("free = %v, want 8192", disk["free"])
	}
	if disk["free_source"] != "client" {
		t.Errorf("free_source = %v, want client", disk["free_source"])
	}
}

func TestStatusDiskFallsBackToLocalStatfs(t *testing.T) {
	e := newEnv(t)
	e.login(t)
	// Default fake behaviour is ErrNotSupported.
	e.fake.SetFreeSpaceErr(downloader.ErrNotSupported)
	cfg := e.config.Get()
	cfg.Paths.Mappings = []config.Mapping{{Remote: "/downloads", Local: "/data"}}
	if err := e.config.Update(cfg); err != nil {
		t.Fatalf("update config: %v", err)
	}
	e.h.diskUsage = func(path string) (int64, int64, error) {
		if path != "/data" {
			t.Errorf("diskUsage path = %q, want /data", path)
		}
		return 300, 1000, nil
	}

	disk := diskOf(t, e)
	if disk["free"] != float64(700) {
		t.Errorf("free = %v, want 700 (total 1000 - used 300)", disk["free"])
	}
	if disk["free_source"] != "local" {
		t.Errorf("free_source = %v, want local", disk["free_source"])
	}
}

func TestStatusDiskOmitsFreeWhenBothFail(t *testing.T) {
	e := newEnv(t)
	e.login(t)
	seedCompleted(t, e, "id-1", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", 100)
	e.fake.SetFreeSpaceErr(errors.New("client unreachable"))
	cfg := e.config.Get()
	cfg.Paths.Mappings = []config.Mapping{{Remote: "/downloads", Local: "/data"}}
	if err := e.config.Update(cfg); err != nil {
		t.Fatalf("update config: %v", err)
	}
	e.h.diskUsage = func(string) (int64, int64, error) {
		return 0, 0, errors.New("statfs failed")
	}

	disk := diskOf(t, e)
	if _, ok := disk["free"]; ok {
		t.Errorf("free present (%v), want omitted when both sources fail", disk["free"])
	}
	if _, ok := disk["free_source"]; ok {
		t.Errorf("free_source present (%v), want omitted", disk["free_source"])
	}
	// used still reports: it comes from data the handler already holds.
	if disk["used"] != float64(100) {
		t.Errorf("used = %v, want 100", disk["used"])
	}
}

func TestStatusDiskOmitsFreeWhenNoMappingConfigured(t *testing.T) {
	e := newEnv(t)
	e.login(t)
	e.fake.SetFreeSpaceErr(downloader.ErrNotSupported)
	e.h.diskUsage = func(string) (int64, int64, error) {
		t.Error("diskUsage called with no mapping configured")
		return 0, 0, nil
	}

	if _, ok := diskOf(t, e)["free"]; ok {
		t.Error("free present, want omitted with no configured download path")
	}
}
```

Note on JSON numbers: decoding into `map[string]any` yields `float64`, hence the `float64(...)` comparisons.

`config.Manager` exposes exactly two methods — `Get() Config` and
`Update(cfg Config) error` (`internal/config/manager.go:21,28`) — so
`Update` is the setter, and it persists to the manager's config path,
which `newEnv` already points at a `t.TempDir()`.

- [ ] **Step 3: Run the tests to verify they fail**

```bash
go test ./internal/admin/ -run TestStatusDisk -v
```

Expected: FAIL to compile — `e.h.diskUsage undefined`. Once that resolves, the remaining failure is `status response has no disk object`.

- [ ] **Step 4: Add the injectable `diskUsage` to the handler**

In `internal/admin/router.go`, add the import:

```go
	"github.com/javib/seedstrem/internal/diskusage"
```

Add the field to `Handler`:

```go
	logger    *slog.Logger
	version   string

	// diskUsage reports (used, total) bytes for a local path. Injectable
	// for tests; defaults to diskusage.Stat. Used only as the fallback
	// when the download client cannot report its own free space.
	diskUsage func(path string) (used, total int64, err error)
```

and default it in `New`:

```go
	return &Handler{config: cm, store: st, dc: dc, svc: svc, newClient: newClient, logger: logger, version: version, diskUsage: diskusage.Stat}
```

- [ ] **Step 5: Add the `diskInfo` helper**

In `internal/admin/router.go`, add directly above `func (h *Handler) status`:

```go
// diskInfo reports seedstrem's on-disk footprint plus the space still
// available for downloads. free comes from the download client when it can
// answer — that is the filesystem the client actually writes to, which is
// not necessarily one seedstrem can stat — and falls back to a local
// statfs on the configured download root otherwise. When neither source
// answers, free is omitted rather than guessed; used is still reported,
// since it comes from data already in hand.
//
// used + free is deliberately not a disk total: used is seedstrem's
// footprint, free is the whole filesystem's headroom. Neither backend can
// report a total, so no percentage is possible.
func (h *Handler) diskInfo(ctx context.Context, cfg config.Config, used int64) map[string]any {
	disk := map[string]any{"used": used}

	if free, err := h.dc.FreeSpace(ctx); err == nil {
		disk["free"] = free
		disk["free_source"] = "client"
		return disk
	}

	path := cfg.Paths.FirstLocal()
	if path == "" {
		return disk
	}
	localUsed, total, err := h.diskUsage(path)
	if err != nil {
		return disk
	}
	disk["free"] = total - localUsed
	disk["free_source"] = "local"
	return disk
}
```

- [ ] **Step 6: Sum completed bytes and emit the object**

In `internal/admin/router.go`, in `status`, widen the accumulator declaration and add the sum:

```go
	counts := map[string]int{}
	var totalUploaded, diskUsed int64
	if stored, err := h.store.AllTorrents(ctx); err == nil {
		live := h.liveByHash(ctx, stored)
		for _, tor := range stored {
			info, inQbit := live[tor.Hash]
			status := torrents.DeriveStatus(tor.Phase, info.State, tor.Error != "" || !inQbit, info.Size > 0, info.Progress)
			counts[status]++
			totalUploaded += info.Uploaded
			diskUsed += info.Completed
		}
	}
```

and add the key to the response map, after `total_uploaded`:

```go
		"torrents":       counts,
		"total_uploaded": totalUploaded,
		"disk":           h.diskInfo(ctx, cfg, diskUsed),
	})
```

- [ ] **Step 7: Run the tests to verify they pass**

```bash
gofmt -w . && go test -race ./internal/admin/ -run TestStatusDisk -v
```

Expected: PASS for all five `TestStatusDisk*` functions.

- [ ] **Step 8: Verify the whole suite**

```bash
go vet ./... && go test -race -cover ./...
```

Expected: all green. Existing `/status` tests keep passing — the change is additive.

- [ ] **Step 9: Commit**

```bash
git add internal/admin/router.go internal/admin/admin_test.go internal/admin/disk_test.go
git commit -m "feat(admin): report download disk used and free on /api/status"
```

---

### Task 5: Dashboard tiles

**Files:**
- Modify: `web/src/api.ts:133-141` (the `Status` interface)
- Modify: `web/src/pages/Dashboard.tsx` (derive a torrent count; two `StatCard`s after the `Uploaded` tile)

**Interfaces:**
- Consumes: the `disk` object from Task 4.
- Produces: the finished UI. Nothing downstream.

**Note:** `web/src` has no test files and this task adds none, per the spec. Verification is typecheck + build + a look at the running page.

- [ ] **Step 1: Add the type**

In `web/src/api.ts`, extend `Status` (every field optional so an older backend, or one that could not read free space, decodes cleanly):

```ts
export interface Status {
  version: string;
  external_url: string;
  manifest_url: string;
  qbittorrent: { connected: boolean; version?: string; error?: string };
  downloader: { type: string; connected: boolean; version?: string; error?: string };
  torrents: Record<string, number>;
  total_uploaded: number;
  // used is seedstrem's own footprint; free is the whole filesystem's
  // headroom, reported by the download client or, when it cannot answer,
  // by a local statfs. They do not sum to a disk total, so there is no
  // percentage to show. free is absent when neither source answered.
  disk?: {
    used: number;
    free?: number;
    free_source?: "client" | "local";
  };
}
```

- [ ] **Step 2: Derive the torrent count**

In `web/src/pages/Dashboard.tsx`, add below the existing `downloaderName` line:

```tsx
  const downloaderName = status.downloader?.type === "deluge" ? "Deluge" : "qBittorrent";
  const trackedTorrents = Object.values(status.torrents ?? {}).reduce((a, b) => a + b, 0);
  const freeSpace = status.disk?.free;
  const freeSpaceHint =
    freeSpace === undefined
      ? "unavailable"
      : status.disk?.free_source === "local"
        ? "on host"
        : `on ${downloaderName}`;
```

- [ ] **Step 3: Add the two tiles**

In `web/src/pages/Dashboard.tsx`, insert directly after the existing `Uploaded` `StatCard` and before the closing `</div>` of the grid:

```tsx
        <StatCard
          label="Used"
          value={formatBytes(status.disk?.used ?? 0)}
          hint={`across ${trackedTorrents} torrent${trackedTorrents === 1 ? "" : "s"}`}
          accent="default"
        />
        <StatCard
          label="Free space"
          value={freeSpace === undefined ? "—" : formatBytes(freeSpace)}
          hint={freeSpaceHint}
          accent="default"
        />
```

Leave the first-load skeleton count at `8`. It is already a rough placeholder rather than a count derived from the real tiles (there are ~12 today), so bumping it is out of scope.

- [ ] **Step 4: Typecheck and build**

```bash
cd web && npm run build
```

`web/package.json`'s build script is `tsc -b && vite build`, so this
typechecks and bundles in one step. Expected: no TypeScript errors, build
succeeds.

- [ ] **Step 5: Verify in the running app**

```bash
make build && ./bin/seedstrem --config ./config.yaml
```

Open the Dashboard and confirm two new tiles: **Used** showing a byte figure with an `across N torrents` hint, and **Free space** showing either a byte figure hinted `on qBittorrent`/`on Deluge`/`on host`, or `—` hinted `unavailable` when no client is reachable. With no download client configured at all, **Free space** must read `—` rather than `0 B`.

- [ ] **Step 6: Commit**

```bash
git add web/src/api.ts web/src/pages/Dashboard.tsx
git commit -m "feat(web): show download disk used and free on the dashboard"
```

---

## Verification

Run before considering the plan done:

```bash
gofmt -l . && go vet ./... && go test -race -cover ./... && make build
```

Expected: `gofmt -l` prints nothing, vet clean, tests pass, build succeeds.

## Spec coverage

| Spec requirement | Task |
|---|---|
| `FreeSpace` on `downloader.Client`, `ErrNotSupported` default | 2 |
| Forwarded through `Swappable` | 2 |
| qBittorrent via `SyncMainDataCtx` → `FreeSpaceOnDisk` | 2 |
| Deluge via `GetFreeSpace(ctx, "")` | 2 |
| `TorrentInfo.Completed` from `Completed` / `TotalDone` | 1 |
| `disk.used` summed over tracked torrents, no extra client calls | 4 |
| `disk.free` client-first, `statfs` fallback, `free_source` label | 4 |
| Both fail → `free`/`free_source` omitted, `used` still reported | 4 |
| Injectable `diskUsage`, path from first local mapping | 3, 4 |
| Raw `int64` bytes, `snake_case` keys | 4 |
| `Status.disk` mirrored in hand-written TS types | 5 |
| Two tiles with source-revealing hints, dash when absent | 5 |
| `/api/status` never fails on disk errors | 4 |
| Go-only tests, no frontend suite | 4, 5 |
| No percentage / no disk total | all — stated in the constraints and in both code comments |
