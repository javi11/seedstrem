# Vendored go-deluge (package `delugerpc`)

This directory is an in-tree copy of [github.com/autobrr/go-deluge](https://github.com/autobrr/go-deluge)
**v1.4.0** (itself a maintained fork of [gdm85/go-libdeluge](https://github.com/gdm85/go-libdeluge) v0.5.6),
a native Deluge daemon RPC client (rencode + zlib over TCP/TLS).

## License

The library is licensed under the **GNU General Public License v2.0**
(see [LICENSE](LICENSE) and the per-file headers). The copied files keep
their original headers. Distributing seedstrem binaries that include this
package is subject to the GPL-2.0 terms.

## Why vendored

Upstream keeps its raw `rpc()` method unexported and does not cover three
calls seedstrem needs. Rather than forking, the library is copied here and
extended in a single separate file so the upstream files stay pristine and
easy to diff/refresh against a newer release.

## Local modifications

- Package renamed `deluge` → `delugerpc` (every `.go` file, mechanical).
- Upstream's tests, integration suite, scripts, go.mod are not copied.
- **All additions live in [seedstrem.go](seedstrem.go)** (new file, not
  from upstream):
  - `RPC(ctx, method, args, kwargs)` — exported raw RPC call, used to
    reach custom plugin exports (`seedstream.prioritize_range`).
  - `PieceStates(ctx, hash)` — fetches the `pieces` torrent-status key
    (per-piece states: 0/1 missing, 2 downloading, 3 have; `nil` when
    Deluge reports None for metadata-less or seeding torrents).
  - `SetFilePriorities(ctx, hash, priorities)` — full `file_priorities`
    write via `core.set_torrent_options`, which upstream's `Options`
    struct cannot express.

## Refreshing

Copy the `.go` files (account, delugeclient, methods, options, plugins,
session_status, torrent_status) + LICENSE from the new upstream tag, re-run
the package rename, and keep `seedstrem.go` + this README.
