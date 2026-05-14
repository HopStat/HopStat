package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/yourorg/lg-looking-glass/internal/config"
)

const testJWTSecret = "test-secret-that-is-at-least-32-chars"

func init() {
	gin.SetMode(gin.TestMode)
}

// helper creates a gin.Context bound to an httptest.Recorder with the given request.
func newTestContext(r *http.Request) (*gin.Context, *httptest.ResponseRecorder) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = r
	return c, w
}

func testConfig() *config.Config {
	return &config.Config{
		Security: config.SecurityConfig{
			JWTSecret: testJWTSecret,
		},
	}
}

func makeTestToken(secret string) string {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, &Claims{
		UserID: 1,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	})
	s, _ := token.SignedString([]byte(secret))
	return s
}

// --- CORS ---

func TestCORS_SetsHeadersOnNormalRequest(t *testing.T) {
	handler := CORS([]string{"https://example.com"})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Origin", "https://example.com")
	c, w := newTestContext(req)

	handler(c)

	if origin := w.Header().Get("Access-Control-Allow-Origin"); origin != "https://example.com" {
		t.Errorf("Access-Control-Allow-Origin = %q, want %q", origin, "https://example.com")
	}
	if methods := w.Header().Get("Access-Control-Allow-Methods"); methods != "GET, POST, PUT, PATCH, DELETE, OPTIONS" {
		t.Errorf("Access-Control-Allow-Methods = %q, want %q", methods, "GET, POST, PUT, PATCH, DELETE, OPTIONS")
	}
	if headers := w.Header().Get("Access-Control-Allow-Headers"); headers != "Origin, Content-Type, Accept, Authorization" {
		t.Errorf("Access-Control-Allow-Headers = %q, want %q", headers, "Origin, Content-Type, Accept, Authorization")
	}
	if cred := w.Header().Get("Access-Control-Allow-Credentials"); cred != "true" {
		t.Errorf("Access-Control-Allow-Credentials = %q, want %q", cred, "true")
	}
	if maxAge := w.Header().Get("Access-Control-Max-Age"); maxAge != "86400" {
		t.Errorf("Access-Control-Max-Age = %q, want %q", maxAge, "86400")
	}
}

func TestCORS_OptionsPreflight_Returns204(t *testing.T) {
	handler := CORS([]string{"https://example.com"})

	req := httptest.NewRequest(http.MethodOptions, "/test", nil)
	c, w := newTestContext(req)

	handler(c)

	if w.Code != http.StatusNoContent {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNoContent)
	}
	// c.Abort was called, so Next should not have been invoked.
	// We verify by checking that the context is aborted.
	if !c.IsAborted() {
		t.Error("expected context to be aborted on OPTIONS preflight")
	}
}

// --- RateLimiter ---

func TestRateLimiter_Allow_TracksPerIPSeparately(t *testing.T) {
	rl := NewRateLimiter(2, time.Minute)
	defer rl.Stop()

	// IP "1.1.1.1" gets 2 requests allowed.
	if !rl.Allow("1.1.1.1") {
		t.Error("first request from 1.1.1.1 should be allowed")
	}
	if !rl.Allow("1.1.1.1") {
		t.Error("second request from 1.1.1.1 should be allowed")
	}
	if rl.Allow("1.1.1.1") {
		t.Error("third request from 1.1.1.1 should be blocked")
	}

	// Different IP is independent.
	if !rl.Allow("2.2.2.2") {
		t.Error("first request from 2.2.2.2 should be allowed")
	}
}

func TestRateLimitMiddleware_AllowsUnderLimit_BlocksOverLimit(t *testing.T) {
	handler := RateLimit(2)

	// First two requests should pass.
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.RemoteAddr = "3.3.3.3:1234"
		c, w := newTestContext(req)
		handler(c)

		if w.Code != http.StatusOK {
			t.Errorf("request %d: status = %d, want %d", i+1, w.Code, http.StatusOK)
		}
	}

	// Third request should be rate limited.
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.RemoteAddr = "3.3.3.3:1234"
	c, w := newTestContext(req)
	handler(c)

	if w.Code != http.StatusTooManyRequests {
		t.Errorf("over-limit request: status = %d, want %d", w.Code, http.StatusTooManyRequests)
	}
}

// --- Auth ---

func TestAuth_RejectsMissingHeader(t *testing.T) {
	handler := Auth(testConfig())

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	c, w := newTestContext(req)

	handler(c)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
	if !c.IsAborted() {
		t.Error("expected context to be aborted")
	}
}

func TestAuth_RejectsInvalidToken(t *testing.T) {
	handler := Auth(testConfig())

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Authorization", "Bearer invalid.token.value")
	c, w := newTestContext(req)

	handler(c)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
	if !c.IsAborted() {
		t.Error("expected context to be aborted")
	}
}

func TestAuth_AcceptsValidToken(t *testing.T) {
	cfg := testConfig()
	handler := Auth(cfg)

	tokenStr := makeTestToken(cfg.Security.JWTSecret)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Authorization", "Bearer "+tokenStr)
	c, _ := newTestContext(req)

	handler(c)

	if c.IsAborted() {
		t.Error("expected context not to be aborted for valid token")
	}
	userID, exists := c.Get("user_id")
	if !exists {
		t.Error("expected user_id to be set in context")
	}
	if userID.(int64) != 1 {
		t.Errorf("user_id = %d, want 1", userID.(int64))
	}
}

// --- BruteForceGuard ---

func TestBruteForceGuard_AllowsNormalRequests(t *testing.T) {
	guard := NewBruteForceGuard(3, 5)

	handler := guard.Middleware()

	for i := 0; i < 3; i++ {
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.RemoteAddr = "4.4.4.4:1234"
		c, w := newTestContext(req)

		// Simulate a successful handler that writes 200.
		handler(c)
		w.WriteHeader(http.StatusOK)

		if c.IsAborted() {
			t.Errorf("request %d: should not be aborted", i+1)
		}
	}
}

func TestBruteForceGuard_BlocksAfterMaxFailures(t *testing.T) {
	guard := NewBruteForceGuard(3, 5)

	unauthorizedHandler := func(c *gin.Context) {
		c.Status(http.StatusUnauthorized)
	}

	// Use single router to preserve state
	r := gin.New()
	r.Use(guard.Middleware())
	r.POST("/login", unauthorizedHandler)

	// Simulate max failed login attempts (401 responses).
	for i := 0; i < 3; i++ {
		req := httptest.NewRequest(http.MethodPost, "/login", nil)
		req.RemoteAddr = "5.5.5.5:1234"
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusUnauthorized {
			t.Errorf("attempt %d: status = %d, want %d", i+1, w.Code, http.StatusUnauthorized)
		}
	}

	// Next request from the same IP should be blocked.
	req := httptest.NewRequest(http.MethodPost, "/login", nil)
	req.RemoteAddr = "5.5.5.5:1234"
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusTooManyRequests {
		t.Errorf("status after ban = %d, want %d", w.Code, http.StatusTooManyRequests)
	}
}

// --- UISessionAuth ---

func TestUISessionAuth_RedirectsWithoutToken(t *testing.T) {
	cfg := testConfig()
	handler := UISessionAuth(cfg)

	req := httptest.NewRequest(http.MethodGet, "/admin/dashboard", nil)
	c, w := newTestContext(req)

	handler(c)

	if w.Code != http.StatusFound {
		t.Errorf("status = %d, want %d", w.Code, http.StatusFound)
	}
	loc := w.Header().Get("Location")
	if loc != "/admin/login" {
		t.Errorf("Location = %q, want %q", loc, "/admin/login")
	}
	if !c.IsAborted() {
		t.Error("expected context to be aborted")
	}
}

func TestUISessionAuth_AcceptsValidCookieToken(t *testing.T) {
	cfg := testConfig()
	handler := UISessionAuth(cfg)

	tokenStr := makeTestToken(cfg.Security.JWTSecret)

	req := httptest.NewRequest(http.MethodGet, "/admin/dashboard", nil)
	req.AddCookie(&http.Cookie{Name: "lg_token", Value: tokenStr})
	c, _ := newTestContext(req)

	handler(c)

	if c.IsAborted() {
		t.Error("expected context not to be aborted for valid cookie token")
	}
	userID, exists := c.Get("user_id")
	if !exists {
		t.Error("expected user_id to be set in context")
	}
	if userID.(int64) != 1 {
		t.Errorf("user_id = %d, want 1", userID.(int64))
	}
}

func TestUISessionAuth_RedirectsWithInvalidCookieToken(t *testing.T) {
	cfg := testConfig()
	handler := UISessionAuth(cfg)

	req := httptest.NewRequest(http.MethodGet, "/admin/dashboard", nil)
	req.AddCookie(&http.Cookie{Name: "lg_token", Value: "invalid.token.value"})
	c, w := newTestContext(req)

	handler(c)

	if w.Code != http.StatusFound {
		t.Errorf("status = %d, want %d", w.Code, http.StatusFound)
	}
	loc := w.Header().Get("Location")
	if loc != "/admin/login" {
		t.Errorf("Location = %q, want %q", loc, "/admin/login")
	}
}
