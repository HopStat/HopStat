package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/HopStat/HopStat/internal/config"
)

const AuthCookieName = "lg_token"

const authCookieMaxAge = 24 * 60 * 60 // seconds

// ExtractBearerToken reads JWT from httpOnly cookie or Authorization header.
func ExtractBearerToken(c *gin.Context) string {
	if cookie, err := c.Cookie(AuthCookieName); err == nil && cookie != "" {
		return cookie
	}
	auth := c.GetHeader("Authorization")
	if parts := splitBearer(auth); parts != "" {
		return parts
	}
	return ""
}

func splitBearer(auth string) string {
	if len(auth) > 7 && auth[:7] == "Bearer " {
		return auth[7:]
	}
	return ""
}

func cookieSecure(c *gin.Context, cfg *config.Config) bool {
	if c.Request.TLS != nil {
		return true
	}
	return cfg != nil && cfg.Server.TLSCert != "" && cfg.Server.TLSKey != ""
}

// SetAuthCookie stores the session JWT in an httpOnly cookie.
func SetAuthCookie(c *gin.Context, token string, cfg *config.Config) {
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(AuthCookieName, token, authCookieMaxAge, "/", "", cookieSecure(c, cfg), true)
}

// ClearAuthCookie removes the session cookie.
func ClearAuthCookie(c *gin.Context, cfg *config.Config) {
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(AuthCookieName, "", -1, "/", "", cookieSecure(c, cfg), true)
}
