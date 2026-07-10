package admin

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/javib/seedstrem/internal/config"
	"github.com/javib/seedstrem/internal/qbit"
	"github.com/javib/seedstrem/internal/qbit/fake"
	"github.com/javib/seedstrem/internal/store"
)

const adminPassword = "test-admin-pw"

type env struct {
	handler http.Handler
	config  *config.Manager
	fake    *fake.Server
	cookie  *http.Cookie
	t       *testing.T
}

func newEnv(t *testing.T) *env {
	t.Helper()
	f := fake.New()
	t.Cleanup(f.Close)

	cfg := config.Default()
	cfg.Server.AdminPassword = adminPassword
	cfg.QBittorrent.URL = f.URL()
	cfg.Storage.Database = filepath.Join(t.TempDir(), "db")
	cm := config.NewManager(cfg, filepath.Join(t.TempDir(), "config.yaml"))

	st, err := store.Open(cfg.Storage.Database)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })

	qb := qbit.NewSwappable(qbit.New(f.URL(), "u", "p"))
	h := New(cm, st, qb, "test", nil)
	return &env{handler: h.Router(), config: cm, fake: f, t: t}
}

func (e *env) login(t *testing.T) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/session", strings.NewReader(`{"password":"`+adminPassword+`"}`))
	w := httptest.NewRecorder()
	e.handler.ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("login failed: %d %s", w.Code, w.Body.String())
	}
	for _, c := range w.Result().Cookies() {
		if c.Name == sessionCookie {
			e.cookie = c
			return
		}
	}
	t.Fatal("no session cookie set")
}

func (e *env) do(t *testing.T, method, target, body string) *httptest.ResponseRecorder {
	t.Helper()
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, target, reader)
	if e.cookie != nil {
		req.AddCookie(e.cookie)
	}
	req.Header.Set("X-Requested-With", "XMLHttpRequest")
	w := httptest.NewRecorder()
	e.handler.ServeHTTP(w, req)
	return w
}

func TestLoginWrongPassword(t *testing.T) {
	e := newEnv(t)
	req := httptest.NewRequest(http.MethodPost, "/session", strings.NewReader(`{"password":"nope"}`))
	w := httptest.NewRecorder()
	e.handler.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d; want 401", w.Code)
	}
}

func TestUnauthenticatedRejected(t *testing.T) {
	e := newEnv(t)
	req := httptest.NewRequest(http.MethodGet, "/config", nil)
	w := httptest.NewRecorder()
	e.handler.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d; want 401", w.Code)
	}
}

func TestCSRFHeaderRequired(t *testing.T) {
	e := newEnv(t)
	e.login(t)

	req := httptest.NewRequest(http.MethodPost, "/config/test-qbit", nil)
	req.AddCookie(e.cookie)
	// No X-Requested-With header.
	w := httptest.NewRecorder()
	e.handler.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("status = %d; want 403", w.Code)
	}
}

func TestSessionExpiry(t *testing.T) {
	if validSession(adminPassword, mintSession(adminPassword, time.Now().Add(-time.Minute)), time.Now()) {
		t.Error("expired session accepted")
	}
	if validSession("other-password", mintSession(adminPassword, time.Now().Add(time.Hour)), time.Now()) {
		t.Error("session valid across password change")
	}
	if !validSession(adminPassword, mintSession(adminPassword, time.Now().Add(time.Hour)), time.Now()) {
		t.Error("fresh session rejected")
	}
	if validSession(adminPassword, "garbage", time.Now()) {
		t.Error("garbage session accepted")
	}
}

func TestGetConfigMasksSecrets(t *testing.T) {
	e := newEnv(t)
	cfg := e.config.Get()
	cfg.QBittorrent.Password = "qbit-secret"
	if err := e.config.Update(cfg); err != nil {
		t.Fatal(err)
	}
	e.login(t)

	w := e.do(t, http.MethodGet, "/config", "")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	body := w.Body.String()
	if strings.Contains(body, "qbit-secret") || strings.Contains(body, adminPassword) {
		t.Errorf("secrets leaked: %s", body)
	}
}

func TestPutConfigKeepsMaskedPassword(t *testing.T) {
	e := newEnv(t)
	cfg := e.config.Get()
	cfg.QBittorrent.Password = "qbit-secret"
	if err := e.config.Update(cfg); err != nil {
		t.Fatal(err)
	}
	e.login(t)

	// Read config, send it back unchanged (password still masked).
	w := e.do(t, http.MethodGet, "/config", "")
	var dto configDTO
	if err := json.Unmarshal(w.Body.Bytes(), &dto); err != nil {
		t.Fatal(err)
	}
	payload, _ := json.Marshal(dto)
	w = e.do(t, http.MethodPut, "/config", string(payload))
	if w.Code != http.StatusOK {
		t.Fatalf("put status = %d body=%s", w.Code, w.Body.String())
	}
	if got := e.config.Get().QBittorrent.Password; got != "qbit-secret" {
		t.Errorf("masked password overwrote stored value: %q", got)
	}

	// Setting a real new password does update.
	dto.QBittorrent.Password = "new-secret"
	payload, _ = json.Marshal(dto)
	if w = e.do(t, http.MethodPut, "/config", string(payload)); w.Code != http.StatusOK {
		t.Fatalf("put status = %d", w.Code)
	}
	if got := e.config.Get().QBittorrent.Password; got != "new-secret" {
		t.Errorf("password not updated: %q", got)
	}
}

func TestPutConfigValidates(t *testing.T) {
	e := newEnv(t)
	e.login(t)

	w := e.do(t, http.MethodGet, "/config", "")
	var dto configDTO
	json.Unmarshal(w.Body.Bytes(), &dto)
	dto.Server.ExternalURL = "not-a-url"
	payload, _ := json.Marshal(dto)

	w = e.do(t, http.MethodPut, "/config", string(payload))
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d; want 400", w.Code)
	}
}

func TestTestQbit(t *testing.T) {
	e := newEnv(t)
	e.login(t)

	w := e.do(t, http.MethodPost, "/config/test-qbit", `{"url":"`+e.fake.URL()+`","username":"u","password":"p"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	var res map[string]any
	json.Unmarshal(w.Body.Bytes(), &res)
	if res["ok"] != true || res["version"] != "v5.0.0" {
		t.Errorf("test-qbit result: %v", res)
	}

	w = e.do(t, http.MethodPost, "/config/test-qbit", `{"url":"http://127.0.0.1:1","username":"u","password":"p"}`)
	json.Unmarshal(w.Body.Bytes(), &res)
	if res["ok"] != false {
		t.Errorf("expected failure for dead qbit: %v", res)
	}
}

func TestStatus(t *testing.T) {
	e := newEnv(t)
	e.login(t)

	w := e.do(t, http.MethodGet, "/status", "")
	if w.Code != http.StatusOK {
		t.Fatalf("status endpoint = %d", w.Code)
	}
	var status map[string]any
	json.Unmarshal(w.Body.Bytes(), &status)
	if status["qbittorrent"].(map[string]any)["connected"] != true {
		t.Errorf("qbit not connected: %v", status)
	}
	manifestURL, _ := status["manifest_url"].(string)
	if !strings.HasSuffix(manifestURL, "/stremio/manifest.json") {
		t.Errorf("status missing manifest_url: %v", status)
	}
}

func TestTorrentsListing(t *testing.T) {
	e := newEnv(t)
	e.login(t)

	// Empty list is fine.
	w := e.do(t, http.MethodGet, "/torrents", "")
	if w.Code != http.StatusOK {
		t.Fatalf("torrents = %d", w.Code)
	}
	var items []map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &items); err != nil {
		t.Fatalf("decode: %v body=%s", err, w.Body.String())
	}
	if len(items) != 0 {
		t.Errorf("expected empty list, got %v", items)
	}
}
