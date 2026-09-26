package geo

import (
	"path/filepath"

	"github.com/HopStat/HopStat/internal/config"
)

func ResolvePaths(cfg config.GeoIPConfig) (asnPath, cityPath string) {
	dbDir := cfg.DBDir
	if dbDir == "" {
		dbDir = "/var/lib/hopstat/geoip"
	}
	return filepath.Join(dbDir, "GeoLite2-ASN.mmdb"), filepath.Join(dbDir, "GeoLite2-City.mmdb")
}
