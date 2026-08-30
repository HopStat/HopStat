package geo

import (
	"github.com/HopStat/HopStat/internal/config"
	"github.com/HopStat/HopStat/internal/store/queries"
)

// SyncSettings copies geoip credentials from config into the settings table on first run,
// so a legacy config.yaml keeps working now that the admin panel owns these values.
func SyncSettings(q *queries.Queries, cfg config.GeoIPConfig) error {
	settings, err := q.GetSettings()
	if err != nil {
		return err
	}

	toSet := map[string]string{}
	seed := func(key, fromConfig string) {
		if settings[key] != "" || fromConfig == "" {
			return
		}
		toSet[key] = fromConfig
	}

	// Credentials removed in the admin panel stay removed. Without this the next restart
	// would seed them straight back out of config.yaml and the clear would look ignored.
	if settings[SettingCredentialsCleared] != "1" {
		seed(SettingLicenseKey, cfg.LicenseKey)
		seed(SettingAccountID, cfg.AccountID)
	}
	seed(SettingUpdateInterval, cfg.UpdateInterval)
	if len(toSet) == 0 {
		return nil
	}
	return q.SetSettings(toSet)
}
