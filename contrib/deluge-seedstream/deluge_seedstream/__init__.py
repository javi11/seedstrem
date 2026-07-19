# Seedstream Deluge plugin: piece-deadline streaming primitives for
# seedstrem. See core.py for the exported RPC methods.
#
# This program is free software; you can redistribute it and/or modify
# it under the terms of the GNU General Public License as published by
# the Free Software Foundation; either version 3 of the License, or (at
# your option) any later version.

from deluge.plugins.init import PluginInitBase


class CorePlugin(PluginInitBase):
    def __init__(self, plugin_name):
        from .core import Core as _pluginCls

        self._plugin_cls = _pluginCls
        super().__init__(plugin_name)
