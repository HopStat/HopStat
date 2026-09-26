package geo

import (
	"time"
)

func ParseUpdateInterval(raw string, fallback time.Duration) time.Duration {
	if raw == "" {
		return fallback
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d <= 0 {
		return fallback
	}
	return d
}

func ResolveUpdateInterval(settings map[string]string) time.Duration {
	return ParseUpdateInterval(settings[SettingUpdateInterval], 72*time.Hour)
}

func LastDownloadFromSettings(settings map[string]string, edition string) time.Time {
	key := SettingASNLastDownload
	if edition == "GeoLite2-City" {
		key = SettingCityLastDownload
	}
	raw := settings[key]
	if raw == "" {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return time.Time{}
	}
	return t.UTC()
}
