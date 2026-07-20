# Unit tests for the Seedstream plugin core, with the Deluge/libtorrent/
# Twisted runtime stubbed out so they run anywhere:
#   python3 -m unittest discover -s contrib/deluge-seedstream/tests
import sys
import types
import unittest
from pathlib import Path


def _install_stubs():
    """Register fake deluge/twisted modules before importing core."""
    if 'deluge' in sys.modules:
        return

    twisted = types.ModuleType('twisted')
    internet = types.ModuleType('twisted.internet')

    class _DelayedCall:
        def __init__(self):
            self._active = True

        def active(self):
            return self._active

        def cancel(self):
            self._active = False

    class _Reactor:
        def callLater(self, _secs, _fn, *args):
            return _DelayedCall()

    internet.reactor = _Reactor()
    twisted.internet = internet

    deluge = types.ModuleType('deluge')
    component = types.ModuleType('deluge.component')
    _registry = {}
    component.get = _registry.get
    component._registry = _registry

    libt = types.ModuleType('deluge._libtorrent')

    class _TorrentFlags:
        sequential_download = 1 << 9

    class _Lt:
        torrent_flags = _TorrentFlags

    libt.lt = _Lt

    rpcserver = types.ModuleType('deluge.core.rpcserver')
    rpcserver.export = lambda fn: fn
    core_pkg = types.ModuleType('deluge.core')
    core_pkg.rpcserver = rpcserver

    pluginbase = types.ModuleType('deluge.plugins.pluginbase')

    class CorePluginBase:
        def __init__(self, *_args, **_kwargs):
            pass

    pluginbase.CorePluginBase = CorePluginBase
    plugins_init = types.ModuleType('deluge.plugins.init')

    class PluginInitBase:
        def __init__(self, *_args, **_kwargs):
            pass

    plugins_init.PluginInitBase = PluginInitBase
    plugins_pkg = types.ModuleType('deluge.plugins')
    plugins_pkg.pluginbase = pluginbase
    plugins_pkg.init = plugins_init

    deluge.component = component
    deluge._libtorrent = libt
    deluge.core = core_pkg
    deluge.plugins = plugins_pkg

    sys.modules.update({
        'twisted': twisted,
        'twisted.internet': internet,
        'deluge': deluge,
        'deluge.component': component,
        'deluge._libtorrent': libt,
        'deluge.core': core_pkg,
        'deluge.core.rpcserver': rpcserver,
        'deluge.plugins': plugins_pkg,
        'deluge.plugins.pluginbase': pluginbase,
        'deluge.plugins.init': plugins_init,
    })


_install_stubs()
sys.path.insert(0, str(Path(__file__).resolve().parent.parent))
from deluge_seedstream import core as seedstream_core  # noqa: E402


class FakeStatus:
    def __init__(self, num_pieces, pieces):
        self.num_pieces = num_pieces
        self.pieces = pieces


class FakeTorrentFile:
    def __init__(self, piece_length):
        self._piece_length = piece_length

    def piece_length(self):
        return self._piece_length


class FakeHandle:
    def __init__(self, num_pieces=100, piece_length=2 * 1024 * 1024):
        self.num_pieces = num_pieces
        self._piece_length = piece_length
        self.pieces = [False] * num_pieces
        self.deadlines = {}       # piece -> deadline_ms
        self.priorities = {}      # piece -> priority
        self.reset_calls = []     # pieces whose deadline was reset
        self._flags = 0

    def status(self):
        return FakeStatus(self.num_pieces, list(self.pieces))

    def torrent_file(self):
        return FakeTorrentFile(self._piece_length)

    def flags(self):
        return self._flags

    def set_flags(self, f):
        self._flags |= f

    def unset_flags(self, f):
        self._flags &= ~f

    def set_piece_deadline(self, piece, deadline_ms):
        self.deadlines[piece] = deadline_ms

    def reset_piece_deadline(self, piece):
        self.reset_calls.append(piece)
        self.deadlines.pop(piece, None)

    def piece_priority(self, piece, priority):
        self.priorities[piece] = priority


class FakeTorrent:
    def __init__(self, handle):
        self.handle = handle


class FakeTorrentManager:
    def __init__(self):
        self.torrents = {}


class FakeSession:
    def __init__(self, settings=None):
        self.settings = dict(settings or {})
        self.applied = []

    def get_settings(self):
        return dict(self.settings)

    def apply_settings(self, changes):
        self.applied.append(dict(changes))
        self.settings.update(changes)


class FakeCore:
    def __init__(self, session):
        self.session = session


def new_plugin(handle=None, session=None):
    import deluge.component as component
    tm = FakeTorrentManager()
    if handle is not None:
        tm.torrents['hash1'] = FakeTorrent(handle)
    session = session or FakeSession({'max_out_request_queue': 500})
    component._registry.clear()
    component._registry['TorrentManager'] = tm
    component._registry['Core'] = FakeCore(session)
    plugin = seedstream_core.Core('Seedstream')
    plugin.enable()
    return plugin, session


class SessionTuningTests(unittest.TestCase):
    def test_enable_raises_max_out_request_queue(self):
        _plugin, session = new_plugin()
        self.assertGreaterEqual(
            session.settings['max_out_request_queue'],
            seedstream_core.MAX_OUT_REQUEST_QUEUE,
        )

    def test_enable_never_lowers_an_already_high_queue(self):
        big = seedstream_core.MAX_OUT_REQUEST_QUEUE * 2
        _plugin, session = new_plugin(
            session=FakeSession({'max_out_request_queue': big})
        )
        self.assertEqual(session.settings['max_out_request_queue'], big)

    def test_disable_restores_previous_queue_setting(self):
        plugin, session = new_plugin()
        plugin.disable()
        self.assertEqual(session.settings['max_out_request_queue'], 500)
        # One apply on enable (raise) and one on disable (restore).
        self.assertEqual(len(session.applied), 2)


class WindowClearingTests(unittest.TestCase):
    def test_new_window_clears_stale_pieces_of_previous_window(self):
        handle = FakeHandle()
        plugin, _ = new_plugin(handle)

        plugin.prioritize_range('hash1', 10, 20)
        plugin.prioritize_range('hash1', 18, 30)

        # Pieces 10-17 left the window: deadline dropped, priority normal.
        for piece in range(10, 18):
            self.assertIn(piece, handle.reset_calls, f'piece {piece} not reset')
            self.assertEqual(handle.priorities.get(piece), 4)
        # Pieces still inside the new window must NOT have been reset.
        for piece in range(18, 31):
            self.assertNotIn(piece, handle.reset_calls, f'piece {piece} reset')

    def test_repeat_of_same_window_clears_nothing(self):
        handle = FakeHandle()
        plugin, _ = new_plugin(handle)

        plugin.prioritize_range('hash1', 10, 20)
        plugin.prioritize_range('hash1', 10, 20)

        self.assertEqual(handle.reset_calls, [])

    def test_clear_range_forgets_tracked_window(self):
        handle = FakeHandle()
        plugin, _ = new_plugin(handle)

        plugin.prioritize_range('hash1', 10, 20)
        plugin.clear_range('hash1', 10, 20)
        handle.reset_calls.clear()

        # No stale window left: a new window must not clear anything.
        plugin.prioritize_range('hash1', 50, 60)
        self.assertEqual(handle.reset_calls, [])

    def test_clear_range_also_clears_tracked_window_on_mismatch(self):
        # A clear_range whose range doesn't match the tracked window
        # (stale/racing RPC) must still drop the window's outstanding
        # deadlines — otherwise they linger with no bookkeeping left.
        handle = FakeHandle()
        plugin, _ = new_plugin(handle)

        plugin.prioritize_range('hash1', 10, 20)
        plugin.clear_range('hash1', 50, 60)

        for piece in range(10, 21):
            self.assertIn(piece, handle.reset_calls, f'piece {piece} not reset')

    def test_update_prunes_windows_of_removed_torrents(self):
        import deluge.component as component
        handle = FakeHandle()
        plugin, _ = new_plugin(handle)

        plugin.prioritize_range('hash1', 10, 20)
        component._registry['TorrentManager'].torrents.clear()
        plugin.update()

        # No stale window survives torrent removal: re-adding the
        # torrent and prioritizing must clear nothing.
        component._registry['TorrentManager'].torrents['hash1'] = FakeTorrent(handle)
        plugin.prioritize_range('hash1', 50, 60)
        self.assertEqual(handle.reset_calls, [])

    def test_stale_window_clearing_survives_per_piece_errors(self):
        handle = FakeHandle()

        original = handle.reset_piece_deadline

        def flaky_reset(piece):
            if piece == 12:
                raise RuntimeError('boom')
            original(piece)

        handle.reset_piece_deadline = flaky_reset
        plugin, _ = new_plugin(handle)

        plugin.prioritize_range('hash1', 10, 20)
        plugin.prioritize_range('hash1', 40, 50)

        # Best-effort: pieces after the failing one are still cleared.
        for piece in (10, 11, 13, 14, 15, 16, 17, 18, 19, 20):
            self.assertIn(piece, handle.reset_calls, f'piece {piece} not reset')

    def test_prioritize_still_deadlines_head_and_prioritizes_rest(self):
        handle = FakeHandle()
        plugin, _ = new_plugin(handle)

        plugin.prioritize_range('hash1', 0, 15, 500, 50)

        # 8 MiB / 2 MiB pieces = 4 deadline pieces, staggered.
        self.assertEqual(handle.deadlines, {0: 500, 1: 550, 2: 600, 3: 650})
        for piece in range(4, 16):
            self.assertEqual(handle.priorities.get(piece), seedstream_core.TOP_PRIORITY)


if __name__ == '__main__':
    unittest.main()
