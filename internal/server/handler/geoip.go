package handler

import (
	"database/sql"
	"net/http"
	"strings"

	"github.com/HopStat/HopStat/internal/config"
	"github.com/HopStat/HopStat/internal/geo"
	"github.com/HopStat/HopStat/internal/store/queries"
	"github.com/gin-gonic/gin"
)

func GeoIPStatus(db *sql.DB, cfg *config.Config, geoDB *geo.GeoIPDB) gin.HandlerFunc {
	return func(c *gin.Context) {
		q := queries.New(db)
		settings, err := q.GetSettings()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load settings"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"data": geo.CollectStatus(settings, cfg.GeoIP, geoDB)})
	}
}

func GeoIPLookup(geoDB *geo.GeoIPDB) gin.HandlerFunc {
	return func(c *gin.Context) {
		if geoDB == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "geoip not available"})
			return
		}
		ip := strings.TrimSpace(c.Query("ip"))
		if ip == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "ip is required"})
			return
		}
		report, err := geoDB.LookupIP(c.Request.Context(), ip)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"data": report})
	}
}
