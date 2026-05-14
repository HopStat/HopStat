package middleware

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/yourorg/lg-looking-glass/internal/config"
)

// UISessionAuth checks for JWT in cookie or Authorization header for admin UI pages.
// If no valid token is found, redirects to /admin/login.
func UISessionAuth(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		tokenStr := ""

		// Check cookie first
		if cookie, err := c.Cookie("lg_token"); err == nil && cookie != "" {
			tokenStr = cookie
		}

		// Fallback to Authorization header
		if tokenStr == "" {
			auth := c.GetHeader("Authorization")
			if parts := strings.SplitN(auth, " ", 2); len(parts) == 2 && parts[0] == "Bearer" {
				tokenStr = parts[1]
			}
		}

		if tokenStr == "" {
			c.Redirect(http.StatusFound, "/admin/login")
			c.Abort()
			return
		}

		claims := &Claims{}
		token, err := jwt.ParseWithClaims(tokenStr, claims, func(token *jwt.Token) (interface{}, error) {
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
			}
			return []byte(cfg.Security.JWTSecret), nil
		})

		if err != nil || !token.Valid {
			c.Redirect(http.StatusFound, "/admin/login")
			c.Abort()
			return
		}

		c.Set("user_id", claims.UserID)
		c.Next()
	}
}
