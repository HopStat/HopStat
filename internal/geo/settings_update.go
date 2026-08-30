package geo

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// MinUpdateInterval is the floor for how often the MaxMind databases are fetched. GeoLite2
// is published twice a week, so anything shorter only re-downloads the same files and
// risks the account being rate limited.
const MinUpdateInterval = time.Hour

// CredentialUpdate is a requested change to the MaxMind settings, as the admin panel sends
// it. An empty LicenseKey means "leave the stored one alone", so saving the interval does
// not require re-typing a key the panel never received in the first place.
type CredentialUpdate struct {
	AccountID      string `json:"account_id"`
	LicenseKey     string `json:"license_key"`
	UpdateInterval string `json:"update_interval"`
	// ClearCredentials removes the stored account and key. Explicit, because an empty
	// field already means "unchanged".
	ClearCredentials bool `json:"clear_credentials"`
}

var errAccountIDNumeric = errors.New("account_id must be digits only")

// SettingsFromUpdate turns a requested change into the settings to write, or reports why
// the request cannot be applied. Returns nil when there is nothing to change.
func SettingsFromUpdate(req CredentialUpdate, current map[string]string) (map[string]string, error) {
	if req.ClearCredentials {
		return map[string]string{
			SettingLicenseKey:         "",
			SettingAccountID:          "",
			SettingCredentialsCleared: "1",
		}, nil
	}

	out := map[string]string{}

	account := strings.TrimSpace(req.AccountID)
	if account != "" {
		if strings.IndexFunc(account, func(r rune) bool { return r < '0' || r > '9' }) >= 0 {
			return nil, errAccountIDNumeric
		}
		out[SettingAccountID] = account
	}

	if key := strings.TrimSpace(req.LicenseKey); key != "" {
		out[SettingLicenseKey] = key
	}

	if interval := strings.TrimSpace(req.UpdateInterval); interval != "" {
		d, err := time.ParseDuration(interval)
		if err != nil {
			return nil, fmt.Errorf("update_interval is not a duration (try 72h): %w", err)
		}
		if d < MinUpdateInterval {
			return nil, fmt.Errorf("update_interval must be at least %s", MinUpdateInterval)
		}
		out[SettingUpdateInterval] = interval
	}

	if len(out) == 0 {
		return nil, nil
	}

	// Storing credentials again lifts the clear, so a later restart may seed from config.
	if out[SettingLicenseKey] != "" || out[SettingAccountID] != "" {
		out[SettingCredentialsCleared] = ""
	}

	// A key without an account, or the reverse, downloads nothing: MaxMind needs both.
	// Reject it here rather than leaving the operator to work it out from the logs.
	account = valueAfter(out, current, SettingAccountID)
	key := valueAfter(out, current, SettingLicenseKey)
	if (account == "") != (key == "") {
		return nil, errors.New("account_id and license_key must be set together")
	}

	return out, nil
}

func valueAfter(pending, current map[string]string, key string) string {
	if v, ok := pending[key]; ok {
		return v
	}
	return current[key]
}
