package handler

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/HopStat/HopStat/internal/geo"
	"github.com/HopStat/HopStat/internal/store/queries"
)

func geoStatusFromBody(t *testing.T, body []byte) geo.Status {
	t.Helper()
	var resp struct {
		Data geo.Status `json:"data"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("decode: %v (body %s)", err, body)
	}
	return resp.Data
}

func TestUpdateGeoIPConfig_InvalidJSON(t *testing.T) {
	db := setupDB(t)
	c, w := setupAdminContext(db, http.MethodPut, "/admin/geoip/config", "{", 1)

	UpdateGeoIPConfig(db, testConfig(), nil)(c)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

func TestUpdateGeoIPConfig_SettingsUnreadable(t *testing.T) {
	db := setupDB(t)
	if _, err := db.Exec(`DROP TABLE settings`); err != nil {
		t.Fatal(err)
	}
	c, w := setupAdminContext(db, http.MethodPut, "/admin/geoip/config",
		`{"account_id":"42","license_key":"k"}`, 1)

	UpdateGeoIPConfig(db, testConfig(), nil)(c)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", w.Code)
	}
}

func TestUpdateGeoIPConfig_Rejected(t *testing.T) {
	db := setupDB(t)
	c, w := setupAdminContext(db, http.MethodPut, "/admin/geoip/config",
		`{"account_id":"not-a-number","license_key":"k"}`, 1)

	UpdateGeoIPConfig(db, testConfig(), nil)(c)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 — body %s", w.Code, w.Body.String())
	}
}

func TestUpdateGeoIPConfig_SaveFails(t *testing.T) {
	db := setupDB(t)
	// Readable but not writable: the handler has to report the failed write rather than
	// answer as though the settings had been stored.
	if _, err := db.Exec(
		`CREATE TRIGGER settings_readonly BEFORE INSERT ON settings
		 BEGIN SELECT RAISE(ABORT, 'read-only'); END`); err != nil {
		t.Fatal(err)
	}
	c, w := setupAdminContext(db, http.MethodPut, "/admin/geoip/config",
		`{"account_id":"42","license_key":"k"}`, 1)

	UpdateGeoIPConfig(db, testConfig(), nil)(c)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500 — body %s", w.Code, w.Body.String())
	}
}

func TestUpdateGeoIPConfig_NothingToChange(t *testing.T) {
	db := setupDB(t)
	c, w := setupAdminContext(db, http.MethodPut, "/admin/geoip/config", `{}`, 1)

	UpdateGeoIPConfig(db, testConfig(), nil)(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 — body %s", w.Code, w.Body.String())
	}
}

func TestUpdateGeoIPConfig_StoresAndReportsWithoutTheKey(t *testing.T) {
	db := setupDB(t)
	c, w := setupAdminContext(db, http.MethodPut, "/admin/geoip/config",
		`{"account_id":"42","license_key":"secret-key","update_interval":"24h"}`, 1)

	UpdateGeoIPConfig(db, testConfig(), nil)(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 — body %s", w.Code, w.Body.String())
	}

	stored, err := queries.New(db).GetSettings()
	if err != nil {
		t.Fatal(err)
	}
	if stored[geo.SettingLicenseKey] != "secret-key" {
		t.Fatalf("license key not stored: %q", stored[geo.SettingLicenseKey])
	}
	if stored[geo.SettingAccountID] != "42" {
		t.Fatalf("account id not stored: %q", stored[geo.SettingAccountID])
	}
	if stored[geo.SettingUpdateInterval] != "24h" {
		t.Fatalf("interval not stored: %q", stored[geo.SettingUpdateInterval])
	}

	// The response tells the panel a key exists without handing it back.
	if body := w.Body.String(); strings.Contains(body, "secret-key") {
		t.Fatalf("response leaked the licence key: %s", body)
	}
	st := geoStatusFromBody(t, w.Body.Bytes())
	if !st.LicenseKeySet || st.AccountID != "42" || !st.Configured {
		t.Fatalf("status = %+v", st)
	}
	if st.UpdateInterval != "24h" {
		t.Fatalf("interval = %q", st.UpdateInterval)
	}
}

func TestUpdateGeoIPConfig_Clear(t *testing.T) {
	db := setupDB(t)
	if err := queries.New(db).SetSettings(map[string]string{
		geo.SettingLicenseKey: "k",
		geo.SettingAccountID:  "42",
	}); err != nil {
		t.Fatal(err)
	}
	c, w := setupAdminContext(db, http.MethodPut, "/admin/geoip/config",
		`{"clear_credentials":true}`, 1)

	UpdateGeoIPConfig(db, testConfig(), nil)(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 — body %s", w.Code, w.Body.String())
	}
	st := geoStatusFromBody(t, w.Body.Bytes())
	if st.Configured || st.LicenseKeySet || st.AccountID != "" {
		t.Fatalf("credentials survived the clear: %+v", st)
	}
}

func TestGetAdminSettings_DoesNotReturnTheLicenceKey(t *testing.T) {
	db := setupDB(t)
	if err := queries.New(db).SetSettings(map[string]string{
		geo.SettingLicenseKey: "secret-key",
		geo.SettingAccountID:  "42",
	}); err != nil {
		t.Fatal(err)
	}
	c, w := setupAdminContext(db, http.MethodGet, "/admin/settings", "", 1)

	GetAdminSettings(db)(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	body := w.Body.String()
	if strings.Contains(body, "secret-key") {
		t.Fatalf("admin settings leaked the licence key: %s", body)
	}
	// The account id is not a secret and the panel needs it.
	if !strings.Contains(body, geo.SettingAccountID) {
		t.Fatalf("account id missing from admin settings: %s", body)
	}
}
