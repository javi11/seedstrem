package stream

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/javib/seedstrem/internal/config"
	"github.com/javib/seedstrem/internal/qbit"
)

// incompleteExt is appended by qBittorrent when "Append .!qB extension
// to incomplete files" is enabled.
const incompleteExt = ".!qB"

// Resolver locates a torrent file on the local filesystem, translating
// qBittorrent's view of paths through the configured mappings.
type Resolver struct {
	dc       qbit.Client
	mappings func() []config.Mapping
}

// NewResolver creates a Resolver. mappings is fetched per call so
// config changes apply live.
func NewResolver(dc qbit.Client, mappings func() []config.Mapping) *Resolver {
	return &Resolver{dc: dc, mappings: mappings}
}

// Remap translates a qBittorrent-side path to a local path using the longest
// matching prefix mapping. Paths already valid locally pass through when
// no mapping matches.
func Remap(mappings []config.Mapping, remotePath string) string {
	best := -1
	bestLen := -1
	for i, m := range mappings {
		prefix := strings.TrimSuffix(m.Remote, "/")
		if (remotePath == prefix || strings.HasPrefix(remotePath, prefix+"/")) && len(prefix) > bestLen {
			best, bestLen = i, len(prefix)
		}
	}
	if best == -1 {
		return remotePath
	}
	m := mappings[best]
	rest := strings.TrimPrefix(remotePath, strings.TrimSuffix(m.Remote, "/"))
	return strings.TrimSuffix(m.Local, "/") + rest
}

// ErrUnsafePath is returned when a torrent file path escapes every
// configured local mapping root (path-traversal defense).
var ErrUnsafePath = errors.New("resolved path escapes configured mapping root")

// FilePath returns the local path of the given file of a torrent,
// probing completed and in-progress locations (qBittorrent keeps
// still-downloading content under a temp/incomplete folder and/or with a
// .!qB suffix). The returned path exists at the time of return and is
// contained within a configured local mapping root.
func (r *Resolver) FilePath(ctx context.Context, info qbit.TorrentInfo, file qbit.FileInfo) (string, error) {
	mappings := r.mappings()

	// Reject traversal in the torrent-supplied file name outright; a
	// crafted .torrent could embed "../" path segments.
	if hasDotDot(file.Name) {
		return "", fmt.Errorf("file %q: %w", file.Name, ErrUnsafePath)
	}

	var candidates []string
	add := func(p string) {
		if p != "" {
			candidates = append(candidates, Remap(mappings, filepath.Clean(p)))
		}
	}

	// Usual location: save path + file name (name includes the torrent's
	// root folder for multi-file torrents).
	add(filepath.Join(info.SavePath, file.Name))

	// content_path tracks the current on-disk location (temp folder while
	// downloading) and renames: for single-file torrents it IS the file;
	// for multi-file it is the root folder, so re-join the remainder of
	// the file path under it.
	if info.ContentPath != "" {
		if slash := strings.IndexByte(file.Name, '/'); slash >= 0 {
			add(filepath.Join(info.ContentPath, file.Name[slash+1:]))
		} else {
			add(info.ContentPath)
		}
	}

	// Incomplete-downloads temp folder, if enabled.
	if prefs, err := r.dc.AppPreferences(ctx); err == nil && prefs.TempPathEnabled && prefs.TempPath != "" {
		add(filepath.Join(prefs.TempPath, file.Name))
	}

	roots := mappingRoots(mappings)
	for _, c := range candidates {
		if !withinAnyRoot(c, roots) {
			// Candidate resolved outside every mapping root — skip it so
			// a traversal can never be served even if it exists on disk.
			continue
		}
		for _, p := range []string{c, c + incompleteExt} {
			if fi, err := os.Stat(p); err == nil && !fi.IsDir() {
				return p, nil
			}
		}
	}
	return "", fmt.Errorf("file %q of torrent %s not found within a mapped root (checked %v): %w",
		file.Name, info.Hash, candidates, os.ErrNotExist)
}

// hasDotDot reports whether a slash-separated path contains a ".."
// segment.
func hasDotDot(name string) bool {
	for _, seg := range strings.Split(filepath.ToSlash(name), "/") {
		if seg == ".." {
			return true
		}
	}
	return false
}

// mappingRoots returns the cleaned local roots of the mappings. When no
// mappings are configured there is no containment boundary to enforce,
// signalled by a nil slice (withinAnyRoot then allows any path).
func mappingRoots(mappings []config.Mapping) []string {
	roots := make([]string, 0, len(mappings))
	for _, m := range mappings {
		if m.Local != "" {
			roots = append(roots, filepath.Clean(m.Local))
		}
	}
	return roots
}

// withinAnyRoot reports whether cleaned path p is inside one of roots.
// A nil/empty roots slice means "no boundary configured" and permits p.
func withinAnyRoot(p string, roots []string) bool {
	if len(roots) == 0 {
		return true
	}
	for _, root := range roots {
		if p == root || strings.HasPrefix(p, root+string(filepath.Separator)) {
			return true
		}
	}
	return false
}
