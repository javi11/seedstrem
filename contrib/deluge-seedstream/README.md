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
| `seedstream.api_version()` | returns `3`; used for detection |
| `seedstream.prioritize_range(torrent_id, first, last, deadline_ms=3000, step_ms=50)` | staggered `set_piece_deadline` on the first ~8 MiB of `[first, last]`, top piece priority on the rest (clamped) |
| `seedstream.clear_range(torrent_id, first, last)` | `reset_piece_deadline` on the range, ends focus mode |

Methods never raise across RPC; failures return `False` and are logged
by the daemon.

### Focus mode (api_version 2)

When `prioritize_range` targets a window starting more than 16 pieces
ahead of the sequential frontier — a player seek, not a playback stall —
the plugin temporarily unsets the torrent's `sequential_download` flag
so the deadline window gets the swarm's full bandwidth instead of
competing with the sequential flood's inflight backlog. Every
`prioritize_range` call re-arms a 15s timer; sequential download is
restored when the timer fires (playback stopped blocking) or on
`clear_range`/plugin disable.

### Session tuning & stale-window cleanup (api_version 3)

On enable the plugin raises libtorrent's `max_out_request_queue` to
3000 blocks (never lowering an operator's own higher setting; restored
on disable). The default (~500 blocks ≈ 8 MiB in flight per peer)
starves fast links: with the queue permanently full, new requests —
including the deadline'd head/tail pieces — cannot be issued, and the
daemon log floods with `outstanding_request_limit_reached` warnings.

Each `prioritize_range` call also remembers its window per torrent and,
on the next call, resets the deadline/priority of pieces that left the
window, so unmet deadlines from superseded windows stop re-requesting
blocks redundantly and eating queue slots.

## Tests

```bash
python3 -m unittest discover -s contrib/deluge-seedstream/tests
```

The Deluge/libtorrent/Twisted runtime is stubbed, so the tests run on
any Python 3 without Deluge installed.

## Download a prebuilt egg

CI builds the egg for Python 3.9–3.13 on every change to this directory
(the **Deluge plugin** workflow under the repo's Actions tab — grab the
artifact matching your daemon's Python major.minor). On `v*` release
tags the same eggs are attached to the GitHub Release.

Check the daemon's Python with:
`docker exec deluge python3 --version` (or `python3 --version` on the
host running deluged).

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
