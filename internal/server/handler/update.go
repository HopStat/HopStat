package handler

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/HopStat/HopStat/internal/updater"
)

func UpdateStatus(upd *updater.Updater) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
		defer cancel()

		status, err := upd.Status(ctx, strings.TrimSpace(c.Query("since")))
		if err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"data": status})
	}
}

func UpdateApply(upd *updater.Updater) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
		defer cancel()

		status, err := upd.Status(ctx, "")
		if err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error()})
			return
		}
		if !status.UpdateAvailable {
			c.JSON(http.StatusBadRequest, gin.H{"error": "no update available"})
			return
		}
		if blocked, msg := updateApplyBlocked(status); blocked {
			if msg == "" {
				msg = "self-update is not available on this deployment"
			}
			c.JSON(http.StatusBadRequest, gin.H{"error": msg})
			return
		}

		c.JSON(http.StatusAccepted, gin.H{"data": gin.H{"message": "update started"}})

		updateApplyAsync(func() {
			updateApplyDelay()
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
			defer cancel()
			if err := upd.Apply(ctx); err != nil {
				slog.Error("self-update failed", "error", err)
			}
		})
	}
}

var (
	updateApplyDelay   = func() { time.Sleep(500 * time.Millisecond) }
	updateApplyAsync   = func(fn func()) { go fn() }
	updateApplyBlocked = func(status *updater.Status) (bool, string) {
		if !status.SelfUpdateEnabled {
			msg := status.SelfUpdateReason
			if msg == "" {
				msg = "self-update is not available on this deployment"
			}
			return true, msg
		}
		return false, ""
	}
)
