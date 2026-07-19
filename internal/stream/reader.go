package stream

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"time"
)

// partialReader is an io.ReadSeeker over a file that qBittorrent may
// still be writing. Each Read first waits for the pieces backing the
// requested byte range, in chunks of at most chunkSize so a large
// http.ServeContent copy only ever waits for the next few pieces.
type partialReader struct {
	ctx    context.Context
	file   *os.File
	reopen func() (*os.File, error) // re-resolve path (file moves on completion)

	size       int64 // final size of the file
	offset     int64
	delivered  int64 // total bytes read out (offset moves on Seek, this doesn't)
	fileOffset int64 // absolute offset of this file in torrent piece space
	pieceSize  int64
	hash       string
	waitFor    waitFunc
	chunkSize  int64
	complete   bool // torrent already finished: skip piece checks

	logger    *slog.Logger
	firstRead bool // instrumentation: first Read of this response not yet logged
}

type waitFunc func(ctx context.Context, hash string, first, last int) error

func (pr *partialReader) Read(p []byte) (int, error) {
	if pr.offset >= pr.size {
		return 0, io.EOF
	}
	n := int64(len(p))
	if n > pr.chunkSize {
		n = pr.chunkSize
	}
	if remaining := pr.size - pr.offset; n > remaining {
		n = remaining
	}

	if !pr.complete {
		first, last := PiecesForRange(pr.fileOffset, pr.pieceSize, pr.offset, pr.offset+n-1)
		waitStart := time.Now()
		if pr.logger != nil && !pr.firstRead {
			pr.firstRead = true
			pr.logger.Debug("stream: first read waiting for head pieces",
				"hash", pr.hash, "offset", pr.offset, "pieces", [2]int{first, last})
		}
		if err := pr.waitFor(pr.ctx, pr.hash, first, last); err != nil {
			if pr.logger != nil {
				pr.logger.Debug("stream: piece wait failed",
					"hash", pr.hash, "offset", pr.offset, "pieces", [2]int{first, last},
					"waited", time.Since(waitStart).Round(time.Millisecond), "error", err)
			}
			return 0, fmt.Errorf("waiting for pieces [%d,%d]: %w", first, last, err)
		}
		if pr.logger != nil && time.Since(waitStart) > 250*time.Millisecond {
			pr.logger.Debug("stream: piece wait satisfied",
				"hash", pr.hash, "offset", pr.offset, "pieces", [2]int{first, last},
				"waited", time.Since(waitStart).Round(time.Millisecond))
		}
	}

	read, err := pr.file.ReadAt(p[:n], pr.offset)
	if err != nil && shouldReopen(err) && pr.reopen != nil {
		// qBittorrent may rename/move the file when it completes (
		// strip or temp-dir move). Reopen at the new location and retry.
		if f, openErr := pr.reopen(); openErr == nil {
			pr.file.Close()
			pr.file = f
			read, err = pr.file.ReadAt(p[:n], pr.offset)
		}
	}
	pr.offset += int64(read)
	pr.delivered += int64(read)
	if err == io.EOF && read > 0 {
		err = nil // partial tail read of a still-growing file
	}
	return read, err
}

func shouldReopen(err error) bool {
	return errors.Is(err, os.ErrNotExist) || errors.Is(err, io.EOF) || errors.Is(err, os.ErrClosed)
}

func (pr *partialReader) Seek(offset int64, whence int) (int64, error) {
	var target int64
	switch whence {
	case io.SeekStart:
		target = offset
	case io.SeekCurrent:
		target = pr.offset + offset
	case io.SeekEnd:
		target = pr.size + offset
	default:
		return 0, fmt.Errorf("bad whence %d", whence)
	}
	if target < 0 {
		return 0, errors.New("negative seek position")
	}
	pr.offset = target
	return target, nil
}

func (pr *partialReader) Close() error {
	return pr.file.Close()
}
