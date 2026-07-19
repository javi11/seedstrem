# 🌱 seedstrem

A self-hosted **Stremio addon** that searches your **Prowlarr** indexers and
streams the chosen torrent through your download client — **qBittorrent**
(default) or **Deluge** — playing files over HTTP **while they're still
downloading**.

```
Stremio ──stream request──► seedstrem ──search──► Prowlarr (your indexers)
   ▲                            │
   │                            ├──add magnet──► qBittorrent / Deluge (downloads & seeds)
   └── plays /dl stream URL ◄───┘  reads partial files from the shared volume
```

Nothing is added to the download client just by browsing — a torrent is only
fetched when you press play. Seeding stays entirely in the client.

## Features

- Stremio addon for movies, TV, and anime (each toggleable)
- Searches all your Prowlarr indexers, with de-dup and seeder/size/quality filters
- Stream-while-downloading with full Range/seek support
- qBittorrent (WebUI API) or Deluge 2 (daemon RPC) as the download client
- Fast in-stream seeking on Deluge via the bundled
  [Seedstream plugin](contrib/deluge-seedstream/) (libtorrent piece
  deadlines — something qBittorrent's API cannot do)
- Web UI for setup and monitoring

## Quick start

```bash
curl -LO https://raw.githubusercontent.com/javi11/seedstrem/main/docker-compose.yml
docker compose up -d
docker compose logs seedstrem | grep password   # admin_password
```

Open `http://<host>:8081` and log in, then in **Settings**:

1. Pick your download client and point seedstrem at it, plus Prowlarr
   (test each connection). qBittorrent: the WebUI API must be enabled and
   reachable with the configured username/password. Deluge: seedstrem
   talks to the Deluge 2 daemon RPC port (58846) — enable "Allow Remote
   Connections" and use an account from Deluge's auth file; for fast
   seeking install the [Seedstream plugin](contrib/deluge-seedstream/).
2. Enable the content types you want (movies / TV / anime).
3. Copy the manifest URL from the **Dashboard** and install it in Stremio.

The download client and seedstrem must see the same downloads directory —
the bundled `docker-compose.yml` mounts it at `/downloads` and `/data`
respectively; adjust **Settings → Path mappings** if you change that.

## Configuration

See [config.example.yaml](config.example.yaml). Every key has a
`SEEDSTREM_*` env override. Config changes made in the web UI are saved and
applied live (no restart needed, except for `listen`).

## Development

```bash
make test        # go test -race ./...
make build        # web UI + binary (bin/seedstrem)
cd web && npm run dev   # UI dev server proxying to :8080
```

Go backend (chi, SQLite, qBittorrent WebUI client, vendored Deluge RPC
client) + React/daisyUI frontend, embedded into a single binary.
