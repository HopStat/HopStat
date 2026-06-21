package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/HopStat/HopStat/internal/config"
	"github.com/gin-gonic/gin"
)

func TestConfigureClientIPBehindCloudflare(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	ConfigureClientIP(router, config.ServerConfig{BehindCloudflare: true})
	router.GET("/", func(c *gin.Context) {
		c.String(http.StatusOK, c.ClientIP())
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("CF-Connecting-IP", "203.0.113.50")
	req.RemoteAddr = "173.245.48.1:12345"

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Body.String() != "203.0.113.50" {
		t.Fatalf("client IP = %q, want 203.0.113.50", w.Body.String())
	}
}

func TestConfigureClientIPWithoutProxyTrust(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	ConfigureClientIP(router, config.ServerConfig{})
	router.GET("/", func(c *gin.Context) {
		c.String(http.StatusOK, c.ClientIP())
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("CF-Connecting-IP", "203.0.113.50")
	req.RemoteAddr = "173.245.48.1:12345"

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Body.String() != "173.245.48.1" {
		t.Fatalf("client IP = %q, want proxy IP when trust disabled", w.Body.String())
	}
}

func TestConfigureClientIPIgnoresSpoofedHeaderFromUntrustedProxy(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	ConfigureClientIP(router, config.ServerConfig{BehindCloudflare: true})
	router.GET("/", func(c *gin.Context) {
		c.String(http.StatusOK, c.ClientIP())
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("CF-Connecting-IP", "203.0.113.50")
	req.RemoteAddr = "198.51.100.10:12345"

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Body.String() != "198.51.100.10" {
		t.Fatalf("client IP = %q, want direct remote IP for untrusted proxy", w.Body.String())
	}
}

func TestConfigureClientIP_InvalidTrustedProxies(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	ConfigureClientIP(router, config.ServerConfig{TrustedProxies: []string{"not-a-cidr"}})
	router.GET("/", func(c *gin.Context) {
		c.String(http.StatusOK, c.ClientIP())
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "10.0.0.1:1234"
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
}

func TestConfigureClientIP_WithTrustedProxies(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	ConfigureClientIP(router, config.ServerConfig{TrustedProxies: []string{"10.0.0.0/8"}})
	router.GET("/", func(c *gin.Context) {
		c.String(http.StatusOK, c.ClientIP())
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Real-IP", "10.1.2.3")
	req.RemoteAddr = "10.0.0.1:1234"
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Body.String() != "10.1.2.3" {
		t.Fatalf("client IP = %q", w.Body.String())
	}
}
