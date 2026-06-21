package middleware

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/HopStat/HopStat/internal/config"
)

func TestBruteForceCleanupExpiredBan(t *testing.T) {
	old := bruteForceCleanupInterval
	bruteForceCleanupInterval = time.Millisecond
	t.Cleanup(func() { bruteForceCleanupInterval = old })

	guard := NewBruteForceGuard(1, 1)
	defer guard.Stop()
	guard.mu.Lock()
	guard.attempts["1.2.3.4"] = &attemptInfo{bannedAt: time.Now().Add(-2 * time.Minute), count: 2}
	guard.mu.Unlock()
	time.Sleep(5 * time.Millisecond)
	guard.mu.Lock()
	_, exists := guard.attempts["1.2.3.4"]
	guard.mu.Unlock()
	if exists {
		t.Fatal("expected expired ban entry to be removed")
	}
}

func TestCORSSameOriginMode(t *testing.T) {
	handler := CORS(nil)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Origin", "https://example.com")
	c, w := newTestContext(req)
	handler(c)
	if w.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Fatal("same-origin mode should not emit ACAO")
	}
}

func TestNodeAgentAuthRepoError(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	db.Close()
	req := httptest.NewRequest(http.MethodGet, "/agent/v1/health", nil)
	req.Header.Set("Authorization", "Bearer token")
	c, w := newTestContext(req)
	NodeAgentAuth(db, "")(c)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", w.Code)
	}
}

func TestNodeAgentAuthSkipsNonStandaloneAndEmptyToken(t *testing.T) {
	db := setupAgentDB(t)
	if _, err := db.Exec(`INSERT INTO nodes (name, type, active, enabled_cmds, agent_token) VALUES ('lg', 'lg_node', 1, '["ping"]', 'secret')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO nodes (name, type, active, enabled_cmds, agent_token) VALUES ('empty', 'standalone', 1, '["ping"]', '')`); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/agent/v1/health", nil)
	req.Header.Set("Authorization", "Bearer secret")
	c, w := newTestContext(req)
	NodeAgentAuth(db, "")(c)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d", w.Code)
	}
}

func TestUISessionAuthRevokedToken(t *testing.T) {
	cfg := &config.Config{Security: config.SecurityConfig{JWTSecret: "test-secret-that-is-at-least-32-chars-long"}}
	deny := NewJTIDenyList()
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, &Claims{
		UserID: 1,
		Role:   "admin",
		RegisteredClaims: jwt.RegisteredClaims{
			ID: "revoked-jti",
		},
	})
	tokenStr, err := token.SignedString([]byte(cfg.Security.JWTSecret))
	if err != nil {
		t.Fatal(err)
	}
	deny.Revoke("revoked-jti", time.Now().Add(time.Hour))

	req := httptest.NewRequest(http.MethodGet, "/admin", nil)
	req.Header.Set("Authorization", "Bearer "+tokenStr)
	c, w := newTestContext(req)
	UISessionAuth(cfg, deny)(c)
	if w.Code != http.StatusFound {
		t.Fatalf("status = %d", w.Code)
	}
}

func TestUISessionAuthUnexpectedSigningMethod(t *testing.T) {
	cfg := &config.Config{Security: config.SecurityConfig{JWTSecret: "test-secret-that-is-at-least-32-chars-long"}}
	token := jwt.NewWithClaims(jwt.SigningMethodNone, &Claims{UserID: 1, Role: "admin"})
	tokenStr, err := token.SignedString(jwt.UnsafeAllowNoneSignatureType)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/admin", nil)
	req.Header.Set("Authorization", "Bearer "+tokenStr)
	c, w := newTestContext(req)
	UISessionAuth(cfg, NewJTIDenyList())(c)
	if w.Code != http.StatusFound {
		t.Fatalf("status = %d", w.Code)
	}
}
