// seedstrem: local addition to the vendored go-deluge library, like
// seedstrem.go. Licensed GPL-2.0 with the rest of the package.
package delugerpc

import (
	"context"

	"github.com/gdm85/go-rencode"
)

// TorrentsStatusByLabel returns the status of every torrent carrying
// label. The filter is applied by the daemon (the Label plugin adds a
// "label" key to core.get_torrents_status's filter dict), so torrents
// belonging to other tools never cross the wire.
func (c *Client) TorrentsStatusByLabel(ctx context.Context, label string) (map[string]*TorrentStatus, error) {
	var filterDict rencode.Dictionary
	filterDict.Add("label", label)

	var args rencode.List
	args.Add(filterDict)
	if !c.v2daemon {
		args.Add(statusKeysV1)
	} else {
		args.Add(statusKeysV2)
	}

	rd, err := c.rpcWithDictionaryResult(ctx, "core.get_torrents_status", args, rencode.Dictionary{})
	if err != nil {
		return nil, err
	}
	d, err := rd.Zip()
	if err != nil {
		return nil, err
	}

	result := map[string]*TorrentStatus{}
	for k, rv := range d {
		v, ok := rv.(rencode.Dictionary)
		if !ok {
			return nil, ErrInvalidDictionaryResponse
		}
		var ts TorrentStatus
		if err := v.ToStruct(&ts, c.excludeTag); err != nil {
			return nil, err
		}
		// on v1 be forward-compatible with v2
		if !c.v2daemon {
			ts.DownloadLocation = ts.SavePath
		}
		result[k] = &ts
	}

	return result, nil
}
