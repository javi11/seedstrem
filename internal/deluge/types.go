package deluge

// TorrentInfo is the subset of Deluge torrent state seedstrem uses.
type TorrentInfo struct {
	Hash     string
	Name     string
	State    string
	Progress float64 // 0..1, converted from Deluge's 0..100 percentage
	Size     int64   // torrent size
	DlSpeed  int64
	NumSeeds int64
	SavePath string
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

// PieceState mirrors Deluge's per-piece status (see
// third_party/go-deluge/deluge_extra.go for the raw 0-3 mapping).
type PieceState int

const (
	PieceMissing     PieceState = 0
	PieceDownloading PieceState = 1
	PieceHave        PieceState = 2
)

// AddOptions controls how a torrent is added.
type AddOptions struct {
	Stopped            bool
	SequentialDownload bool
	FirstLastPiecePrio bool
}

// Deluge's torrent state strings (see deluge/common.py), plus the
// special "Active" state some queries can return.
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
