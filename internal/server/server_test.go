package server

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"io/fs"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/HopStat/HopStat/internal/config"
	_ "modernc.org/sqlite"
)

// minimal FS for tests — serves an empty index.html
type testFS struct{}

func (testFS) Open(name string) (fs.File, error) {
	if name == "web/dist/index.html" || name == "web/dist/assets/" {
		return nil, fs.ErrNotExist
	}
	return nil, fs.ErrNotExist
}

func newTestServerFS() fs.FS {
	// Create a simple in-memory FS with an index.html
	sub, _ := fs.Sub(&memFS{}, "web/dist")
	return sub
}

type memFS struct{}

func (m *memFS) Open(name string) (fs.File, error) {
	if strings.HasSuffix(name, "index.html") || name == "web/dist/index.html" {
		return &memFile{bytes.NewReader([]byte("<!DOCTYPE html><html><body>test</body></html>"))}, nil
	}
	return nil, fs.ErrNotExist
}

type memFile struct{ *bytes.Reader }

func (m *memFile) Stat() (fs.FileInfo, error) { return nil, fs.ErrNotExist }
func (m *memFile) Close() error               { return nil }

func testServerConfig() *config.Config {
	return &config.Config{
		Server:   config.ServerConfig{Host: "127.0.0.1", Port: 0, Mode: "server", ASNumber: "AS65000", OrgName: "Test"},
		Security: config.SecurityConfig{JWTSecret: "test-secret-that-is-at-least-32-chars", RateLimitPerMin: 100, BruteForceMax: 5, BruteForceBanMin: 15},
		Query:    config.QueryConfig{MaxConcurrent: 10, DefaultTimeoutSec: 30, MTRTimeoutSec: 60, TracerouteTimeoutSec: 30},
		GeoIP:    config.GeoIPConfig{},
	}
}

func setupTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	migrations := []string{
		`CREATE TABLE IF NOT EXISTS users (id INTEGER PRIMARY KEY AUTOINCREMENT, email TEXT NOT NULL UNIQUE, password_hash TEXT NOT NULL, role TEXT DEFAULT 'admin', last_login_at TEXT, created_at TEXT DEFAULT CURRENT_TIMESTAMP)`,
		`CREATE TABLE IF NOT EXISTS nodes (id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT NOT NULL, description TEXT, type TEXT NOT NULL DEFAULT 'standalone', city TEXT NOT NULL DEFAULT '', country TEXT NOT NULL DEFAULT '', lat REAL, lon REAL, credential_id INTEGER, active INTEGER DEFAULT 1, enabled_cmds TEXT, bgp_config TEXT, agent_url TEXT, agent_token TEXT, created_at TEXT DEFAULT CURRENT_TIMESTAMP, updated_at TEXT DEFAULT CURRENT_TIMESTAMP)`,
		`CREATE TABLE IF NOT EXISTS audit_log (id INTEGER PRIMARY KEY AUTOINCREMENT, created_at TEXT DEFAULT CURRENT_TIMESTAMP, source_ip TEXT, user_id INTEGER REFERENCES users(id), node_id INTEGER REFERENCES nodes(id), command TEXT, params TEXT, duration_ms INTEGER DEFAULT 0, success INTEGER DEFAULT 1, error_msg TEXT)`,
		`CREATE TABLE IF NOT EXISTS community_rules (id INTEGER PRIMARY KEY AUTOINCREMENT, community TEXT NOT NULL, severity TEXT DEFAULT 'info', message_i18n TEXT, scope TEXT DEFAULT 'global', active INTEGER DEFAULT 1, created_at TEXT DEFAULT CURRENT_TIMESTAMP, updated_at TEXT DEFAULT CURRENT_TIMESTAMP)`,
	}
	for _, m := range migrations {
		if _, err := db.Exec(m); err != nil {
			t.Fatalf("run migration: %v\nquery: %s", err, m)
		}
	}
	return db
}

func TestNewServer(t *testing.T) {
	db := setupTestDB(t)
	cfg := testServerConfig()

	srv := New(cfg, db, nil, newTestServerFS(), nil, "dev")
	if srv == nil {
		t.Fatal("New() returned nil server")
	}
	if srv.router == nil {
		t.Error("server router is nil")
	}
	if srv.cfg == nil {
		t.Error("server config is nil")
	}
	if srv.db == nil {
		t.Error("server db is nil")
	}
}

func TestServerHealthEndpoint(t *testing.T) {
	db := setupTestDB(t)
	cfg := testServerConfig()
	srv := New(cfg, db, nil, newTestServerFS(), nil, "dev")

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("GET /health status = %d, want %d", w.Code, http.StatusOK)
	}

	var body map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("parse response body: %v", err)
	}
	if body["status"] != "ok" {
		t.Errorf("status = %v, want ok", body["status"])
	}
	if body["mode"] != "server" {
		t.Errorf("mode = %v, want server", body["mode"])
	}
}

func TestServerRoutesRegistered(t *testing.T) {
	db := setupTestDB(t)
	cfg := testServerConfig()
	srv := New(cfg, db, nil, newTestServerFS(), nil, "dev")

	wantRoutes := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/health"},
		{http.MethodGet, "/api/v1/nodes"},
		{http.MethodPost, "/api/v1/query"},
		{http.MethodGet, "/api/v1/myip"},
		{http.MethodPost, "/api/v1/auth/login"},
	}

	routes := srv.router.Routes()
	routeMap := make(map[string]bool)
	for _, r := range routes {
		routeMap[r.Method+" "+r.Path] = true
	}

	for _, want := range wantRoutes {
		key := want.method + " " + want.path
		if !routeMap[key] {
			t.Errorf("missing route: %s %s", want.method, want.path)
		}
	}
}

func TestServerCORSEnabled(t *testing.T) {
	db := setupTestDB(t)
	cfg := testServerConfig()
	srv := New(cfg, db, nil, newTestServerFS(), nil, "dev")

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	origin := w.Header().Get("Access-Control-Allow-Origin")
	if origin != "" {
		t.Errorf("Access-Control-Allow-Origin = %q, want empty (same-origin mode)", origin)
	}

	methods := w.Header().Get("Access-Control-Allow-Methods")
	if methods == "" {
		t.Error("Access-Control-Allow-Methods header is missing")
	}

	headers := w.Header().Get("Access-Control-Allow-Headers")
	if headers == "" {
		t.Error("Access-Control-Allow-Headers header is missing")
	}

	// With CORS(nil) — same-origin mode — no credentials header
	credentials := w.Header().Get("Access-Control-Allow-Credentials")
	if credentials != "" {
		t.Errorf("Access-Control-Allow-Credentials = %q, want empty (same-origin mode)", credentials)
	}
}

func TestServerCORSPreflight(t *testing.T) {
	db := setupTestDB(t)
	cfg := testServerConfig()
	srv := New(cfg, db, nil, newTestServerFS(), nil, "dev")

	req := httptest.NewRequest(http.MethodOptions, "/api/v1/nodes", nil)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("OPTIONS preflight status = %d, want %d", w.Code, http.StatusNoContent)
	}
}

func TestServerMyIP(t *testing.T) {
	db := setupTestDB(t)
	cfg := testServerConfig()
	srv := New(cfg, db, nil, newTestServerFS(), nil, "dev")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/myip", nil)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("GET /api/v1/myip status = %d, want %d", w.Code, http.StatusOK)
	}

	var body map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("parse response body: %v", err)
	}

	data, ok := body["data"].(map[string]interface{})
	if !ok {
		t.Fatal("response data field is not an object")
	}

	ip, ok := data["ip"].(string)
	if !ok || ip == "" {
		t.Errorf("data.ip = %v, want a non-empty string", data["ip"])
	}
}

func TestServerRunAndShutdown(t *testing.T) {
	db := setupTestDB(t)
	cfg := testServerConfig()

	// Get a free port by listening on :0
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("get free port: %v", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	listener.Close()

	cfg.Server.Port = port
	srv := New(cfg, db, nil, newTestServerFS(), nil, "dev")

	ctx, cancel := context.WithCancel(context.Background())

	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.Run(ctx)
	}()

	// Wait for server to be ready by polling the health endpoint
	client := &http.Client{Timeout: 2 * time.Second}
	url := "http://127.0.0.1:" + strings.TrimLeft(listener.Addr().String(), "127.0.0.1:")
	url = "http://127.0.0.1:" + itoa(port) + "/health"

	var lastErr error
	for i := 0; i < 50; i++ {
		resp, err := client.Get(url)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				lastErr = nil
				break
			}
		}
		lastErr = err
		time.Sleep(20 * time.Millisecond)
	}
	if lastErr != nil {
		cancel()
		t.Fatalf("server never became ready: %v", lastErr)
	}

	// Shut down
	cancel()

	select {
	case err := <-errCh:
		if err != nil {
			t.Errorf("Run() returned error: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Run() did not return within 10 seconds after context cancellation")
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	digits := make([]byte, 0, 10)
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}
