package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestAuth_AcceptsCookieToken(t *testing.T) {
	cfg := testConfig()
	denyList := NewJTIDenyList()
	token := makeTestToken(cfg.Security.JWTSecret)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/admin/nodes", nil)
	c.Request.AddCookie(&http.Cookie{Name: AuthCookieName, Value: token})

	Auth(cfg, denyList)(c)

	if c.IsAborted() {
		t.Fatalf("auth rejected valid cookie token; code=%d", w.Code)
	}
	if _, ok := c.Get("user_id"); !ok {
		t.Fatal("expected user_id in context")
	}
}

func TestExtractBearerToken_PrefersCookie(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: AuthCookieName, Value: "cookie-token"})
	req.Header.Set("Authorization", "Bearer header-token")
	c.Request = req

	if got := ExtractBearerToken(c); got != "cookie-token" {
		t.Fatalf("ExtractBearerToken = %q, want cookie-token", got)
	}
}
