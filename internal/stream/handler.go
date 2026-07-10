package stream

import (
	"context"
	"errors"
	"log/slog"
	"mime"
	"net/http"
	"os"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/javib/seedstrem/internal/qbit"
	"github.com/javib/seedstrem/internal/store"
)

// Settings is the live configuration slice the streaming handler needs.
type Settings struct {
	WaitTimeout time.Duration
	ReadChunk   int64
}

// Handler serves /dl/{token}/{filename} with Range support over files
// qBittorrent may still be downloading.
type Handler struct {
	store    *store.Store
	qb       qbit.Client
	resolver *Resolver
	avail    *Availability
	settings func() Settings
	logger   *slog.Logger
}

// NewHandler creates the streaming handler.
func NewHandler(st *store.Store, qb qbit.Client, resolver *Resolver, avail *Availability, settings func() Settings, logger *slog.Logger) *Handler {
	if logger == nil {
		logger = slog.Default()
	}
	return &Handler{store: st, qb: qb, resolver: resolver, avail: avail, settings: settings, logger: logger}
}

// Router returns the router to mount at /dl.
func (h *Handler) Router() http.Handler {
	r := chi.NewRouter()
	r.Get("/{token}", h.serve)
	r.Head("/{token}", h.serve)
	r.Get("/{token}/*", h.serve)
	r.Head("/{token}/*", h.serve)
	return r
}

func (h *Handler) serve(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	settings := h.settings()

	link, err := h.store.LinkByToken(ctx, chi.URLParam(r, "token"))
	if errors.Is(err, store.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		h.internalError(w, "lookup link", err)
		return
	}

	tor, err := h.store.TorrentByID(ctx, link.TorrentID)
	if err != nil || tor.Error != "" {
		http.NotFound(w, r)
		return
	}

	info, err := h.qb.Torrent(ctx, tor.Hash)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	files, err := h.qb.Files(ctx, tor.Hash)
	if err != nil {
		h.internalError(w, "qbit files", err)
		return
	}
	var file qbit.FileInfo
	found := false
	for _, f := range files {
		if f.Index == link.FileIndex {
			file, found = f, true
			break
		}
	}
	if !found {
		http.NotFound(w, r)
		return
	}

	complete := file.Progress >= 1

	localPath, err := h.resolver.FilePath(ctx, info, file)
	if err != nil {
		// Selected but qBittorrent has not created the file yet.
		h.retryLater(w, "file not on disk yet", err)
		return
	}

	contentType := mime.TypeByExtension(path.Ext(file.Name))
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	w.Header().Set("Content-Type", contentType)

	if complete {
		f, err := os.Open(localPath)
		if err != nil {
			h.internalError(w, "open completed file", err)
			return
		}
		defer f.Close()
		http.ServeContent(w, r, path.Base(file.Name), time.Time{}, f)
		return
	}

	// Streaming while downloading.
	props, err := h.qb.Properties(ctx, tor.Hash)
	if err != nil || props.PieceSize <= 0 {
		h.retryLater(w, "piece size unavailable", err)
		return
	}
	fileOffset := FileOffset(files, file.Index)
	if fileOffset < 0 {
		h.internalError(w, "file offset", errors.New("file index not in torrent"))
		return
	}

	// Before ServeContent writes headers, make sure the first chunk of
	// the requested range is on disk; otherwise answer 503 so players
	// retry rather than hanging on a dead connection.
	start := requestedStart(r.Header.Get("Range"), file.Size)
	chunk := settings.ReadChunk
	if chunk <= 0 {
		chunk = 4 << 20
	}
	end := min(start+chunk-1, file.Size-1)
	first, last := PiecesForRange(fileOffset, props.PieceSize, start, end)
	if err := h.avail.WaitForRange(ctx, tor.Hash, first, last, settings.WaitTimeout); err != nil {
		if errors.Is(err, ErrWaitTimeout) {
			h.retryLater(w, "pieces not available in time", err)
			return
		}
		if ctx.Err() != nil {
			return // client went away
		}
		h.internalError(w, "wait for pieces", err)
		return
	}

	f, err := os.Open(localPath)
	if err != nil {
		h.retryLater(w, "open partial file", err)
		return
	}

	pr := &partialReader{
		ctx:        ctx,
		file:       f,
		size:       file.Size,
		fileOffset: fileOffset,
		pieceSize:  props.PieceSize,
		hash:       tor.Hash,
		chunkSize:  chunk,
		reopen: func() (*os.File, error) {
			freshInfo, err := h.qb.Torrent(ctx, tor.Hash)
			if err != nil {
				return nil, err
			}
			p, err := h.resolver.FilePath(ctx, freshInfo, file)
			if err != nil {
				return nil, err
			}
			return os.Open(p)
		},
		waitFor: func(ctx context.Context, hash string, first, last int) error {
			return h.avail.WaitForRange(ctx, hash, first, last, settings.WaitTimeout)
		},
	}
	defer pr.Close()

	http.ServeContent(w, r, path.Base(file.Name), time.Time{}, pr)
}

// requestedStart extracts the first byte offset a Range header asks
// for; 0 when absent or unparseable (ServeContent does full parsing).
func requestedStart(rangeHeader string, size int64) int64 {
	spec, ok := strings.CutPrefix(rangeHeader, "bytes=")
	if !ok {
		return 0
	}
	// Only the first range of a multi-range request matters here.
	spec, _, _ = strings.Cut(spec, ",")
	first, rest, ok := strings.Cut(spec, "-")
	if !ok {
		return 0
	}
	first = strings.TrimSpace(first)
	if first == "" {
		// Suffix range "bytes=-N": last N bytes.
		if n, err := strconv.ParseInt(strings.TrimSpace(rest), 10, 64); err == nil && n > 0 && n <= size {
			return size - n
		}
		return 0
	}
	if n, err := strconv.ParseInt(first, 10, 64); err == nil && n >= 0 && n < size {
		return n
	}
	return 0
}

func (h *Handler) retryLater(w http.ResponseWriter, msg string, err error) {
	h.logger.Debug("stream retry-later", "reason", msg, "error", err)
	w.Header().Set("Retry-After", "10")
	http.Error(w, "content not available yet, retry shortly", http.StatusServiceUnavailable)
}

func (h *Handler) internalError(w http.ResponseWriter, msg string, err error) {
	h.logger.Error("stream "+msg, "error", err)
	http.Error(w, "internal error", http.StatusInternalServerError)
}
