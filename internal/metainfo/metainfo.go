// Package metainfo extracts the v1 infohash (and display name) from
// magnet URIs and raw .torrent files. The bencode scanner is minimal on
// purpose: it only needs to locate the raw bytes of the "info" dict to
// SHA-1 them, plus read the name field.
package metainfo

import (
	"crypto/sha1"
	"encoding/base32"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"strings"
)

// ErrInvalid is returned for unparseable magnets or torrent files.
var ErrInvalid = errors.New("invalid torrent metadata")

// FromMagnet extracts the infohash (lowercase hex) and display name
// from a magnet URI.
func FromMagnet(magnet string) (hash, name string, err error) {
	u, err := url.Parse(magnet)
	if err != nil || u.Scheme != "magnet" {
		return "", "", fmt.Errorf("%w: not a magnet URI", ErrInvalid)
	}
	q := u.Query()
	name = q.Get("dn")
	for _, xt := range q["xt"] {
		raw, ok := strings.CutPrefix(xt, "urn:btih:")
		if !ok {
			continue
		}
		switch len(raw) {
		case 40: // hex
			if _, err := hex.DecodeString(raw); err != nil {
				return "", "", fmt.Errorf("%w: bad hex infohash", ErrInvalid)
			}
			return strings.ToLower(raw), name, nil
		case 32: // base32
			decoded, err := base32.StdEncoding.DecodeString(strings.ToUpper(raw))
			if err != nil || len(decoded) != 20 {
				return "", "", fmt.Errorf("%w: bad base32 infohash", ErrInvalid)
			}
			return hex.EncodeToString(decoded), name, nil
		}
	}
	return "", "", fmt.Errorf("%w: no btih hash in magnet", ErrInvalid)
}

// FromTorrent extracts the v1 infohash (lowercase hex), name, and
// announce trackers from a raw .torrent file. Trackers come from the
// top-level "announce" and "announce-list" keys, deduplicated in order;
// they are essential for private torrents, whose peers are only
// discoverable via the tracker (DHT/PEX are typically disabled).
func FromTorrent(raw []byte) (hash, name string, trackers []string, err error) {
	d := &decoder{data: raw}
	value, err := d.decode()
	if err != nil {
		return "", "", nil, fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	root, ok := value.(dict)
	if !ok {
		return "", "", nil, fmt.Errorf("%w: root is not a dict", ErrInvalid)
	}
	info, ok := root.values["info"]
	if !ok {
		return "", "", nil, fmt.Errorf("%w: missing info dict", ErrInvalid)
	}
	infoDict, ok := info.(dict)
	if !ok {
		return "", "", nil, fmt.Errorf("%w: info is not a dict", ErrInvalid)
	}

	sum := sha1.Sum(raw[infoDict.start:infoDict.end])
	if n, ok := infoDict.values["name"].(string); ok {
		name = n
	}
	return hex.EncodeToString(sum[:]), name, announceTrackers(root), nil
}

// announceTrackers collects tracker URLs from the "announce" and
// "announce-list" keys, preserving order and dropping duplicates/blanks.
func announceTrackers(root dict) []string {
	var out []string
	seen := map[string]bool{}
	add := func(v any) {
		s, ok := v.(string)
		if !ok || s == "" || seen[s] {
			return
		}
		seen[s] = true
		out = append(out, s)
	}
	// announce-list is a list of tiers, each a list of tracker strings.
	if tiers, ok := root.values["announce-list"].([]any); ok {
		for _, tier := range tiers {
			if urls, ok := tier.([]any); ok {
				for _, u := range urls {
					add(u)
				}
			}
		}
	}
	add(root.values["announce"])
	return out
}

// dict is a decoded bencode dictionary plus the byte range it occupies.
type dict struct {
	values     map[string]any
	start, end int
}

type decoder struct {
	data []byte
	pos  int
}

func (d *decoder) decode() (any, error) {
	if d.pos >= len(d.data) {
		return nil, errors.New("unexpected end of data")
	}
	switch c := d.data[d.pos]; {
	case c == 'd':
		return d.decodeDict()
	case c == 'l':
		return d.decodeList()
	case c == 'i':
		return d.decodeInt()
	case c >= '0' && c <= '9':
		return d.decodeString()
	default:
		return nil, fmt.Errorf("unexpected byte %q at %d", c, d.pos)
	}
}

func (d *decoder) decodeDict() (dict, error) {
	start := d.pos
	d.pos++ // 'd'
	values := map[string]any{}
	for {
		if d.pos >= len(d.data) {
			return dict{}, errors.New("unterminated dict")
		}
		if d.data[d.pos] == 'e' {
			d.pos++
			return dict{values: values, start: start, end: d.pos}, nil
		}
		key, err := d.decodeString()
		if err != nil {
			return dict{}, err
		}
		val, err := d.decode()
		if err != nil {
			return dict{}, err
		}
		values[key] = val
	}
}

func (d *decoder) decodeList() ([]any, error) {
	d.pos++ // 'l'
	var items []any
	for {
		if d.pos >= len(d.data) {
			return nil, errors.New("unterminated list")
		}
		if d.data[d.pos] == 'e' {
			d.pos++
			return items, nil
		}
		item, err := d.decode()
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
}

func (d *decoder) decodeInt() (int64, error) {
	d.pos++ // 'i'
	end := d.pos
	for end < len(d.data) && d.data[end] != 'e' {
		end++
	}
	if end >= len(d.data) {
		return 0, errors.New("unterminated integer")
	}
	var n int64
	var neg bool
	s := d.data[d.pos:end]
	if len(s) == 0 {
		return 0, errors.New("empty integer")
	}
	if s[0] == '-' {
		neg = true
		s = s[1:]
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, fmt.Errorf("bad integer byte %q", c)
		}
		n = n*10 + int64(c-'0')
	}
	if neg {
		n = -n
	}
	d.pos = end + 1
	return n, nil
}

func (d *decoder) decodeString() (string, error) {
	colon := d.pos
	for colon < len(d.data) && d.data[colon] != ':' {
		colon++
	}
	if colon >= len(d.data) {
		return "", errors.New("unterminated string length")
	}
	var length int
	for _, c := range d.data[d.pos:colon] {
		if c < '0' || c > '9' {
			return "", fmt.Errorf("bad string length byte %q", c)
		}
		length = length*10 + int(c-'0')
		if length > len(d.data) {
			return "", errors.New("string length exceeds data")
		}
	}
	start := colon + 1
	if start+length > len(d.data) {
		return "", errors.New("string exceeds data")
	}
	d.pos = start + length
	return string(d.data[start : start+length]), nil
}
