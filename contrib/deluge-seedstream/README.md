# Seedstream Deluge plugin

A tiny Deluge 2 core plugin that exposes libtorrent's
`set_piece_deadline` over the daemon RPC, so seedstrem can prioritize
the exact pieces a video player just seeked to. Without it, seeking into
a not-yet-downloaded region waits for the sequential download to reach
that offset; with it, the swarm is redirected to the seek target within
seconds.

seedstrem detects the plugin automatically (`seedstream.api_version`)
and degrades gracefully when it is absent — streaming still works, only
seek prioritization is skipped.

## RPC surface

| Method | Purpose |
| --- | --- |
| `seedstream.api_version()` | returns `1`; used for detection |
| `seedstream.prioritize_range(torrent_id, first, last, deadline_ms=3000, step_ms=50)` | staggered `set_piece_deadline` on pieces `[first, last]` (clamped) |
| `seedstream.clear_range(torrent_id, first, last)` | `reset_piece_deadline` on the range |

Methods never raise across RPC; failures return `False` and are logged
by the daemon.

## Build

The egg must be built with the **same Python major.minor as the Deluge
daemon** — Deluge only loads eggs matching its interpreter version.

```bash
# On a machine with that Python + setuptools:
make egg          # → dist/Seedstream-1.0-py3.X.egg

# Or inside a Deluge container (linuxserver.io image shown):
docker cp contrib/deluge-seedstream deluge:/tmp/seedstream
docker exec -w /tmp/seedstream deluge python3 setup.py bdist_egg
docker exec deluge sh -c 'cp /tmp/seedstream/dist/*.egg /config/plugins/'
```

## Install & enable

1. Copy the egg into the Deluge config's `plugins/` directory
   (`/config/plugins/` on the linuxserver.io image).
2. Restart the daemon, or reload plugins.
3. Enable **Seedstream** under Preferences → Plugins (web UI or GTK), or
   with the console: `deluge-console "plugin -e Seedstream"`.

## Smoke test

With a still-downloading torrent playing through seedstrem, seek to
~80% of the video. In seedstrem's debug logs the availability summary
should show the seek-target pieces flipping to `have` within a few
seconds while the sequential frontier is still far behind; in the Deluge
daemon log (level debug) you should see
`seedstream: prioritized pieces N-M`.
