package turnstile

import (
	"errors"
	"strings"

	"github.com/HopStat/HopStat/internal/config"
	"github.com/HopStat/HopStat/internal/store/queries"
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
// because the panel never received it. Clear turns the check off and stops config.yaml
// from seeding it back on the next restart.
type Update struct {
	SiteKey   string `json:"site_key"`
	Secret    string `json:"secret"`
	Hostnames string `json:"hostnames"`
	Clear     bool   `json:"clear"`
}

// SyncSettings copies config.yaml into the settings table once, so an existing deployment
// keeps working and the admin panel can show and change those values.
func SyncSettings(q *queries.Queries, cfg config.TurnstileConfig) error {
	settings, err := q.GetSettings()
	if err != nil {
		return err
	}
	if settings[SettingCleared] == "1" {
		return nil
	}
	toSet := map[string]string{}
	seed := func(key, value string) {
		if settings[key] != "" || strings.TrimSpace(value) == "" {
			return
		}
		toSet[key] = strings.TrimSpace(value)
	}
	seed(SettingSiteKey, cfg.SiteKey)
	seed(SettingSecret, cfg.Secret)
	if hosts := joinHostnames(ParseHostnames(strings.Join(cfg.Hostnames, "\n"))); hosts != "" {
		seed(SettingHostnames, hosts)
	}
	if len(toSet) == 0 {
		return nil
	}
	return q.SetSettings(toSet)
}

// Effective is the config the public query API should enforce. A cleared panel choice
// wins over config.yaml. Stored settings win over the file. An empty store falls back
// to the file so the check stays on before the first panel save.
func Effective(settings map[string]string, fallback config.TurnstileConfig) config.TurnstileConfig {
	if settings[SettingCleared] == "1" {
		return config.TurnstileConfig{}
	}
	site := strings.TrimSpace(settings[SettingSiteKey])
	secret := strings.TrimSpace(settings[SettingSecret])
	hosts := ParseHostnames(settings[SettingHostnames])
	if site == "" && secret == "" && len(hosts) == 0 {
		return normalizeConfig(fallback)
	}
	return config.TurnstileConfig{SiteKey: site, Secret: secret, Hostnames: hosts}
}

// StatusFrom describes the effective check without including the secret.
func StatusFrom(settings map[string]string, fallback config.TurnstileConfig) Status {
	cfg := Effective(settings, fallback)
	hosts := cfg.Hostnames
	if hosts == nil {
		hosts = []string{}
	}
	return Status{
		Configured: cfg.SiteKey != "" && strings.TrimSpace(cfg.Secret) != "" && len(cfg.Hostnames) > 0,
		SiteKey:    cfg.SiteKey,
		SecretSet:  strings.TrimSpace(cfg.Secret) != "",
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

func normalizeConfig(cfg config.TurnstileConfig) config.TurnstileConfig {
	return config.TurnstileConfig{
		SiteKey:   strings.TrimSpace(cfg.SiteKey),
		Secret:    strings.TrimSpace(cfg.Secret),
		Hostnames: ParseHostnames(strings.Join(cfg.Hostnames, "\n")),
	}
}
