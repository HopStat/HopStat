package middleware

import (
	"fmt"
	"net/http"

	"github.com/HopStat/HopStat/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

// UISessionAuth checks for JWT in cookie or Authorization header for admin UI pages.
// If no valid token is found, redirects to /admin/login.
func UISessionAuth(cfg *config.Config, denyList *JTIDenyList) gin.HandlerFunc {
	return func(c *gin.Context) {
		tokenStr := ExtractBearerToken(c)

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

		if claims.ID != "" && denyList.IsRevoked(claims.ID) {
			c.Redirect(http.StatusFound, "/admin/login")
			c.Abort()
			return
		}

		c.Set("user_id", claims.UserID)
		c.Set("user_role", claims.Role)
		c.Set("jti", claims.ID)
		if claims.ExpiresAt != nil {
			c.Set("token_exp", claims.ExpiresAt.Time)
		}
		c.Next()
	}
}
