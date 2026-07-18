# Local fork of github.com/autobrr/go-deluge (v1.4.0)

Vendored here (via a `go.mod` `replace` directive) rather than depended on
directly, because upstream is missing two RPC calls seedstrem's streaming
logic needs:

- Per-piece completion state (`core.get_torrent_status` with the `"pieces"`
  key) — upstream's `TorrentStatus`/`TorrentsStatus` never request it.
- Setting per-file download priority (`core.set_torrent_options` with a
  `"file_priorities"` key) — upstream's `Options` struct has no such field
  and there is no dedicated method for it.

Both are added here as new methods (`PieceStates`, `SetFilePriorities`) using
the exact same `rpc`/`rpcWithDictionaryResult` pattern upstream already uses
internally, in `deluge_extra.go`. Everything else is unmodified upstream
source (GPL-2.0, see LICENSE) as of the v1.4.0 tag.

If upstream ever adds equivalent methods, drop this fork and the `replace`
directive in the root `go.mod`.
