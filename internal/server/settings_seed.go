package server

import (
	"database/sql"
	"strconv"

	"github.com/HopStat/HopStat/internal/audit"
	"github.com/HopStat/HopStat/internal/config"
	"github.com/HopStat/HopStat/internal/engine"
	"github.com/HopStat/HopStat/internal/store/queries"
	"github.com/HopStat/HopStat/internal/updater"
)

// SeedSettingsFromConfig copies values that used to live only in config.yaml into the
// settings table, once, so an existing deployment keeps the behaviour it had while the
// admin panel takes over as the place to change them. A stored value always wins, and an
// empty stored value is left alone — it may be a deliberate choice made in the panel.
func SeedSettingsFromConfig(q *queries.Queries, cfg *config.Config) error {
	stored, err := q.GetSettings()
	if err != nil {
		return err
	}

	toSet := map[string]string{}
	seed := func(key, value string) {
		if stored[key] != "" {
			return
		}
		toSet[key] = value
	}

	if cfg.Query.DefaultTimeoutSec > 0 {
		seed(engine.SettingQueryTimeoutSec, strconv.Itoa(cfg.Query.DefaultTimeoutSec))
	}
	if cfg.Query.TracerouteTimeoutSec > 0 {
		seed(engine.SettingTracerouteTimeoutSec, strconv.Itoa(cfg.Query.TracerouteTimeoutSec))
	}
	seed(updater.SettingSelfUpdateEnabled, strconv.FormatBool(cfg.Update.Enabled))
	if cfg.Audit.RetentionDays > 0 {
		seed(audit.SettingRetentionDays, strconv.Itoa(cfg.Audit.RetentionDays))
	}

	if len(toSet) == 0 {
		return nil
	}
	return q.SetSettings(toSet)
}

// selfUpdateSettingSource answers from the settings table, reporting "no answer" when the
// row is missing or unreadable so the value from config.yaml still stands.
func selfUpdateSettingSource(db *sql.DB) func() (bool, bool) {
	return func() (bool, bool) {
		stored, err := queries.New(db).GetSettings()
		if err != nil {
			return false, false
		}
		raw, ok := stored[updater.SettingSelfUpdateEnabled]
		if !ok || raw == "" {
			return false, false
		}
		return raw == "true", true
	}
}
