package middleware

import (
	"crypto/tls"
	"net/http/httptest"
	"testing"

	"github.com/HopStat/HopStat/internal/config"
)

func TestSetAuthCookie(t *testing.T) {
	cfg := &config.Config{}
	c, w := newTestContext(httptest.NewRequest("GET", "/", nil))

	SetAuthCookie(c, "jwt-value", cfg)

	resp := w.Result()
	defer resp.Body.Close()
	var found bool
	for _, ck := range resp.Cookies() {
		if ck.Name == AuthCookieName {
			found = true
			if !ck.HttpOnly {
				t.Error("expected httpOnly")
			}
			if ck.Value != "jwt-value" {
				t.Errorf("value = %q", ck.Value)
			}
		}
	}
	if !found {
		t.Fatal("cookie not set")
	}
}

func TestClearAuthCookie(t *testing.T) {
	cfg := &config.Config{}
	c, w := newTestContext(httptest.NewRequest("GET", "/", nil))

	ClearAuthCookie(c, cfg)

	resp := w.Result()
	defer resp.Body.Close()
	for _, ck := range resp.Cookies() {
		if ck.Name == AuthCookieName {
			if ck.MaxAge != -1 {
				t.Errorf("expected clearing MaxAge=-1, got %d", ck.MaxAge)
			}
			return
		}
	}
	t.Fatal("clear cookie header missing")
}

func TestCookieSecure_WithTLS(t *testing.T) {
	cfg := &config.Config{}
	req := httptest.NewRequest("GET", "https://example.com/", nil)
	req.TLS = &tls.ConnectionState{}
	c, _ := newTestContext(req)
	if !cookieSecure(c, cfg) {
		t.Error("expected secure cookie when TLS present")
	}
}

func TestSplitBearer(t *testing.T) {
	if got := splitBearer("Bearer abc"); got != "abc" {
		t.Errorf("got %q", got)
	}
	if got := splitBearer("Basic abc"); got != "" {
		t.Errorf("got %q, want empty", got)
	}
}
