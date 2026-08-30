//go:build smoke

package smoke

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

// The MaxMind credentials used to live only in config.yaml. This drives the path that
// replaced that — through the real binary, with a real admin session — and checks the one
// thing the panel must never do: hand the licence key back.
func TestGeoIPCredentialsRoundTripWithoutLeakingTheKey(t *testing.T) {
	c := newClient(t)

	login := doJSON(t, c, http.MethodPost, "/api/v1/auth/login",
		map[string]string{"email": "admin@hopstat.local", "password": adminPassword})
	if login.StatusCode != http.StatusOK {
		t.Fatalf("login status = %d", login.StatusCode)
	}
	_ = login.Body.Close()

	const key = "smoke-licence-key"

	save := doJSON(t, c, http.MethodPut, "/api/v1/admin/geoip/config", map[string]any{
		"account_id":      "654321",
		"license_key":     key,
		"update_interval": "48h",
	})
	saveBody, _ := io.ReadAll(save.Body)
	_ = save.Body.Close()
	if save.StatusCode != http.StatusOK {
		t.Fatalf("save status = %d, body %s", save.StatusCode, saveBody)
	}
	if strings.Contains(string(saveBody), key) {
		t.Fatalf("save response returned the licence key: %s", saveBody)
	}

	status := geoipStatus(t, c)
	if !status.LicenseKeySet {
		t.Fatal("status does not report a stored licence key")
	}
	if status.AccountID != "654321" {
		t.Fatalf("account_id = %q", status.AccountID)
	}
	if status.UpdateInterval != "48h" {
		t.Fatalf("update_interval = %q", status.UpdateInterval)
	}

	// The generic settings endpoint must not carry it either.
	settings := doJSON(t, c, http.MethodGet, "/api/v1/admin/settings", nil)
	settingsBody, _ := io.ReadAll(settings.Body)
	_ = settings.Body.Close()
	if strings.Contains(string(settingsBody), key) {
		t.Fatalf("admin settings returned the licence key: %s", settingsBody)
	}

	// Saving the interval alone must not disturb the stored key.
	again := doJSON(t, c, http.MethodPut, "/api/v1/admin/geoip/config",
		map[string]any{"update_interval": "72h"})
	_ = again.Body.Close()
	if again.StatusCode != http.StatusOK {
		t.Fatalf("interval-only save = %d", again.StatusCode)
	}
	if status = geoipStatus(t, c); !status.LicenseKeySet || status.UpdateInterval != "72h" {
		t.Fatalf("interval-only save changed the wrong thing: %+v", status)
	}

	// An interval below the floor is refused rather than stored.
	bad := doJSON(t, c, http.MethodPut, "/api/v1/admin/geoip/config",
		map[string]any{"update_interval": "10s"})
	_ = bad.Body.Close()
	if bad.StatusCode != http.StatusBadRequest {
		t.Fatalf("sub-minimum interval = %d, want 400", bad.StatusCode)
	}

	clear := doJSON(t, c, http.MethodPut, "/api/v1/admin/geoip/config",
		map[string]any{"clear_credentials": true})
	_ = clear.Body.Close()
	if clear.StatusCode != http.StatusOK {
		t.Fatalf("clear status = %d", clear.StatusCode)
	}
	if status = geoipStatus(t, c); status.Configured || status.LicenseKeySet || status.AccountID != "" {
		t.Fatalf("credentials survived the clear: %+v", status)
	}
}

type geoipStatusBody struct {
	Configured     bool   `json:"configured"`
	AccountID      string `json:"account_id"`
	LicenseKeySet  bool   `json:"license_key_set"`
	UpdateInterval string `json:"update_interval"`
}

func geoipStatus(t *testing.T, c *http.Client) geoipStatusBody {
	t.Helper()
	res := doJSON(t, c, http.MethodGet, "/api/v1/admin/geoip/status", nil)
	raw, _ := io.ReadAll(res.Body)
	_ = res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status endpoint = %d, body %s", res.StatusCode, raw)
	}
	var wrapper struct {
		Data geoipStatusBody `json:"data"`
	}
	if err := json.Unmarshal(raw, &wrapper); err != nil {
		t.Fatalf("decode status: %v (body %s)", err, raw)
	}
	return wrapper.Data
}
