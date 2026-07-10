# 🌱 seedstrem

A self-hosted **Stremio addon** that searches your own **Prowlarr** indexers and
streams the chosen torrent through **qBittorrent** — playing files over HTTP
**while they are still downloading**, with full Range/seek support.

```
Stremio ──stream request──► seedstrem ──search──► Prowlarr (your indexers)
   ▲                            │
   │                            ├──add magnet──► qBittorrent (downloads & seeds)
   └── plays /dl stream URL ◄───┘  reads partial files from the shared volume
```

## How it works

seedstrem is a **stream-only** Stremio addon: it declares no catalogs and lets
Stremio's built-in Cinemeta provide metadata. When you open a movie or episode:

1. **Discovery** (`/stremio/stream/{type}/{id}.json`): seedstrem resolves the
   title (Cinemeta for IMDB ids, Kitsu for anime), searches Prowlarr, then
   ranks/filters the results and returns them as streams.
2. **Resolve-on-play** (`/stremio/play/{infohash}`): only when you actually
   click a stream does seedstrem add the magnet to qBittorrent, wait for
   metadata, pick the right file (episode-matching for series), and redirect to
   the `/dl` streaming endpoint.
3. **Stream while downloading**: `/dl/{token}` maps byte ranges to torrent
   pieces and waits for the pieces each read needs (bounded by a timeout), so
   playback starts as soon as the head of the file exists. Torrents are added
   with sequential download + first/last-piece priority so the container header
   (MP4 `moov`, MKV cues) arrives first.

Nothing is added to qBittorrent just by browsing — torrents are only downloaded
when you press play. Seeding stays entirely in qBittorrent (configure ratio /
time limits there; seedstrem never interferes).

## Features

- **Stremio addon** (`/stremio/manifest.json`) for movies, TV, and anime —
  each toggleable per install.
- **Prowlarr search** across all your configured indexers, with de-duplication
  by infohash and configurable min-seeders / size-band / quality filters.
- **Resolve-on-play** so browsing never floods qBittorrent.
- **Range requests**: seeking inside downloaded regions is instant; seeking
  ahead of the download frontier buffers until the pieces arrive.
- **Web UI** (React + daisyUI): dashboard with the installable manifest URL,
  torrent list with per-file stream links, and settings with live Prowlarr /
  qBittorrent connection tests.

## Quick start (Docker Compose)

```bash
curl -LO https://raw.githubusercontent.com/javi11/seedstrem/main/docker-compose.yml
docker compose up -d
docker compose logs seedstrem | grep password   # admin_password
```

Open `http://<host>:8080`, log in with the printed admin password, then:

1. **Settings → qBittorrent**: set the WebUI URL/credentials, hit **Test
   connection**. (The linuxserver image prints its initial password with
   `docker compose logs qbittorrent`.)
2. **Settings → Prowlarr**: set the Prowlarr URL + API key (Prowlarr →
   Settings → General), hit **Test connection**. Add indexers inside Prowlarr.
3. **Settings → Content types**: enable movies / TV / anime as desired.
4. **Dashboard**: copy the manifest URL or click **Install in Stremio**.

### The path mapping (important)

seedstrem reads the files qBittorrent writes, so **both containers must see the
same directory**:

| Container   | Host path     | Mount        |
|-------------|---------------|--------------|
| qbittorrent | `./downloads` | `/downloads` |
| seedstrem   | `./downloads` | `/data`      |

The default config maps `qbit: /downloads → local: /data`. If you change the
mounts, update **Settings → Path mappings**. Run both containers with the same
PUID/GID so seedstrem can read the files.

## Installing in Stremio

Copy the manifest URL from the dashboard (e.g.
`http://<host>:8080/stremio/manifest.json`) and paste it into Stremio →
Addons → *Install from URL*, or use the **Install in Stremio** button (a
`stremio://` deep link). `server.external_url` must be an address **Stremio can
reach** — the dashboard warns when it doesn't match your browsing address.

## Configuration

See [config.example.yaml](config.example.yaml). Every key has a `SEEDSTREM_*`
environment override (e.g. `SEEDSTREM_PROWLARR_URL`). Settings changed in the
web UI are persisted to the config file and hot-applied (changing `listen`
needs a restart). Prowlarr and Cinemeta are read live per request, so URL/key
changes take effect immediately.

Prowlarr may be left unconfigured on first boot — the addon simply returns no
streams until you set it up in the UI.

> **Note:** on first run seedstrem generates and **logs** the admin password
> once (then persists it to the config file). Restrict access to your logs, or
> set `SEEDSTREM_SERVER_ADMIN_PASSWORD` yourself to avoid the log line.

## Development

```bash
make test        # go test -race ./...
make go-build    # build without rebuilding the web UI
make build       # web UI + binary (bin/seedstrem)
cd web && npm run dev   # UI dev server proxying /api to :8080
```

Backend: Go (chi, modernc SQLite, autobrr/go-qbittorrent). Frontend: Vite,
React 19, Tailwind v4, daisyUI 5, embedded into the binary with `go:embed`.

### Layout

```
cmd/seedstrem        entrypoint
internal/stremio     Stremio manifest + stream/resolve handlers
internal/prowlarr    Prowlarr search client + result ranking/filtering
internal/meta        Cinemeta + Kitsu title resolution, Stremio id parsing
internal/torrents    add magnet → wait for metadata → select file → link token
internal/stream      piece math, availability waits, Range streaming
internal/qbit        qBittorrent client wrapper (+ fake WebUI for tests)
internal/store       SQLite: torrent ids, selection phase, link tokens
internal/admin       web UI API (sessions, config, status)
internal/syncer      background reconcile with qBittorrent
web/                 React + daisyUI SPA
```
