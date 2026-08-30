package middleware

import (
	"crypto/subtle"
	"database/sql"
	"net/http"
	"strings"

	"github.com/HopStat/HopStat/internal/domain"
	"github.com/HopStat/HopStat/internal/store/repo"
	"github.com/gin-gonic/gin"
)

const AgentNodeKey = "agent_node"

func NodeAgentAuth(db *sql.DB, credKey string) gin.HandlerFunc {
	nodeRepo := repo.NewNodeRepo(db, credKey)
	return func(c *gin.Context) {
		auth := c.GetHeader("Authorization")
		if auth == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing authorization header"})
			return
		}
		if !strings.HasPrefix(auth, "Bearer ") {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid authorization format"})
			return
		}
		token := strings.TrimPrefix(auth, "Bearer ")

		nodes, err := nodeRepo.GetActive(c.Request.Context())
		if err != nil {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
			return
		}

		for _, node := range nodes {
			if node.Type != domain.NodeTypeStandalone {
				continue
			}
			if node.AgentToken == "" {
				continue
			}
			if subtle.ConstantTimeCompare([]byte(token), []byte(node.AgentToken)) == 1 {
				c.Set(AgentNodeKey, node)
				c.Next()
				return
			}
		}

		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
	}
}
