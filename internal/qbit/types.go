package qbit

import "time"

// TorrentInfo is the subset of qBittorrent torrent state seedstrem uses.
type TorrentInfo struct {
	Hash     string
	Name     string
	State    string  // normalized to the canonical StateXxx constants below
	Progress float64 // 0..1
	Size     int64   // wanted (selected) size
	DlSpeed  int64
	NumSeeds int64
	Uploaded int64   // total bytes uploaded (for ratio tracking)
	Ratio    float64 // upload/download ratio reported by qBittorrent
	SavePath string
	// ContentPath is qBittorrent's current on-disk location of the
	// content: the temp/incomplete folder while downloading, the final
	// path once complete. For single-file torrents it is the file itself.
	// Essential for locating a file that is still downloading.
	ContentPath string
	SeedingTime time.Duration // time spent seeding since the torrent finished
	// Streaming flags as currently in effect in qBittorrent (WebUI
	// fields seq_dl / f_l_piece_prio). Read back so seedstrem can
	// re-assert them when they were dropped or never stuck.
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

// PieceState mirrors qBittorrent's per-piece status (WebUI
// /torrents/pieceStates: 0 = not downloaded, 1 = downloading, 2 = have).
type PieceState int

const (
	PieceMissing     PieceState = 0
	PieceDownloading PieceState = 1
	PieceHave        PieceState = 2
)

// Prefs is the subset of qBittorrent app preferences seedstrem needs to
// locate in-progress files on disk.
type Prefs struct {
	TempPath           string
	TempPathEnabled    bool
	IncompleteFilesExt bool // "Append .!qB extension to incomplete files"
}

// AddOptions controls how a torrent is added.
type AddOptions struct {
	// Category tags the torrent so seedstrem's torrents are
	// distinguishable on a shared qBittorrent instance. Empty falls back
	// to the client's configured category.
	Category           string
	Stopped            bool
	SequentialDownload bool
	FirstLastPiecePrio bool
}

// Canonical torrent states seedstrem reasons about. qBittorrent's many
// native WebUI states (downloading, stalledDL, pausedUP, uploading, …)
// are normalized into these by the client, so DeriveStatus works against
// a small stable vocabulary regardless of backend.
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

// normalizeState maps a raw qBittorrent WebUI state string to one of the
// canonical StateXxx constants. Unknown states fall back to
// StateDownloading (the safe default DeriveStatus also uses).
func normalizeState(raw string) string {
	switch raw {
	case "error", "missingFiles":
		return StateError
	case "uploading", "forcedUP", "stalledUP":
		return StateSeeding
	case "pausedDL", "pausedUP", "stoppedDL", "stoppedUP":
		return StatePaused
	case "queuedDL", "queuedUP":
		return StateQueued
	case "checkingDL", "checkingUP", "checkingResumeData":
		return StateChecking
	case "allocating":
		return StateAllocating
	case "moving":
		return StateMoving
	case "downloading", "forcedDL", "metaDL", "stalledDL":
		return StateDownloading
	default:
		return StateDownloading
	}
}
