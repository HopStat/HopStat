package geo

import (
	"testing"

	"github.com/HopStat/HopStat/internal/config"
)

func TestCollectStatus_Unconfigured(t *testing.T) {
	st := CollectStatus(map[string]string{}, config.GeoIPConfig{}, nil)
	if st.Configured {
		t.Error("expected not configured")
	}
	if st.UpdateInterval != "72h" {
		t.Errorf("interval = %q", st.UpdateInterval)
	}
}

func TestCollectStatus_Configured(t *testing.T) {
	settings := map[string]string{
		SettingLicenseKey:     "key",
		SettingAccountID:      "acc",
		SettingUpdateInterval: "24h",
	}
	st := CollectStatus(settings, config.GeoIPConfig{}, nil)
	if !st.Configured {
		t.Error("expected configured")
	}
	if st.UpdateInterval != "24h" {
		t.Errorf("interval = %q", st.UpdateInterval)
	}
}

func TestLatestRFC3339(t *testing.T) {
	if got := latestRFC3339("", "2024-01-02T00:00:00Z"); got != "2024-01-02T00:00:00Z" {
		t.Errorf("got %q", got)
	}
	if got := latestRFC3339("2024-06-01T00:00:00Z", "2024-01-02T00:00:00Z"); got != "2024-06-01T00:00:00Z" {
		t.Errorf("got %q", got)
	}
}
