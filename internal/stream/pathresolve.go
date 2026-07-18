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

// FilePath returns the local path of the given file of a torrent. The
// returned path exists at the time of return and is contained within a
// configured local mapping root.
func (r *Resolver) FilePath(ctx context.Context, info qbit.TorrentInfo, file qbit.FileInfo) (string, error) {
	mappings := r.mappings()

	// Reject traversal in the torrent-supplied file name outright; a
	// crafted .torrent could embed "../" path segments.
	if hasDotDot(file.Name) {
		return "", fmt.Errorf("file %q: %w", file.Name, ErrUnsafePath)
	}

	candidate := Remap(mappings, filepath.Clean(filepath.Join(info.SavePath, file.Name)))

	roots := mappingRoots(mappings)
	if !withinAnyRoot(candidate, roots) {
		// Candidate resolved outside every mapping root — refuse to
		// serve it even if it exists on disk (traversal defense).
		return "", fmt.Errorf("file %q of torrent %s resolved outside mapped roots: %w",
			file.Name, info.Hash, ErrUnsafePath)
	}
	if fi, err := os.Stat(candidate); err == nil && !fi.IsDir() {
		return candidate, nil
	}
	return "", fmt.Errorf("file %q of torrent %s not found at %s: %w",
		file.Name, info.Hash, candidate, os.ErrNotExist)
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
