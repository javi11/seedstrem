# seedstrem Web UI Redesign — Design

**Date:** 2026-07-21
**Branch:** `session/web-ui-friendliness-resilience-e84f49`
**Goal:** Make the admin web UI more **user-friendly** and **resilient** via a full visual redesign, weighing both goals roughly equally.

## Decisions (locked with user)

| Question | Decision |
|----------|----------|
| Scope | Full visual redesign |
| Priority | Balanced: friendliness + resilience |
| Aesthetic | Modern self-hosted dashboard (Sonarr/Radarr/Overseerr family) |
| Navigation | **Left sidebar** shell (collapses to top bar + hamburger on mobile) |
| Theme | **Dark-first** (`dim`) with a persisted light/dark toggle |
| Component base | Keep **DaisyUI 5** + Tailwind 4, custom theme |
| Torrent row actions | **Delete only** (new `DELETE /api/torrents/{id}`); no Retry |

## Current State (baseline)

- React 19 + Vite 6 + Tailwind 4 + DaisyUI 5, `HashRouter`. Themes today: `dim --default, light`.
- Pages: `Dashboard`, `Torrents`, `Settings`, `Login`; shell in `Layout.tsx` (top navbar).
- `api.ts` is a thin fetch client; a 401 sets `window.location.hash = "#/login"`. API is **read-only** for torrents (`GET /api/torrents`).
- No global error boundary, no toast system, no offline handling, no theme toggle, no web tests.

### Concrete problems this redesign fixes

- Torrents table renders **raw status strings** (`waiting_files_selection`) though friendly labels already exist in `Dashboard.tsx`.
- Torrents empty state wrongly references "the RealDebrid API".
- Torrents silently keeps **stale data** on fetch failure with no indicator; Dashboard only surfaces errors on *first* load.
- A render error **white-screens** the whole app (no error boundary).
- Settings is one ~740-line scrolling form with no navigation and no unsaved-changes protection.
- Feedback (saved/copied/errors) is scattered and inconsistent.

## Architecture

Keep the existing stack, router, and `api.ts` request/secret conventions. Introduce a small set of focused, independently-testable units. **Many small files** over large ones.

### New / changed structure

```
web/src/
  main.tsx                     # wrap routes in <ErrorBoundary> + <ToastProvider>
  theme.ts                     # get/set/toggle theme, persist to localStorage, apply data-theme
  index.css                    # custom daisyUI theme tokens (dark-first "dim"-based + light)
  api.ts                       # + deleteTorrent(); session-expired event instead of hard hash redirect
  lib/
    status.ts                  # STATUS_LABELS + badge/icon/color maps (single source of truth)
    format.ts                  # formatBytes (moved from api.ts), availableUntil (moved from Torrents)
    usePolling.ts              # polling hook: interval + backoff + stale/offline state
  components/
    AppShell.tsx               # sidebar + mobile top bar + <Outlet>; theme toggle; logout
    Sidebar.tsx / NavItem.tsx
    ErrorBoundary.tsx          # class component; friendly fallback + reload
    Toast.tsx / ToastProvider  # context + <useToast()>; success/error/info; auto-dismiss
    OfflineBanner.tsx          # navigator.onLine + polling-failure driven
    StatusBadge.tsx            # friendly badge from lib/status
    ProgressCell.tsx           # progress bar + %
    ConfirmDialog.tsx          # generic confirm (used by delete + unsaved-changes)
    Skeleton.tsx               # loading skeletons
    StatCard.tsx               # dashboard stat card
  pages/
    Dashboard.tsx              # stat cards + addon card; uses usePolling; skeletons
    Torrents.tsx               # redesigned table + mobile cards; delete; stale banner
    Settings/                  # split the mega-form
      Settings.tsx             # section router + sticky SaveBar + unsaved-changes guard
      sections/*.tsx           # DownloadClient, Prowlarr, ContentTypes, Filters,
                               #   Metadata, Seeding, PathMappings, Server, Streaming
      SaveBar.tsx
```

## Components & Data Flow

### 1. App shell (`AppShell`, `Sidebar`)
Replaces the `Layout.tsx` top navbar. Left sidebar: brand, nav (Dashboard / Torrents / Settings), and a footer with **theme toggle** + **Log out**. Below `md` the sidebar becomes a top bar with a hamburger drawer (DaisyUI `drawer`). Session check on mount is unchanged (redirect to `/login` if `sessionInfo()` fails), but shown with a skeleton, not a bare spinner.

### 2. Theming (`theme.ts`, `index.css`)
Define a custom dark theme (based on the current `dim`) + a light theme via DaisyUI 5 `@plugin "daisyui"` theme tokens, using the redesign palette (deep navy surfaces, green `--accent` for "growth"/success, blue `--primary`). `theme.ts` reads `localStorage.theme` (default dark), applies `data-theme` on `<html>`, and exposes a toggle. `index.html` keeps `data-theme="dim"` as the pre-hydration default to avoid a flash.

### 3. Status single-source (`lib/status.ts`)
One map from raw status → `{ label, icon, badgeClass, tone }`. Consumed by `StatusBadge`, Dashboard stat cards, and Torrents. Eliminates the raw-string leak and the duplicated label map.

### 4. Polling + resilience (`lib/usePolling.ts`)
A hook wrapping the existing interval pattern: `{ data, error, isStale, lastUpdated, isOffline }`. On failure it keeps last data, marks `isStale`, and backs off (e.g. 3s → up to ~30s), auto-resuming on success or `online` event. Dashboard (5s) and Torrents (3s) both use it. Drives `OfflineBanner` and the Torrents "Live / stale" indicator.

### 5. Toasts (`ToastProvider`, `useToast`)
Context providing `toast.success/error/info(msg)`. Auto-dismiss + manual close, stacked bottom-right. Replaces scattered inline "Copied ✓" / save messages / test-result strings where a transient confirmation is appropriate. (Persistent states — connection test results, restart-required — stay inline.)

### 6. Error boundary (`ErrorBoundary`)
Class component wrapping the router. Catches render errors, shows a branded "Something went wrong" card with the error message + **Reload**. Prevents white-screen.

### 7. Session-expired handling (`api.ts` + shell)
Instead of `api.ts` directly mutating `window.location.hash` on 401, it dispatches a `session-expired` event (or callback). The shell shows a `ConfirmDialog` ("Session expired — log in again") that navigates to `/login`, so an in-flight action doesn't vanish silently. `GET /api/session` 401 during the initial check still routes to login normally.

### 8. Torrents page
- Table with `StatusBadge`, `ProgressCell` (bar + %), speed/seeds/size/ratio/available-until.
- Header shows count + **Live / stale** indicator; stale banner when `isStale`.
- Expandable row: error message + file list with "Copy stream URL" (toast on copy).
- **Delete** action per row → `ConfirmDialog` → `api.deleteTorrent(id)` → toast + optimistic removal.
- Corrected empty state (remove "RealDebrid"; describe Stremio/Prowlarr flow).
- Below `md`: each row renders as a stacked **card** instead of a wide table.

### 9. Settings page
Split the mega-form into section components under `Settings/sections/`. A left sub-menu (grouped **Connections / Addon / System**) selects the visible section; sections needing a restart are tagged. A **sticky SaveBar** shows a dirty indicator and the Save button. Navigating away (section change or route change) with unsaved edits triggers `ConfirmDialog`. **Secret-handling and save/test logic are preserved exactly** (blank = keep; re-blank after save; category parsing; indexer toggles). On mobile the sub-menu becomes a `select` dropdown.

### 10. Dashboard
Stat cards (`StatCard`) for downloader connection (live pulse chip) + status counts + uploaded total; keep the external-URL-mismatch warning and the Stremio addon card (copy → toast, install deep link). Skeletons on first load.

## Backend change (Go)

Add one route + client method; everything else is frontend.

- `internal/admin/router.go`: `r.Delete("/torrents/{id}", h.deleteTorrent)` inside the authenticated group.
- `internal/admin/`: `deleteTorrent` handler — look up the stored torrent by id, call `torrents.Service.Remove(ctx, tor)` (already exists), return 204 / JSON error. Honors existing `delete_files_on_remove` config.
- Reuse existing CSRF/session middleware. No new config.

## Error Handling

- Network/API errors → kept data + stale/offline indicators + toast on user-initiated actions.
- Render errors → error boundary.
- 401 → graceful session-expired dialog.
- Delete failure → toast, row restored (no optimistic loss).
- Input: preserve existing client-side parsing/validation; keep "empty secret = unchanged".

## Testing

Add Vitest + React Testing Library (dev deps; `test` script already present). Focus on logic-heavy, low-flake units per the testing pyramid:

- **Unit:** `lib/format.ts` (`formatBytes`, `availableUntil` edge cases), `lib/status.ts` maps, `usePolling` stale/backoff/offline transitions (fake timers), theme persistence.
- **Component:** `StatusBadge`, `ErrorBoundary` fallback, `ToastProvider` add/dismiss, Torrents delete confirm→call→toast (mocked `api`), Settings unsaved-changes guard.
- **Backend:** Go handler test for `DELETE /api/torrents/{id}` (success, not-found, downloader error) alongside existing admin tests.
- Manual/visual verification for full-page layout and responsive breakpoints. Target meaningful coverage on the new `lib/` + components rather than a blanket percentage on presentational markup.

## Out of Scope (YAGNI)

- Retry action / re-add pipeline.
- Pause/resume, bulk actions, sorting/filtering/search on Torrents.
- Switching off HashRouter, i18n, real-time (WebSocket/SSE) — polling stays.
- Any new config surface beyond the delete route.

## Rollout

Incremental, each step keeping the app runnable: (1) foundation — theme, `lib/*`, ErrorBoundary, ToastProvider, shell; (2) Dashboard; (3) Torrents + backend delete; (4) Settings restructure; (5) resilience polish (offline, session-expired, skeletons); (6) tests. Detailed steps come from the implementation plan.
