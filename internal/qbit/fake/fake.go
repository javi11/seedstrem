// Package fake provides an in-memory fake of the qBittorrent WebUI API
// for integration tests. It implements the endpoints used by the
// autobrr/go-qbittorrent client so tests exercise the real adapter.
package fake

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
)

// File is one file of a fake torrent.
type File struct {
	Name     string
	Size     int64
	Progress float64
	Priority int
}

// Torrent is the mutable state of one fake torrent. Tests adjust fields
// directly via Server.Update.
type Torrent struct {
	Hash        string
	Name        string
	State       string
	Category    string
	Progress    float64
	DlSpeed     int64
	NumSeeds    int64
	SavePath    string
	ContentPath string
	Files       []File
	PieceSize   int64
	PieceStates []int

	SequentialDownload bool
	FirstLastPiecePrio bool
}

// Prefs mirrors the app preferences served by the fake.
type Prefs struct {
	TempPath           string `json:"temp_path"`
	TempPathEnabled    bool   `json:"temp_path_enabled"`
	IncompleteFilesExt bool   `json:"incomplete_files_ext"`
}

// Server is a fake qBittorrent WebUI.
type Server struct {
	mu       sync.Mutex
	torrents map[string]*Torrent
	prefs    Prefs
	calls    []string
	ts       *httptest.Server
}

// New starts a fake qBittorrent WebUI server. Callers must Close it.
func New() *Server {
	s := &Server{torrents: map[string]*Torrent{}}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v2/auth/login", s.login)
	mux.HandleFunc("GET /api/v2/app/webapiVersion", text("2.11.2"))
	mux.HandleFunc("GET /api/v2/app/version", text("v5.0.0"))
	mux.HandleFunc("GET /api/v2/app/preferences", s.preferences)
	mux.HandleFunc("POST /api/v2/torrents/add", s.add)
	mux.HandleFunc("GET /api/v2/torrents/info", s.info)
	mux.HandleFunc("GET /api/v2/torrents/files", s.files)
	mux.HandleFunc("GET /api/v2/torrents/properties", s.properties)
	mux.HandleFunc("GET /api/v2/torrents/pieceStates", s.pieceStates)
	mux.HandleFunc("POST /api/v2/torrents/filePrio", s.filePrio)
	mux.HandleFunc("POST /api/v2/torrents/start", s.start)
	mux.HandleFunc("POST /api/v2/torrents/resume", s.start)
	mux.HandleFunc("POST /api/v2/torrents/delete", s.delete)
	s.ts = httptest.NewServer(mux)
	return s
}

// URL returns the base URL of the fake WebUI.
func (s *Server) URL() string { return s.ts.URL }

// Close shuts the fake server down.
func (s *Server) Close() { s.ts.Close() }

// Put inserts or replaces a torrent.
func (s *Server) Put(t *Torrent) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.torrents[strings.ToLower(t.Hash)] = t
}

// Get returns a copy of the torrent, or nil if absent.
func (s *Server) Get(hash string) *Torrent {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.torrents[strings.ToLower(hash)]
	if !ok {
		return nil
	}
	cp := *t
	cp.Files = append([]File(nil), t.Files...)
	cp.PieceStates = append([]int(nil), t.PieceStates...)
	return &cp
}

// Update mutates a torrent under the lock.
func (s *Server) Update(hash string, fn func(*Torrent)) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.torrents[strings.ToLower(hash)]
	if !ok {
		return false
	}
	fn(t)
	return true
}

// Remove deletes a torrent.
func (s *Server) Remove(hash string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.torrents, strings.ToLower(hash))
}

// SetPrefs sets the app preferences the fake serves.
func (s *Server) SetPrefs(p Prefs) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.prefs = p
}

// Calls returns the recorded mutating API calls, oldest first.
func (s *Server) Calls() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.calls...)
}

func (s *Server) record(format string, args ...any) {
	s.calls = append(s.calls, fmt.Sprintf(format, args...))
}

func text(body string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		io.WriteString(w, body)
	}
}

func (s *Server) login(w http.ResponseWriter, _ *http.Request) {
	http.SetCookie(w, &http.Cookie{Name: "SID", Value: "fake-session"})
	io.WriteString(w, "Ok.")
}

func (s *Server) preferences(w http.ResponseWriter, _ *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	json.NewEncoder(w).Encode(s.prefs)
}

// magnetHash extracts the btih infohash from a magnet URI.
func magnetHash(magnet string) string {
	u, err := url.Parse(magnet)
	if err != nil || u.Scheme != "magnet" {
		return ""
	}
	for _, xt := range u.Query()["xt"] {
		if h, ok := strings.CutPrefix(xt, "urn:btih:"); ok {
			return strings.ToLower(h)
		}
	}
	return ""
}

func (s *Server) add(w http.ResponseWriter, r *http.Request) {
	// The client sends form-urlencoded for URL adds and multipart for
	// raw torrent files.
	contentType := r.Header.Get("Content-Type")
	if strings.HasPrefix(contentType, "multipart/") {
		if err := r.ParseMultipartForm(8 << 20); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	} else if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	stopped := r.FormValue("stopped") == "true" || r.FormValue("paused") == "true"
	state := "metaDL"
	if stopped {
		state = "stoppedDL"
	}

	if magnet := r.FormValue("urls"); magnet != "" {
		hash := magnetHash(magnet)
		if hash == "" {
			http.Error(w, "invalid magnet", http.StatusUnsupportedMediaType)
			return
		}
		s.record("add magnet=%s category=%s stopped=%v seq=%v flp=%v",
			hash, r.FormValue("category"), stopped,
			r.FormValue("sequentialDownload") == "true", r.FormValue("firstLastPiecePrio") == "true")
		if _, exists := s.torrents[hash]; !exists {
			s.torrents[hash] = &Torrent{
				Hash:               hash,
				State:              state,
				Category:           r.FormValue("category"),
				SequentialDownload: r.FormValue("sequentialDownload") == "true",
				FirstLastPiecePrio: r.FormValue("firstLastPiecePrio") == "true",
			}
		}
		io.WriteString(w, "Ok.")
		return
	}

	// Raw torrent upload: tests preload the torrent via Put keyed by the
	// hash they expect; the fake only records the call.
	s.record("add file category=%s stopped=%v", r.FormValue("category"), stopped)
	io.WriteString(w, "Ok.")
}

func (s *Server) info(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()

	category, hasCategory := r.URL.Query()["category"]
	hashesParam := r.URL.Query().Get("hashes")
	var wanted map[string]bool
	if hashesParam != "" {
		wanted = map[string]bool{}
		for _, h := range strings.Split(hashesParam, "|") {
			wanted[strings.ToLower(h)] = true
		}
	}

	type torrentJSON struct {
		Hash        string  `json:"hash"`
		Name        string  `json:"name"`
		State       string  `json:"state"`
		Category    string  `json:"category"`
		Progress    float64 `json:"progress"`
		Size        int64   `json:"size"`
		TotalSize   int64   `json:"total_size"`
		DlSpeed     int64   `json:"dlspeed"`
		NumSeeds    int64   `json:"num_seeds"`
		SavePath    string  `json:"save_path"`
		ContentPath string  `json:"content_path"`
	}

	out := []torrentJSON{}
	for _, t := range s.torrents {
		if wanted != nil && !wanted[strings.ToLower(t.Hash)] {
			continue
		}
		if hasCategory && len(category) > 0 && category[0] != "" && t.Category != category[0] {
			continue
		}
		var size, total int64
		for _, f := range t.Files {
			total += f.Size
			if f.Priority != 0 {
				size += f.Size
			}
		}
		if size == 0 {
			size = total
		}
		out = append(out, torrentJSON{
			Hash: t.Hash, Name: t.Name, State: t.State, Category: t.Category,
			Progress: t.Progress, Size: size, TotalSize: total,
			DlSpeed: t.DlSpeed, NumSeeds: t.NumSeeds,
			SavePath: t.SavePath, ContentPath: t.ContentPath,
		})
	}
	json.NewEncoder(w).Encode(out)
}

func (s *Server) lookup(w http.ResponseWriter, r *http.Request) *Torrent {
	hash := strings.ToLower(r.URL.Query().Get("hash"))
	t, ok := s.torrents[hash]
	if !ok {
		http.NotFound(w, r)
		return nil
	}
	return t
}

func (s *Server) files(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t := s.lookup(w, r)
	if t == nil {
		return
	}

	type fileJSON struct {
		Index      int     `json:"index"`
		Name       string  `json:"name"`
		Size       int64   `json:"size"`
		Progress   float64 `json:"progress"`
		Priority   int     `json:"priority"`
		PieceRange []int   `json:"piece_range"`
	}
	out := make([]fileJSON, 0, len(t.Files))
	var offset int64
	for i, f := range t.Files {
		fj := fileJSON{Index: i, Name: f.Name, Size: f.Size, Progress: f.Progress, Priority: f.Priority}
		if t.PieceSize > 0 {
			first := int(offset / t.PieceSize)
			last := first
			if f.Size > 0 {
				last = int((offset + f.Size - 1) / t.PieceSize)
			}
			fj.PieceRange = []int{first, last}
		}
		offset += f.Size
		out = append(out, fj)
	}
	json.NewEncoder(w).Encode(out)
}

func (s *Server) properties(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t := s.lookup(w, r)
	if t == nil {
		return
	}
	json.NewEncoder(w).Encode(map[string]any{
		"piece_size": t.PieceSize,
		"pieces_num": len(t.PieceStates),
		"save_path":  t.SavePath,
	})
}

func (s *Server) pieceStates(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t := s.lookup(w, r)
	if t == nil {
		return
	}
	states := t.PieceStates
	if states == nil {
		states = []int{}
	}
	json.NewEncoder(w).Encode(states)
}

func (s *Server) filePrio(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	hash := strings.ToLower(r.FormValue("hash"))
	t, ok := s.torrents[hash]
	if !ok {
		http.NotFound(w, r)
		return
	}
	priority, err := strconv.Atoi(r.FormValue("priority"))
	if err != nil {
		http.Error(w, "bad priority", http.StatusBadRequest)
		return
	}
	ids := r.FormValue("id")
	s.record("filePrio hash=%s ids=%s priority=%d", hash, ids, priority)
	for _, idStr := range strings.Split(ids, "|") {
		id, err := strconv.Atoi(idStr)
		if err != nil || id < 0 || id >= len(t.Files) {
			http.Error(w, "bad file id", http.StatusConflict)
			return
		}
		t.Files[id].Priority = priority
	}
}

func (s *Server) start(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, hash := range strings.Split(r.FormValue("hashes"), "|") {
		hash = strings.ToLower(hash)
		s.record("start hash=%s", hash)
		if t, ok := s.torrents[hash]; ok {
			if t.State == "stoppedDL" || t.State == "pausedDL" {
				t.State = "downloading"
			}
		}
	}
}

func (s *Server) delete(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	deleteFiles := r.FormValue("deleteFiles") == "true"
	for _, hash := range strings.Split(r.FormValue("hashes"), "|") {
		hash = strings.ToLower(hash)
		s.record("delete hash=%s deleteFiles=%v", hash, deleteFiles)
		delete(s.torrents, hash)
	}
}
