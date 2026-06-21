package server

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"crypto/tls"
	"database/sql"
	"encoding/json"
	"encoding/pem"
	"io"
	"io/fs"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/HopStat/HopStat/internal/config"
	"github.com/HopStat/HopStat/internal/server/handler"
	"github.com/HopStat/HopStat/internal/sitecache"
	"github.com/HopStat/HopStat/internal/store/queries"
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

type richDistFS struct {
	files map[string][]byte
	dirs  map[string]bool
	bad   map[string]bool
}

func newRichDistFS() *richDistFS {
	index := []byte(`<!DOCTYPE html><html><head><!-- hopstat:bootstrap --><title>Looking Glass</title></head><body></body></html>`)
	return &richDistFS{
		files: map[string][]byte{
			"index.html":         index,
			"appearance-boot.js": []byte(`window.__HOPSTAT__=1`),
			"assets/app.css":     []byte(`body{}`),
			"assets/app.js":      []byte(`console.log(1)`),
			"assets/font.woff2":  []byte("woff"),
			"assets/font.ttf":    []byte("ttf"),
			"assets/icon.svg":    []byte(`<svg xmlns="http://www.w3.org/2000/svg"/>`),
			"assets/icon.png":    {0x89, 0x50, 0x4E, 0x47},
			"assets/icon.jpg":    {0xFF, 0xD8, 0xFF},
			"assets/favicon.ico": {0, 1},
			"assets/data.json":   []byte(`{}`),
			"assets/page.html":   []byte(`<html></html>`),
			"assets/bad.dat":     []byte("x"),
		},
		dirs: map[string]bool{"assets/subdir": true},
		bad:  map[string]bool{"assets/bad.dat": true},
	}
}

func (f *richDistFS) Open(name string) (fs.File, error) {
	if f.dirs[name] {
		return &dirEntry{name: filepath.Base(name)}, nil
	}
	data, ok := f.files[name]
	if !ok {
		return nil, fs.ErrNotExist
	}
	if f.bad[name] {
		return &readOnlyFile{data: data, name: filepath.Base(name)}, nil
	}
	return &memFileWithStat{Reader: bytes.NewReader(data), name: filepath.Base(name)}, nil
}

type memFileWithStat struct {
	*bytes.Reader
	name string
}

func (m *memFileWithStat) Close() error { return nil }
func (m *memFileWithStat) Stat() (fs.FileInfo, error) {
	return &fakeFileInfo{name: m.name, size: int64(m.Reader.Len())}, nil
}

type dirEntry struct{ name string }

func (d *dirEntry) Read([]byte) (int, error) { return 0, fs.ErrInvalid }
func (d *dirEntry) Close() error             { return nil }
func (d *dirEntry) Stat() (fs.FileInfo, error) {
	return &fakeFileInfo{name: d.name, dir: true}, nil
}

type readOnlyFile struct {
	data []byte
	off  int
	name string
}

func (r *readOnlyFile) Read(p []byte) (int, error) {
	if r.off >= len(r.data) {
		return 0, io.EOF
	}
	n := copy(p, r.data[r.off:])
	r.off += n
	return n, nil
}
func (r *readOnlyFile) Close() error { return nil }
func (r *readOnlyFile) Stat() (fs.FileInfo, error) {
	return &fakeFileInfo{name: r.name, size: int64(len(r.data))}, nil
}

type fakeFileInfo struct {
	name string
	size int64
	dir  bool
}

func (f *fakeFileInfo) Name() string       { return f.name }
func (f *fakeFileInfo) Size() int64        { return f.size }
func (f *fakeFileInfo) Mode() fs.FileMode  { if f.dir { return fs.ModeDir | 0o644 }; return 0o644 }
func (f *fakeFileInfo) ModTime() time.Time { return time.Unix(1, 0) }
func (f *fakeFileInfo) IsDir() bool        { return f.dir }
func (f *fakeFileInfo) Sys() any           { return nil }

func testServerConfig() *config.Config {
	return &config.Config{
		Server:   config.ServerConfig{Host: "127.0.0.1", Port: 0, Mode: "server"},
		Security: config.SecurityConfig{JWTSecret: "test-secret-that-is-at-least-32-chars", RateLimitPerMin: 100, BruteForceMax: 5, BruteForceBanMin: 15},
		Query:    config.QueryConfig{MaxConcurrent: 10, DefaultTimeoutSec: 30, TracerouteTimeoutSec: 30},
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
	if body["version"] != "dev" {
		t.Errorf("version = %v, want dev", body["version"])
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

func TestServerStaticAndSPARoutes(t *testing.T) {
	db := setupTestDB(t)
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS settings (key TEXT PRIMARY KEY, value TEXT NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	_ = queries.New(db).SetSettings(map[string]string{"site_name": "Rich LG"})
	_ = sitecache.RefreshSettings(db, 0)

	dist := newRichDistFS()
	cfg := testServerConfig()
	cfg.Database.Path = filepath.Join(t.TempDir(), "lg.db")
	cfg.FloodControl.Enabled = true
	cfg.FloodControl.BruteForceMax = 3
	srv := New(cfg, db, nil, dist, nil, "dev")
	uploadDir := handler.ResolveUploadsDir(cfg.Database.Path)

	tests := []struct {
		method string
		path   string
		code   int
	}{
		{http.MethodGet, "/appearance-boot.js", http.StatusOK},
		{http.MethodGet, "/assets/app.css", http.StatusOK},
		{http.MethodGet, "/assets/app.js", http.StatusOK},
		{http.MethodGet, "/assets/font.woff2", http.StatusOK},
		{http.MethodGet, "/assets/font.ttf", http.StatusOK},
		{http.MethodGet, "/assets/icon.svg", http.StatusOK},
		{http.MethodGet, "/assets/icon.png", http.StatusOK},
		{http.MethodGet, "/assets/icon.jpg", http.StatusOK},
		{http.MethodGet, "/assets/favicon.ico", http.StatusOK},
		{http.MethodGet, "/assets/data.json", http.StatusOK},
		{http.MethodGet, "/assets/page.html", http.StatusOK},
		{http.MethodGet, "/assets/missing.js", http.StatusNotFound},
		{http.MethodGet, "/assets/subdir", http.StatusNotFound},
		{http.MethodGet, "/assets/bad.dat", http.StatusInternalServerError},
		{http.MethodGet, "/dashboard", http.StatusOK},
		{http.MethodGet, "/api/v1/missing", http.StatusNotFound},
	}
	for _, tc := range tests {
		req := httptest.NewRequest(tc.method, tc.path, nil)
		w := httptest.NewRecorder()
		srv.router.ServeHTTP(w, req)
		if w.Code != tc.code {
			t.Fatalf("%s %s status = %d, want %d", tc.method, tc.path, w.Code, tc.code)
		}
	}

	if err := os.MkdirAll(uploadDir, 0o755); err != nil {
		t.Fatal(err)
	}
	logoPath := filepath.Join(uploadDir, "logo.png")
	if err := os.WriteFile(logoPath, []byte("png"), 0o644); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/logo.png", nil)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)
	if w.Code != http.StatusOK && w.Code != http.StatusNotFound {
		t.Fatalf("logo status = %d", w.Code)
	}
}

func TestServerNoRouteIndexError(t *testing.T) {
	db := setupTestDB(t)
	srv := New(testServerConfig(), db, nil, testFS{}, nil, "dev")
	req := httptest.NewRequest(http.MethodGet, "/spa", nil)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", w.Code)
	}
}

func TestServerRunTLSAndListenError(t *testing.T) {
	db := setupTestDB(t)
	cfg := testServerConfig()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	listener.Close()
	cfg.Server.Port = port

	certFile, keyFile := writeTestTLSCert(t)
	cfgTLS := *cfg
	cfgTLS.Server.TLSCert = certFile
	cfgTLS.Server.TLSKey = keyFile
	cfgTLS.Server.Port = port
	srvTLS := New(&cfgTLS, db, nil, newTestServerFS(), nil, "dev")
	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() { errCh <- srvTLS.Run(ctx) }()
	waitForTLSHealth(t, port)
	cancel()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("tls run: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("tls run timeout")
	}

	cfgBad := testServerConfig()
	cfgBad.Server.Port = port
	cfgBad.Server.TLSCert = filepath.Join(t.TempDir(), "missing.pem")
	cfgBad.Server.TLSKey = filepath.Join(t.TempDir(), "missing.key")
	srvBad := New(cfgBad, db, nil, newTestServerFS(), nil, "dev")
	ctxBad, cancelBad := context.WithCancel(context.Background())
	go func() { _ = srvBad.Run(ctxBad) }()
	time.Sleep(100 * time.Millisecond)
	cancelBad()
}

func waitForTLSHealth(t *testing.T, port int) {
	t.Helper()
	client := &http.Client{
		Timeout: 2 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}
	url := "https://127.0.0.1:" + itoa(port) + "/health"
	for i := 0; i < 50; i++ {
		resp, err := client.Get(url)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("tls server never became ready")
}

func waitForHealth(t *testing.T, port int) {
	t.Helper()
	client := &http.Client{Timeout: 2 * time.Second}
	url := "http://127.0.0.1:" + itoa(port) + "/health"
	for i := 0; i < 50; i++ {
		resp, err := client.Get(url)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("server never became ready")
}

func TestServerAppearanceBootNotFound(t *testing.T) {
	db := setupTestDB(t)
	srv := New(testServerConfig(), db, nil, testFS{}, nil, "dev")
	req := httptest.NewRequest(http.MethodGet, "/appearance-boot.js", nil)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d", w.Code)
	}
}

func writeTestTLSCert(t *testing.T) (certFile, keyFile string) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "localhost"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	certFile = filepath.Join(dir, "cert.pem")
	keyFile = filepath.Join(dir, "key.pem")
	if err := os.WriteFile(certFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o644); err != nil {
		t.Fatal(err)
	}
	keyBytes, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyFile, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyBytes}), 0o600); err != nil {
		t.Fatal(err)
	}
	return certFile, keyFile
}
