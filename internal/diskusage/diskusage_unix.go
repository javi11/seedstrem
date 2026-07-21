//go:build unix

package diskusage

import (
	"fmt"
	"syscall"
)

// Stat returns the used and total bytes of the filesystem containing path.
// used is total minus the space available to an unprivileged user, so it
// reflects what a new download would actually have to fit into.
func Stat(path string) (used, total int64, err error) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil {
		return 0, 0, fmt.Errorf("statfs %s: %w", path, err)
	}
	// Bsize is int64 on Linux and uint32 on darwin; widen through uint64.
	bsize := uint64(st.Bsize)
	totalBytes := st.Blocks * bsize
	availBytes := st.Bavail * bsize
	usedBytes := totalBytes - availBytes
	return int64(usedBytes), int64(totalBytes), nil
}
