package middleware

import (
	"bytes"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

func TestRequireAdmin(t *testing.T) {
	handler := RequireAdmin()

	req := httptest.NewRequest(http.MethodGet, "/admin", nil)
	c, w := newTestContext(req)
	handler(c)
	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d", w.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/admin", nil)
	c, _ = newTestContext(req)
	c.Set("user_role", "admin")
	handler(c)
	if c.IsAborted() {
		t.Fatal("admin should pass")
	}
}

func TestLogger(t *testing.T) {
	var buf bytes.Buffer
	old := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	t.Cleanup(func() { slog.SetDefault(old) })

	r := gin.New()
	r.Use(Logger())
	r.GET("/hello", func(c *gin.Context) { c.Status(http.StatusOK) })

	req := httptest.NewRequest(http.MethodGet, "/hello", nil)
	req.RemoteAddr = "127.0.0.1:1234"
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	if !bytes.Contains(buf.Bytes(), []byte("request")) {
		t.Fatalf("log = %q", buf.String())
	}
}

func TestJTIDenyList_RevokeAndIsRevoked(t *testing.T) {
	dl := NewJTIDenyList()

	future := time.Now().Add(time.Hour)
	dl.Revoke("jti-1", future)
	if !dl.IsRevoked("jti-1") {
		t.Fatal("expected revoked")
	}
	if dl.IsRevoked("missing") {
		t.Fatal("expected not revoked")
	}

	past := time.Now().Add(-time.Hour)
	dl.Revoke("expired", past)
	if dl.IsRevoked("expired") {
		t.Fatal("expired token should not be revoked")
	}

	for i := 0; i < maxDenyListEntries+1; i++ {
		dl.Revoke("fill-"+string(rune('a'+i%26)), future)
	}
	dl.Revoke("should-drop", future)

	time.Sleep(20 * time.Millisecond)
	dl.Revoke("after-purge", future)
}

func TestAuth_RevokedToken(t *testing.T) {
	cfg := testConfig()
	dl := NewJTIDenyList()
	handler := Auth(cfg, dl)

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, &Claims{
		UserID: 1,
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        "revoked-jti",
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	})
	tokenStr, _ := token.SignedString([]byte(cfg.Security.JWTSecret))
	dl.Revoke("revoked-jti", time.Now().Add(time.Hour))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Authorization", "Bearer "+tokenStr)
	c, w := newTestContext(req)
	handler(c)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d", w.Code)
	}
}

func TestAuth_UnexpectedSigningMethod(t *testing.T) {
	cfg := testConfig()
	handler := Auth(cfg, NewJTIDenyList())

	token := jwt.NewWithClaims(jwt.SigningMethodNone, &Claims{UserID: 1})
	tokenStr, _ := token.SignedString(jwt.UnsafeAllowNoneSignatureType)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Authorization", "Bearer "+tokenStr)
	c, w := newTestContext(req)
	handler(c)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d", w.Code)
	}
}

func TestBruteForceGuard_StopAndCleanup(t *testing.T) {
	guard := NewBruteForceGuard(2, 1)
	defer guard.Stop()

	unauthorized := func(c *gin.Context) { c.Status(http.StatusUnauthorized) }
	r := gin.New()
	r.Use(guard.Middleware())
	r.POST("/login", unauthorized)

	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodPost, "/login", nil)
		req.RemoteAddr = "6.6.6.6:1234"
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("attempt %d status = %d", i+1, w.Code)
		}
	}

	req := httptest.NewRequest(http.MethodPost, "/login", nil)
	req.RemoteAddr = "6.6.6.6:1234"
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d", w.Code)
	}

	time.Sleep(20 * time.Millisecond)
}

func TestBruteForceGuard_SuccessClearsAttempts(t *testing.T) {
	guard := NewBruteForceGuard(3, 5)
	defer guard.Stop()
	handler := guard.Middleware()

	req := httptest.NewRequest(http.MethodPost, "/login", nil)
	req.RemoteAddr = "7.7.7.7:1234"
	c, _ := newTestContext(req)
	handler(c)
	c.Status(http.StatusUnauthorized)

	req = httptest.NewRequest(http.MethodPost, "/login", nil)
	req.RemoteAddr = "7.7.7.7:1234"
	c, _ = newTestContext(req)
	handler(c)
	c.Status(http.StatusOK)
}

func TestRateLimiter_StopAndCleanup(t *testing.T) {
	rl := NewRateLimiter(1, 50*time.Millisecond)
	defer rl.Stop()

	if !rl.Allow("8.8.8.8") {
		t.Fatal("first should pass")
	}
	if rl.Allow("8.8.8.8") {
		t.Fatal("second should fail")
	}
	time.Sleep(60 * time.Millisecond)
	if !rl.Allow("8.8.8.8") {
		t.Fatal("expected allow after window")
	}
	time.Sleep(20 * time.Millisecond)
}

func TestRateLimiter_MaxTrackedIPs(t *testing.T) {
	prev := rateLimitMaxTrackedIPs
	rateLimitMaxTrackedIPs = 3
	t.Cleanup(func() { rateLimitMaxTrackedIPs = prev })

	rl := NewRateLimiter(100, time.Minute)
	defer rl.Stop()
	for i := 0; i < 3; i++ {
		ip := fmt.Sprintf("10.0.0.%d", i+1)
		if !rl.Allow(ip) {
			t.Fatalf("ip %s should be allowed", ip)
		}
	}
	if rl.Allow("new-ip") {
		t.Fatal("expected reject when map full")
	}
}

func TestOptionalRateLimit(t *testing.T) {
	disabled := OptionalRateLimit(false, 10)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	c, _ := newTestContext(req)
	disabled(c)
	if c.IsAborted() {
		t.Fatal("disabled should not abort")
	}

	zero := OptionalRateLimit(true, 0)
	c, _ = newTestContext(req)
	zero(c)
	if c.IsAborted() {
		t.Fatal("zero limit should not abort")
	}

	enabled := OptionalRateLimit(true, 1)
	c, _ = newTestContext(req)
	c.Request.RemoteAddr = "9.9.9.9:1"
	enabled(c)
	c.Status(http.StatusOK)
	c, w := newTestContext(req)
	c.Request.RemoteAddr = "9.9.9.9:1"
	enabled(c)
	if w.Code == 0 {
		c.Status(http.StatusOK)
	}
}
