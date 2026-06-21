package handler

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/HopStat/HopStat/internal/hoststats"
	"github.com/HopStat/HopStat/internal/hostips"
)

var systemStatsCollector hoststats.Collector = hoststatsCollector{}

var systemAddressLister = func() ([]hostips.Address, []hostips.Address, error) {
	return hostips.List()
}

type hoststatsCollector struct{}

func (hoststatsCollector) Snapshot(ctx context.Context) (hoststats.Snapshot, error) {
	return hoststats.Collect(ctx)
}

func SystemStatus() gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
		defer cancel()

		snap, err := systemStatsCollector.Snapshot(ctx)
		if err != nil {
			slog.Error("failed to collect system status", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": sanitizeError(err)})
			return
		}
		c.JSON(http.StatusOK, gin.H{"data": snap})
	}
}

func SystemAddresses() gin.HandlerFunc {
	return func(c *gin.Context) {
		ipv4, ipv6, err := systemAddressLister()
		if err != nil {
			slog.Error("failed to list system addresses", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": sanitizeError(err)})
			return
		}
		c.JSON(http.StatusOK, gin.H{"data": gin.H{
			"ipv4": ipv4,
			"ipv6": ipv6,
		}})
	}
}
