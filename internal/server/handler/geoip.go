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

// UpdateGeoIPConfig writes the MaxMind account, licence key and update interval so they
// can be managed from the admin panel instead of only the config file. The stored key is
// never sent back, so an empty key in the request means "keep the one already stored".
func UpdateGeoIPConfig(db *sql.DB, cfg *config.Config, geoDB *geo.GeoIPDB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req geo.CredentialUpdate
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

		toSet, err := geo.SettingsFromUpdate(req, settings)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		if toSet != nil {
			if err := q.SetSettings(toSet); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save settings"})
				return
			}
			for k, v := range toSet {
				settings[k] = v
			}
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
