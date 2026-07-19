package downloader

import "time"

// TorrentInfo is the subset of download-client torrent state seedstrem uses.
type TorrentInfo struct {
	Hash     string
	Name     string
	State    string  // normalized to the canonical StateXxx constants below
	Progress float64 // 0..1
	Size     int64   // wanted (selected) size
	DlSpeed  int64
	NumSeeds int64
	Uploaded int64   // total bytes uploaded (for ratio tracking)
	Ratio    float64 // upload/download ratio reported by the client
	SavePath string
	// ContentPath is the client's current on-disk location of the
	// content: the temp/incomplete folder while downloading, the final
	// path once complete. For single-file torrents it is the file itself.
	// Backends without the concept (Deluge) leave it empty; resolution
	// then relies on SavePath + file name.
	ContentPath string
	SeedingTime time.Duration // time spent seeding since the torrent finished
	// Streaming flags as currently in effect in the client. Read back so
	// seedstrem can re-assert them when they were dropped or never stuck.
	// Best-effort: backends that cannot read them back report false.
	SequentialDownload bool
	FirstLastPiecePrio bool
}

// FileInfo describes one file inside a torrent.
type FileInfo struct {
	Index    int
	Name     string // path relative to SavePath
	Size     int64
	Progress float64 // 0..1
	Priority int     // 0 = do not download, libtorrent 0-7 scale
}

// Properties holds per-torrent generic properties.
type Properties struct {
	PieceSize int64
	PiecesNum int
	SavePath  string
}

// PieceState is the per-piece download status (0 = not downloaded,
// 1 = downloading, 2 = have), matching qBittorrent's pieceStates wire
// values; other backends map into it.
type PieceState int

const (
	PieceMissing     PieceState = 0
	PieceDownloading PieceState = 1
	PieceHave        PieceState = 2
)

// IncompleteHints describes where a backend keeps in-progress downloads.
type IncompleteHints struct {
	// TempDir is a separate directory holding files while they download
	// ("" = files are written in place).
	TempDir string
	// IncompleteExt is an extension appended to in-progress files, e.g.
	// qBittorrent's ".!qB" ("" = none).
	IncompleteExt string
}

// AddOptions controls how a torrent is added.
type AddOptions struct {
	// Category tags the torrent so seedstrem's torrents are
	// distinguishable on a shared client instance (a label on Deluge).
	// Empty falls back to the client's configured category.
	Category           string
	Stopped            bool
	SequentialDownload bool
	FirstLastPiecePrio bool
}

// Canonical torrent states seedstrem reasons about. Each backend's many
// native states are normalized into these by its client implementation,
// so DeriveStatus works against a small stable vocabulary regardless of
// backend.
const (
	StateActive      = "Active"
	StateAllocating  = "Allocating"
	StateChecking    = "Checking"
	StateDownloading = "Downloading"
	StateSeeding     = "Seeding"
	StatePaused      = "Paused"
	StateError       = "Error"
	StateQueued      = "Queued"
	StateMoving      = "Moving"
)
