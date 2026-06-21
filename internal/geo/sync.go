package geo

import (
	"github.com/HopStat/HopStat/internal/config"
	"github.com/HopStat/HopStat/internal/store/queries"
)

// SyncSettings copies geoip credentials from config into the settings table when
// the DB keys are still empty (first-run / legacy config.yaml setups).
func SyncSettings(q *queries.Queries, cfg config.GeoIPConfig) error {
	settings, err := q.GetSettings()
	if err != nil {
		return err
	}

	toSet := map[string]string{}
	if settings[SettingLicenseKey] == "" && cfg.LicenseKey != "" {
		toSet[SettingLicenseKey] = cfg.LicenseKey
	}
	if settings[SettingAccountID] == "" && cfg.AccountID != "" {
		toSet[SettingAccountID] = cfg.AccountID
	}
	if settings[SettingUpdateInterval] == "" && cfg.UpdateInterval != "" {
		toSet[SettingUpdateInterval] = cfg.UpdateInterval
	}
	if len(toSet) == 0 {
		return nil
	}
	return q.SetSettings(toSet)
}
