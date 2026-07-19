package stream

import (
	"bytes"
	"embed"
	"fmt"
	"net/http"
	"time"
)

// placeholderFS holds short bundled clips telling the viewer the
// download is still in progress ("X % descargado — vuelve en unos
// minutos"), one per 10% progress bucket. They are served in place of
// the real stream when the file's head/tail pieces are not available
// yet — which happens when the swarm (e.g. a super-seeding initial
// seeder) refuses to deliver pieces in streaming order, so no amount of
// waiting on the real bytes would let playback start.
// Regenerate with scripts/gen-placeholders.sh.
//
//go:embed assets/downloading_*.mp4
var placeholderFS embed.FS

// placeholderModTime is a fixed timestamp so If-Modified-Since behavior
// is stable across restarts.
var placeholderModTime = time.Date(2026, 7, 19, 0, 0, 0, 0, time.UTC)

// placeholderFor returns the bundled clip whose baked-in progress bar
// matches progress (0..1), rounded down to the nearest 10%.
func placeholderFor(progress float64) []byte {
	pct := min(max(int(progress*100)/10*10, 0), 90)
	clip, err := placeholderFS.ReadFile(fmt.Sprintf("assets/downloading_%d.mp4", pct))
	if err != nil {
		// All buckets are embedded; this cannot happen short of a broken
		// build. Fall back to the 0% clip, which must exist.
		clip, _ = placeholderFS.ReadFile("assets/downloading_0.mp4")
	}
	return clip
}

// servePlaceholder streams the "still downloading" clip for the given
// progress with full Range support (players probe with range requests).
func servePlaceholder(w http.ResponseWriter, r *http.Request, progress float64) {
	w.Header().Set("Content-Type", "video/mp4")
	http.ServeContent(w, r, "downloading.mp4", placeholderModTime, bytes.NewReader(placeholderFor(progress)))
}
