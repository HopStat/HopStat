package geo

import (
	"os"
	"time"

	"github.com/HopStat/HopStat/internal/config"
)

const (
	SettingLicenseKey       = "geoip_license_key"
	SettingAccountID        = "geoip_account_id"
	SettingUpdateInterval   = "geoip_update_interval"
	SettingASNLastDownload  = "geoip_asn_last_download"
	SettingCityLastDownload = "geoip_city_last_download"
	// SettingCredentialsCleared records that the operator removed the credentials in the
	// admin panel. The settings rows are pre-created empty by the migration, so an empty
	// value alone cannot tell "never set" from "deliberately cleared".
	SettingCredentialsCleared = "geoip_credentials_cleared" //nolint:gosec // G101: a settings key name, not a credential
)

type Status struct {
	Configured bool `json:"configured"`
	Enabled    bool `json:"enabled"`
	// AccountID is safe to show; the licence key never leaves the server, so the panel is
	// told only whether one is stored.
	AccountID        string `json:"account_id"`
	LicenseKeySet    bool   `json:"license_key_set"`
	UpdateInterval   string `json:"update_interval"`
	ASNLastDownload  string `json:"asn_last_download"`
	CityLastDownload string `json:"city_last_download"`
	LastDownload     string `json:"last_download"`
	ASNBuildDate     string `json:"asn_build_date"`
	CityBuildDate    string `json:"city_build_date"`
	ASNLoaded        bool   `json:"asn_loaded"`
	CityLoaded       bool   `json:"city_loaded"`
}

func CollectStatus(settings map[string]string, cfg config.GeoIPConfig, geoDB *GeoIPDB) Status {
	license := settings[SettingLicenseKey]
	account := settings[SettingAccountID]
	interval := settings[SettingUpdateInterval]
	if interval == "" {
		interval = cfg.UpdateInterval
	}
	if interval == "" {
		interval = "72h"
	}

	st := Status{
		Configured:     license != "" && account != "",
		Enabled:        geoDB != nil && geoDB.Enabled(),
		AccountID:      account,
		LicenseKeySet:  license != "",
		UpdateInterval: interval,
	}

	if !st.Configured {
		return st
	}

	st.ASNLastDownload = settings[SettingASNLastDownload]
	st.CityLastDownload = settings[SettingCityLastDownload]

	asnPath, cityPath := ResolvePaths(cfg)
	if st.ASNLastDownload == "" {
		st.ASNLastDownload = fileModTimeRFC3339(asnPath)
	}
	if st.CityLastDownload == "" {
		st.CityLastDownload = fileModTimeRFC3339(cityPath)
	}
	st.LastDownload = latestRFC3339(st.ASNLastDownload, st.CityLastDownload)

	if geoDB != nil {
		info := geoDB.BuildInfo()
		st.ASNLoaded = info.ASNLoaded
		st.CityLoaded = info.CityLoaded
		st.ASNBuildDate = epochToRFC3339(info.ASNBuild)
		st.CityBuildDate = epochToRFC3339(info.CityBuild)
	}

	return st
}

func fileModTimeRFC3339(path string) string {
	info, err := os.Stat(path)
	if err != nil {
		return ""
	}
	return info.ModTime().UTC().Format(time.RFC3339)
}

func epochToRFC3339(epoch int64) string {
	if epoch <= 0 {
		return ""
	}
	return time.Unix(epoch, 0).UTC().Format(time.RFC3339)
}

func latestRFC3339(a, b string) string {
	tA, errA := time.Parse(time.RFC3339, a)
	tB, errB := time.Parse(time.RFC3339, b)
	if errA != nil && errB != nil {
		return ""
	}
	if errA != nil {
		return b
	}
	if errB != nil {
		return a
	}
	if tA.After(tB) {
		return a
	}
	return b
}
