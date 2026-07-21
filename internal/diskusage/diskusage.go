// Package diskusage reports filesystem usage for a local path. It backs
// seedstrem's disk-usage threshold gate, which stops offering new streams
// once the download disk crosses a configured usage percentage.
package diskusage

import "errors"

// ErrUnsupported is returned by Stat on platforms without a statfs
// primitive (everything but unix). seedstrem's deploy target is Linux, so
// this only affects non-unix builds.
var ErrUnsupported = errors.New("disk usage not supported on this platform")
