package turnstile

import (
	"errors"
	"strings"
)

const (
	SettingSiteKey   = "turnstile_site_key"
	SettingSecret    = "turnstile_secret"
	SettingHostnames = "turnstile_hostnames"
	SettingCleared   = "turnstile_cleared"
)

// Status is what the admin panel may see. The secret itself never leaves the server.
type Status struct {
	Configured bool     `json:"configured"`
	SiteKey    string   `json:"site_key"`
	SecretSet  bool     `json:"secret_set"`
	Hostnames  []string `json:"hostnames"`
}

// Update is a change from the admin panel. An empty Secret means "keep the stored one",
// because the panel never received it. Clear turns the check off.
type Update struct {
	SiteKey   string `json:"site_key"`
	Secret    string `json:"secret"`
	Hostnames string `json:"hostnames"`
	Clear     bool   `json:"clear"`
}

// Config is the check enforced on public queries. It comes from the settings database.
type Config struct {
	SiteKey   string
	Secret    string
	Hostnames []string
}

// Effective reads the stored check. A cleared panel choice stays off.
func Effective(settings map[string]string) Config {
	if settings[SettingCleared] == "1" {
		return Config{}
	}
	return Config{
		SiteKey:   strings.TrimSpace(settings[SettingSiteKey]),
		Secret:    strings.TrimSpace(settings[SettingSecret]),
		Hostnames: ParseHostnames(settings[SettingHostnames]),
	}
}

// StatusFrom describes the stored check without including the secret.
func StatusFrom(settings map[string]string) Status {
	cfg := Effective(settings)
	hosts := cfg.Hostnames
	if hosts == nil {
		hosts = []string{}
	}
	return Status{
		Configured: cfg.SiteKey != "" && cfg.Secret != "" && len(cfg.Hostnames) > 0,
		SiteKey:    cfg.SiteKey,
		SecretSet:  cfg.Secret != "",
		Hostnames:  hosts,
	}
}

// SettingsFromUpdate turns a panel save into rows to write.
func SettingsFromUpdate(req Update, current map[string]string) (map[string]string, error) {
	if req.Clear {
		return map[string]string{
			SettingSiteKey:   "",
			SettingSecret:    "",
			SettingHostnames: "",
			SettingCleared:   "1",
		}, nil
	}

	site := strings.TrimSpace(req.SiteKey)
	hosts := ParseHostnames(req.Hostnames)
	secret := strings.TrimSpace(req.Secret)
	if secret == "" {
		secret = strings.TrimSpace(current[SettingSecret])
	}
	out := map[string]string{
		SettingSiteKey:   site,
		SettingHostnames: joinHostnames(hosts),
		SettingCleared:   "",
	}
	if strings.TrimSpace(req.Secret) != "" {
		out[SettingSecret] = strings.TrimSpace(req.Secret)
	}
	if site == "" && secret == "" && len(hosts) == 0 {
		out[SettingSecret] = ""
		out[SettingCleared] = "1"
		return out, nil
	}
	if site == "" || secret == "" || len(hosts) == 0 {
		return nil, errors.New("turnstile site key, secret, and hostnames must all be set together")
	}
	return out, nil
}

// ParseHostnames splits a textarea or comma list and drops blanks and duplicates.
func ParseHostnames(raw string) []string {
	parts := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == '\n' || r == '\r' || r == ' ' || r == '\t'
	})
	out := make([]string, 0, len(parts))
	seen := map[string]struct{}{}
	for _, part := range parts {
		if _, ok := seen[part]; ok {
			continue
		}
		seen[part] = struct{}{}
		out = append(out, part)
	}
	return out
}

func joinHostnames(hosts []string) string {
	return strings.Join(hosts, ",")
}
