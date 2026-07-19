package stream

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"
)

// delivered must count bytes actually read, not the final file position.
// http.ServeContent seeks to the range start before copying, so for a
// tail-range request offset ends near the file size while only a few
// bytes were sent — the serve-finished log must report the latter.
func TestPartialReaderDeliveredCountsBytesReadNotOffset(t *testing.T) {
	path := filepath.Join(t.TempDir(), "movie.mkv")
	content := make([]byte, 100)
	for i := range content {
		content[i] = byte(i)
	}
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}

	pr := &partialReader{
		ctx:       context.Background(),
		file:      f,
		size:      100,
		chunkSize: 64,
		complete:  true, // skip piece waits
	}
	defer pr.Close()

	// Simulate a tail range request: ServeContent seeks to the range
	// start, then copies to EOF.
	if _, err := pr.Seek(90, io.SeekStart); err != nil {
		t.Fatalf("seek: %v", err)
	}
	n, err := io.Copy(io.Discard, io.Reader(pr))
	if err != nil {
		t.Fatalf("copy: %v", err)
	}
	if n != 10 {
		t.Fatalf("copied %d bytes, want 10", n)
	}

	if pr.delivered != 10 {
		t.Errorf("delivered = %d, want 10 (offset is %d — delivered must not track position)", pr.delivered, pr.offset)
	}
}
