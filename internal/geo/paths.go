package geo

import (
	"path/filepath"

	"github.com/HopStat/HopStat/internal/config"
)

func ResolvePaths(cfg config.GeoIPConfig) (asnPath, cityPath string) {
	dbDir := cfg.DBDir
	if dbDir == "" {
		dbDir = "./data/geoip"
	}
	if cfg.ASNDBPath != "" {
		asnPath = cfg.ASNDBPath
	} else {
		asnPath = filepath.Join(dbDir, "GeoLite2-ASN.mmdb")
	}
	if cfg.CityDBPath != "" {
		cityPath = cfg.CityDBPath
	} else {
		cityPath = filepath.Join(dbDir, "GeoLite2-City.mmdb")
	}
	return asnPath, cityPath
}
