//go:build !unix

package diskusage

// Stat is unavailable off unix; callers treat the error as fail-open (the
// disk-usage gate simply does not filter).
func Stat(path string) (used, total int64, err error) {
	return 0, 0, ErrUnsupported
}
