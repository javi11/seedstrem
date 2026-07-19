# Seedstream plugin core: exposes libtorrent piece deadlines over RPC.
#
# RPC methods (wire names):
#   seedstream.api_version()                  -> int
#   seedstream.prioritize_range(id, a, b, …)  -> bool
#   seedstream.clear_range(id, a, b)          -> bool
#
# seedstrem calls prioritize_range when a player seeks into a region of
# the file that has not downloaded yet: set_piece_deadline makes
# libtorrent fetch exactly those pieces ahead of the sequential order.

import logging

import deluge.component as component
from deluge.core.rpcserver import export
from deluge.plugins.pluginbase import CorePluginBase

log = logging.getLogger(__name__)

API_VERSION = 1

# Default deadline for the first prioritized piece, and the stagger added
# per subsequent piece so they arrive roughly in playback order.
DEFAULT_DEADLINE_MS = 3000
DEFAULT_STEP_MS = 50


class Core(CorePluginBase):
    def enable(self):
        log.info('Seedstream plugin enabled (api_version=%d)', API_VERSION)

    def disable(self):
        pass

    def update(self):
        pass

    def _handle(self, torrent_id):
        """Return the raw libtorrent handle for a torrent, or None."""
        torrents = component.get('TorrentManager').torrents
        torrent = torrents.get(torrent_id)
        if torrent is None:
            return None
        return torrent.handle

    @export
    def api_version(self):
        """Protocol version, so seedstrem can detect the plugin."""
        return API_VERSION

    @export
    def prioritize_range(
        self, torrent_id, first, last, deadline_ms=DEFAULT_DEADLINE_MS, step_ms=DEFAULT_STEP_MS
    ):
        """Ask libtorrent to fetch pieces [first, last] ASAP.

        Deadlines are staggered so earlier pieces get earlier deadlines.
        Out-of-range indices are clamped; a missing torrent or absent
        metadata returns False instead of raising across RPC.
        """
        handle = self._handle(torrent_id)
        if handle is None:
            log.debug('seedstream.prioritize_range: unknown torrent %s', torrent_id)
            return False
        status = handle.status()
        num_pieces = int(getattr(status, 'num_pieces', 0) or 0)
        if num_pieces <= 0:
            log.debug('seedstream.prioritize_range: %s has no metadata yet', torrent_id)
            return False

        first = max(0, min(int(first), num_pieces - 1))
        last = max(first, min(int(last), num_pieces - 1))
        deadline_ms = max(0, int(deadline_ms))
        step_ms = max(0, int(step_ms))

        for i, piece in enumerate(range(first, last + 1)):
            try:
                handle.set_piece_deadline(piece, deadline_ms + i * step_ms)
            except Exception:
                log.exception(
                    'seedstream.prioritize_range: set_piece_deadline(%d) failed for %s',
                    piece,
                    torrent_id,
                )
                return False
        log.debug(
            'seedstream: prioritized pieces %d-%d of %s (deadline=%dms step=%dms)',
            first,
            last,
            torrent_id,
            deadline_ms,
            step_ms,
        )
        return True

    @export
    def clear_range(self, torrent_id, first, last):
        """Drop the deadlines previously set on pieces [first, last]."""
        handle = self._handle(torrent_id)
        if handle is None:
            return False
        status = handle.status()
        num_pieces = int(getattr(status, 'num_pieces', 0) or 0)
        if num_pieces <= 0:
            return False

        first = max(0, min(int(first), num_pieces - 1))
        last = max(first, min(int(last), num_pieces - 1))
        for piece in range(first, last + 1):
            try:
                handle.reset_piece_deadline(piece)
            except Exception:
                log.exception(
                    'seedstream.clear_range: reset_piece_deadline(%d) failed for %s',
                    piece,
                    torrent_id,
                )
                return False
        return True
