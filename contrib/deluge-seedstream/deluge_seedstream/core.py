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
#
# When the requested window sits well ahead of the sequential frontier
# (a forward seek), the plugin additionally enters "focus" mode: the
# torrent's sequential_download flag is temporarily unset so the whole
# swarm bandwidth serves the deadline window instead of competing with
# the frontier flood. Each prioritize_range call re-arms the focus
# timer; sequential download is restored FOCUS_SECS after the last one.

import logging

from twisted.internet import reactor

import deluge.component as component
from deluge._libtorrent import lt
from deluge.core.rpcserver import export
from deluge.plugins.pluginbase import CorePluginBase

log = logging.getLogger(__name__)

API_VERSION = 3

# Default deadline for the first prioritized piece, and the stagger added
# per subsequent piece so they arrive roughly in playback order.
DEFAULT_DEADLINE_MS = 3000
DEFAULT_STEP_MS = 50

# Focus mode: how long after the last prioritize_range call sequential
# download is restored, and how far (in pieces) ahead of the frontier a
# window must start to count as a seek rather than a playback stall.
FOCUS_SECS = 15
FOCUS_MARGIN_PIECES = 16

# Only the first ~DEADLINE_BYTES of a window get set_piece_deadline;
# the rest get top piece priority instead. Deadline (time-critical)
# pieces that miss their deadline are re-requested redundantly from
# multiple peers — deadlining a whole 32 MiB window put dozens of
# permanently-late pieces in that mode, flooding the daemon with
# outstanding_request_limit_reached alerts hard enough to stall the RPC
# thread. Priorities order the picker with none of that urgency.
DEADLINE_BYTES = 8 * 1024 * 1024
DEADLINE_MIN_PIECES = 2
DEADLINE_MAX_PIECES = 8
TOP_PRIORITY = 7
NORMAL_PRIORITY = 4

# libtorrent's default max_out_request_queue (500 blocks ≈ 8 MiB in
# flight per peer) starves fast links: with the queue permanently full,
# new requests — including the time-critical head/tail pieces this
# plugin deadlines — cannot be issued, and the daemon log fills with
# outstanding_request_limit_reached performance warnings. Raised on
# enable (never lowered below an operator's own higher setting) and
# restored on disable.
MAX_OUT_REQUEST_QUEUE = 3000


class Core(CorePluginBase):
    def enable(self):
        # torrent_id -> twisted DelayedCall restoring sequential download.
        self._focus = {}
        # torrent_id -> (first, last) window last passed to
        # prioritize_range, so a new window can clear the deadlines and
        # priorities the previous one left behind.
        self._windows = {}
        self._saved_queue = None
        self._tune_session()
        log.info('Seedstream plugin enabled (api_version=%d)', API_VERSION)

    def disable(self):
        for torrent_id in list(self._focus):
            self._unfocus(torrent_id)
        self._windows.clear()
        self._restore_session()

    def _tune_session(self):
        try:
            session = component.get('Core').session
            current = session.get_settings().get('max_out_request_queue', 0)
            if current >= MAX_OUT_REQUEST_QUEUE:
                return
            self._saved_queue = current
            session.apply_settings({'max_out_request_queue': MAX_OUT_REQUEST_QUEUE})
            log.info(
                'seedstream: raised max_out_request_queue %d -> %d',
                current,
                MAX_OUT_REQUEST_QUEUE,
            )
        except Exception:
            log.exception('seedstream: tuning session settings failed')

    def _restore_session(self):
        if self._saved_queue is None:
            return
        try:
            session = component.get('Core').session
            session.apply_settings({'max_out_request_queue': self._saved_queue})
        except Exception:
            log.exception('seedstream: restoring session settings failed')
        self._saved_queue = None

    def update(self):
        # Deluge calls this ~once a second: prune window bookkeeping for
        # torrents that were removed without a final clear_range, so the
        # per-torrent dicts don't grow for the daemon's lifetime.
        try:
            torrents = component.get('TorrentManager').torrents
        except Exception:
            return
        for torrent_id in list(self._windows):
            if torrent_id not in torrents:
                del self._windows[torrent_id]

    def _handle(self, torrent_id):
        """Return the raw libtorrent handle for a torrent, or None."""
        torrents = component.get('TorrentManager').torrents
        torrent = torrents.get(torrent_id)
        if torrent is None:
            return None
        return torrent.handle

    @staticmethod
    def _sequential_enabled(handle):
        try:
            return bool(handle.flags() & lt.torrent_flags.sequential_download)
        except Exception:
            return False

    def _focus_window(self, torrent_id, handle):
        """Suspend sequential download while a seek window is fetched.

        Only torrents currently in sequential mode are touched, and the
        flag is restored FOCUS_SECS after the last prioritize_range call
        (each call re-arms the timer, so focus tracks active playback).
        """
        call = self._focus.get(torrent_id)
        if call is None:
            if not self._sequential_enabled(handle):
                return
            try:
                handle.unset_flags(lt.torrent_flags.sequential_download)
            except Exception:
                log.exception(
                    'seedstream: suspending sequential download failed for %s', torrent_id
                )
                return
            log.debug('seedstream: focus on %s (sequential suspended)', torrent_id)
        elif call.active():
            call.cancel()
        self._focus[torrent_id] = reactor.callLater(FOCUS_SECS, self._unfocus, torrent_id)

    def _unfocus(self, torrent_id):
        call = self._focus.pop(torrent_id, None)
        if call is None:
            return
        if call.active():
            call.cancel()
        handle = self._handle(torrent_id)
        if handle is None:
            return
        try:
            handle.set_flags(lt.torrent_flags.sequential_download)
            log.debug('seedstream: focus off %s (sequential restored)', torrent_id)
        except Exception:
            log.exception('seedstream: restoring sequential download failed for %s', torrent_id)

    @staticmethod
    def _deadline_pieces(handle):
        """How many leading window pieces get a hard deadline (~8 MiB)."""
        piece_len = 0
        try:
            tf = handle.torrent_file()
            if tf is not None:
                piece_len = int(tf.piece_length())
        except Exception:
            piece_len = 0
        if piece_len <= 0:
            return DEADLINE_MAX_PIECES
        k = DEADLINE_BYTES // piece_len
        return max(DEADLINE_MIN_PIECES, min(DEADLINE_MAX_PIECES, k))

    @staticmethod
    def _frontier(status):
        """Index of the first piece not yet downloaded, or None."""
        try:
            bits = status.pieces
        except Exception:
            return None
        if not bits:
            return None
        for i, have in enumerate(bits):
            if not have:
                return i
        return None

    def _clear_stale_window(self, torrent_id, handle, first, last):
        """Reset deadline/priority on old-window pieces outside [first, last]."""
        prev = self._windows.get(torrent_id)
        if prev is None:
            return
        for piece in range(prev[0], prev[1] + 1):
            if first <= piece <= last:
                continue
            # Best-effort per piece: the calls are independent, and the
            # window entry is overwritten right after this returns, so a
            # piece skipped here would keep its stale deadline forever.
            try:
                handle.reset_piece_deadline(piece)
                handle.piece_priority(piece, NORMAL_PRIORITY)
            except Exception:
                log.exception(
                    'seedstream: clearing stale piece %d failed for %s',
                    piece,
                    torrent_id,
                )

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
        metadata returns False instead of raising across RPC. A window
        starting well ahead of the sequential frontier also suspends
        sequential download for a while (see module docstring).
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

        frontier = self._frontier(status)
        if frontier is not None and first - frontier > FOCUS_MARGIN_PIECES:
            self._focus_window(torrent_id, handle)

        # Clear what the previous window set on pieces that left the
        # window: deadlines libtorrent could not meet keep re-requesting
        # blocks redundantly forever, eating outstanding-request-queue
        # slots the *current* window needs.
        self._clear_stale_window(torrent_id, handle, first, last)
        self._windows[torrent_id] = (first, last)

        deadline_pieces = self._deadline_pieces(handle)
        for i, piece in enumerate(range(first, last + 1)):
            try:
                if i < deadline_pieces:
                    handle.set_piece_deadline(piece, deadline_ms + i * step_ms)
                else:
                    handle.piece_priority(piece, TOP_PRIORITY)
            except Exception:
                log.exception(
                    'seedstream.prioritize_range: prioritizing piece %d failed for %s',
                    piece,
                    torrent_id,
                )
                return False
        log.debug(
            'seedstream: prioritized pieces %d-%d of %s (deadline=%dms step=%dms deadline_pieces=%d)',
            first,
            last,
            torrent_id,
            deadline_ms,
            step_ms,
            deadline_pieces,
        )
        return True

    @export
    def clear_range(self, torrent_id, first, last):
        """Drop the deadlines previously set on pieces [first, last].

        The tracked prioritize_range window is cleared as well even when
        it doesn't match the caller's range (stale or racing RPC call):
        its bookkeeping is dropped here, so this is the last chance to
        reset those deadlines.
        """
        self._unfocus(torrent_id)
        window = self._windows.pop(torrent_id, None)
        handle = self._handle(torrent_id)
        if handle is None:
            return False
        status = handle.status()
        num_pieces = int(getattr(status, 'num_pieces', 0) or 0)
        if num_pieces <= 0:
            return False

        first = max(0, min(int(first), num_pieces - 1))
        last = max(first, min(int(last), num_pieces - 1))
        pieces = set(range(first, last + 1))
        if window is not None:
            pieces.update(range(window[0], window[1] + 1))
        ok = True
        for piece in sorted(pieces):
            try:
                handle.reset_piece_deadline(piece)
                handle.piece_priority(piece, NORMAL_PRIORITY)
            except Exception:
                log.exception(
                    'seedstream.clear_range: reset_piece_deadline(%d) failed for %s',
                    piece,
                    torrent_id,
                )
                ok = False
        return ok
