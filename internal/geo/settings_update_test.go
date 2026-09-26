package geo

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/HopStat/HopStat/internal/config"
)

func TestSettingsFromUpdate_Clear(t *testing.T) {
	got, err := SettingsFromUpdate(
		CredentialUpdate{ClearCredentials: true},
		map[string]string{SettingLicenseKey: "k", SettingAccountID: "1"},
	)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if got[SettingLicenseKey] != "" || got[SettingAccountID] != "" {
		t.Fatalf("clear left credentials behind: %+v", got)
	}
}

func TestSettingsFromUpdate_NothingToChange(t *testing.T) {
	got, err := SettingsFromUpdate(CredentialUpdate{}, map[string]string{})
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if got != nil {
		t.Fatalf("expected no writes, got %+v", got)
	}
}

func TestSettingsFromUpdate_AccountIDMustBeNumeric(t *testing.T) {
	if _, err := SettingsFromUpdate(
		CredentialUpdate{AccountID: "12a4", LicenseKey: "k"},
		map[string]string{},
	); err == nil {
		t.Fatal("expected a non-numeric account id to be rejected")
	}
}

func TestSettingsFromUpdate_IntervalMustParse(t *testing.T) {
	if _, err := SettingsFromUpdate(
		CredentialUpdate{UpdateInterval: "every tuesday"},
		map[string]string{},
	); err == nil {
		t.Fatal("expected an unparseable interval to be rejected")
	}
}

func TestSettingsFromUpdate_IntervalHasAFloor(t *testing.T) {
	_, err := SettingsFromUpdate(
		CredentialUpdate{UpdateInterval: "30s"},
		map[string]string{},
	)
	if err == nil {
		t.Fatalf("expected an interval under %s to be rejected", MinUpdateInterval)
	}
}

func TestSettingsFromUpdate_IntervalAlone(t *testing.T) {
	// The panel never receives the stored key, so saving only the interval has to work
	// without the caller re-sending credentials.
	got, err := SettingsFromUpdate(
		CredentialUpdate{UpdateInterval: "24h"},
		map[string]string{SettingLicenseKey: "k", SettingAccountID: "1"},
	)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if got[SettingUpdateInterval] != "24h" {
		t.Fatalf("interval = %q", got[SettingUpdateInterval])
	}
	if _, ok := got[SettingLicenseKey]; ok {
		t.Fatal("interval-only save must not touch the stored key")
	}
}

func TestSettingsFromUpdate_KeyWithoutAccountRejected(t *testing.T) {
	if _, err := SettingsFromUpdate(
		CredentialUpdate{LicenseKey: "k"},
		map[string]string{},
	); err == nil {
		t.Fatal("expected a key with no account to be rejected")
	}
}

func TestSettingsFromUpdate_AccountWithoutKeyRejected(t *testing.T) {
	if _, err := SettingsFromUpdate(
		CredentialUpdate{AccountID: "42"},
		map[string]string{},
	); err == nil {
		t.Fatal("expected an account with no key to be rejected")
	}
}

func TestSettingsFromUpdate_KeyCompletesStoredAccount(t *testing.T) {
	got, err := SettingsFromUpdate(
		CredentialUpdate{LicenseKey: "k"},
		map[string]string{SettingAccountID: "42"},
	)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if got[SettingLicenseKey] != "k" {
		t.Fatalf("key = %q", got[SettingLicenseKey])
	}
}

func TestSettingsFromUpdate_Full(t *testing.T) {
	got, err := SettingsFromUpdate(
		CredentialUpdate{AccountID: " 42 ", LicenseKey: " abc ", UpdateInterval: " 72h "},
		map[string]string{},
	)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	want := map[string]string{
		SettingAccountID:      "42",
		SettingLicenseKey:     "abc",
		SettingUpdateInterval: "72h",
	}
	for k, v := range want {
		if got[k] != v {
			t.Fatalf("%s = %q, want %q", k, got[k], v)
		}
	}
}

func TestMinUpdateIntervalIsAnHour(t *testing.T) {
	if MinUpdateInterval != time.Hour {
		t.Fatalf("MinUpdateInterval = %s", MinUpdateInterval)
	}
}

func TestUpdateAllSkipsWithoutCredentials(t *testing.T) {
	// Observed through the lastDownload hook: needsDownload consults it, so an untouched
	// hook proves updateAll returned before attempting anything.
	consulted := false
	u := NewUpdater(config.GeoIPConfig{}, New("", ""))
	u.SetLastDownload(func(string) time.Time { consulted = true; return time.Time{} })
	u.SetCredentials(func() (string, string) { return "", "" })

	u.updateAll(context.Background())

	if consulted {
		t.Fatal("updateAll attempted a download with no credentials stored")
	}
}

func TestUpdateAllProceedsWithCredentials(t *testing.T) {
	// The edition files have to exist, otherwise needsDownload short-circuits to an
	// immediate download and never reaches the hook this asserts on.
	dir := t.TempDir()
	consulted := false
	u := NewUpdater(config.GeoIPConfig{}, New("", ""))
	u.asnPath = writeASNEditionFiles(t, dir)
	u.cityPath = filepath.Join(dir, "GeoLite2-City.mmdb")
	u.SetLastDownload(func(string) time.Time { consulted = true; return time.Now().UTC() })
	u.SetCredentials(func() (string, string) { return "key", "42" })

	u.updateAll(context.Background())

	if !consulted {
		t.Fatal("updateAll skipped even though credentials were stored")
	}
}
