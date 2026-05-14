package handler

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/HopStat/HopStat/internal/updater"
)

func UpdateStatus(upd *updater.Updater) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
		defer cancel()

		status, err := upd.Status(ctx)
		if err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"data": status})
	}
}

func UpdateApply(upd *updater.Updater) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusAccepted, gin.H{"data": gin.H{"message": "update started"}})

		go func() {
			time.Sleep(500 * time.Millisecond)
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
			defer cancel()
			if err := upd.Apply(ctx); err != nil {
				slog.Error("self-update failed", "error", err)
			}
		}()
	}
}
