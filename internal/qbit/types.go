package qbit

// TorrentInfo is the subset of qBittorrent torrent state seedstrem uses.
type TorrentInfo struct {
	Hash        string
	Name        string
	State       string
	Progress    float64 // 0..1, over wanted (selected) files
	Size        int64   // wanted size
	TotalSize   int64   // full torrent size
	DlSpeed     int64
	NumSeeds    int64
	SavePath    string
	ContentPath string
	AmountLeft  int64
}

// FileInfo describes one file inside a torrent.
type FileInfo struct {
	Index      int
	Name       string // path relative to save path, includes root folder for multi-file torrents
	Size       int64
	Progress   float64
	Priority   int // 0 = do not download
	PieceRange [2]int
}

// Properties holds per-torrent generic properties.
type Properties struct {
	PieceSize int64
	PiecesNum int
	SavePath  string
}

// PieceState mirrors qBittorrent piece states.
type PieceState int

const (
	PieceMissing     PieceState = 0
	PieceDownloading PieceState = 1
	PieceHave        PieceState = 2
)

// Prefs is the subset of qBittorrent app preferences we care about.
type Prefs struct {
	TempPath           string
	TempPathEnabled    bool
	IncompleteFilesExt bool // "Append .!qB extension to incomplete files"
}

// AddOptions controls how a torrent is added.
type AddOptions struct {
	Category           string
	Stopped            bool
	SequentialDownload bool
	FirstLastPiecePrio bool
}

// Known qBittorrent state strings (WebUI API).
const (
	StateError              = "error"
	StateMissingFiles       = "missingFiles"
	StateUploading          = "uploading"
	StateStoppedUP          = "stoppedUP" // qbit 5.x; 4.x reports pausedUP
	StatePausedUP           = "pausedUP"
	StateQueuedUP           = "queuedUP"
	StateStalledUP          = "stalledUP"
	StateCheckingUP         = "checkingUP"
	StateForcedUP           = "forcedUP"
	StateAllocating         = "allocating"
	StateDownloading        = "downloading"
	StateMetaDL             = "metaDL"
	StateStoppedDL          = "stoppedDL" // qbit 5.x; 4.x reports pausedDL
	StatePausedDL           = "pausedDL"
	StateQueuedDL           = "queuedDL"
	StateStalledDL          = "stalledDL"
	StateCheckingDL         = "checkingDL"
	StateForcedDL           = "forcedDL"
	StateCheckingResumeData = "checkingResumeData"
	StateMoving             = "moving"
	StateUnknown            = "unknown"
)
