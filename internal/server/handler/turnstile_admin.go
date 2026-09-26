package handler

import (
	"database/sql"
	"log/slog"
	"net/http"

	"github.com/HopStat/HopStat/internal/config"
	"github.com/HopStat/HopStat/internal/store/queries"
	"github.com/HopStat/HopStat/internal/turnstile"
	"github.com/gin-gonic/gin"
)

func TurnstileStatus(db *sql.DB, cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		settings, err := queries.New(db).GetSettings()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load settings"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"data": turnstile.StatusFrom(settings, cfg.Turnstile)})
	}
}

// UpdateTurnstile stores the public query check so it can be changed in the admin panel.
// An empty secret keeps the stored one. The secret is not included in the response.
func UpdateTurnstile(db *sql.DB, cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req turnstile.Update
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		q := queries.New(db)
		settings, err := q.GetSettings()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load settings"})
			return
		}
		toSet, err := turnstile.SettingsFromUpdate(req, settings)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		if err := q.SetSettings(toSet); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save settings"})
			return
		}
		for k, v := range toSet {
			settings[k] = v
		}
		if err := refreshSettingsCacheFn(db, 0); err != nil {
			slog.Warn("failed to refresh settings cache", "error", err)
		}
		c.JSON(http.StatusOK, gin.H{"data": turnstile.StatusFrom(settings, cfg.Turnstile)})
	}
}
