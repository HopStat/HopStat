package server

import (
	"database/sql"
	"testing"

	"github.com/HopStat/HopStat/internal/audit"
	"github.com/HopStat/HopStat/internal/config"
	"github.com/HopStat/HopStat/internal/engine"
	"github.com/HopStat/HopStat/internal/store/queries"
	"github.com/HopStat/HopStat/internal/updater"
)

// setupTestDB does not create the settings table; these tests own it.
func settingsDB(t *testing.T) *sql.DB {
	t.Helper()
	db := setupTestDB(t)
	if _, err := db.Exec(
		`CREATE TABLE IF NOT EXISTS settings (key TEXT PRIMARY KEY, value TEXT NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	return db
}

func seedConfig() *config.Config {
	cfg := &config.Config{}
	cfg.Query.DefaultTimeoutSec = 30
	cfg.Query.TracerouteTimeoutSec = 60
	cfg.Update.Enabled = true
	cfg.Audit.RetentionDays = 90
	return cfg
}

func TestSeedSettingsFromConfig_FirstRun(t *testing.T) {
	db := settingsDB(t)
	q := queries.New(db)

	if err := SeedSettingsFromConfig(q, seedConfig()); err != nil {
		t.Fatal(err)
	}

	stored, err := q.GetSettings()
	if err != nil {
		t.Fatal(err)
	}
	if stored[engine.SettingQueryTimeoutSec] != "30" {
		t.Fatalf("query timeout = %q", stored[engine.SettingQueryTimeoutSec])
	}
	if stored[engine.SettingTracerouteTimeoutSec] != "60" {
		t.Fatalf("traceroute timeout = %q", stored[engine.SettingTracerouteTimeoutSec])
	}
	if stored[updater.SettingSelfUpdateEnabled] != "true" {
		t.Fatalf("self update = %q", stored[updater.SettingSelfUpdateEnabled])
	}
	if stored[audit.SettingRetentionDays] != "90" {
		t.Fatalf("audit retention = %q", stored[audit.SettingRetentionDays])
	}
}

func TestSeedSettingsFromConfig_DoesNotOverwriteThePanel(t *testing.T) {
	db := settingsDB(t)
	q := queries.New(db)

	// What the operator chose in the admin panel.
	if err := q.SetSettings(map[string]string{
		engine.SettingQueryTimeoutSec:    "5",
		updater.SettingSelfUpdateEnabled: "false",
	}); err != nil {
		t.Fatal(err)
	}

	if err := SeedSettingsFromConfig(q, seedConfig()); err != nil {
		t.Fatal(err)
	}

	stored, err := q.GetSettings()
	if err != nil {
		t.Fatal(err)
	}
	if stored[engine.SettingQueryTimeoutSec] != "5" {
		t.Fatalf("restart overwrote the panel's timeout: %q", stored[engine.SettingQueryTimeoutSec])
	}
	if stored[updater.SettingSelfUpdateEnabled] != "false" {
		t.Fatalf("restart re-enabled self-update: %q", stored[updater.SettingSelfUpdateEnabled])
	}
}

func TestSeedSettingsFromConfig_NothingToSeed(t *testing.T) {
	db := settingsDB(t)
	q := queries.New(db)

	// Second call has nothing left to write.
	if err := SeedSettingsFromConfig(q, seedConfig()); err != nil {
		t.Fatal(err)
	}
	if err := SeedSettingsFromConfig(q, seedConfig()); err != nil {
		t.Fatal(err)
	}
}

func TestSeedSettingsFromConfig_SkipsUnsetTimeouts(t *testing.T) {
	db := settingsDB(t)
	q := queries.New(db)
	cfg := &config.Config{} // no timeouts, self-update off

	if err := SeedSettingsFromConfig(q, cfg); err != nil {
		t.Fatal(err)
	}

	stored, err := q.GetSettings()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := stored[engine.SettingQueryTimeoutSec]; ok {
		t.Fatal("seeded a timeout the config never set")
	}
	if _, ok := stored[audit.SettingRetentionDays]; ok {
		t.Fatal("seeded a retention the config never set")
	}
	if stored[updater.SettingSelfUpdateEnabled] != "false" {
		t.Fatalf("self update = %q, want the config's false", stored[updater.SettingSelfUpdateEnabled])
	}
}

func TestSeedSettingsFromConfig_ReportsAnUnreadableStore(t *testing.T) {
	// No settings table at all: the seed must report it rather than carry on.
	db := setupTestDB(t)
	if err := SeedSettingsFromConfig(queries.New(db), seedConfig()); err == nil {
		t.Fatal("expected an error when the settings table cannot be read")
	}
}

func TestSelfUpdateSettingSource(t *testing.T) {
	db := settingsDB(t)
	source := selfUpdateSettingSource(db)

	// Nothing stored: no answer, so the config value keeps deciding.
	if _, ok := source(); ok {
		t.Fatal("an unset row should not answer")
	}

	q := queries.New(db)
	for _, tc := range []struct {
		stored         string
		want, answered bool
	}{
		{"true", true, true},
		{"false", false, true},
		{"", false, false},
	} {
		if err := q.SetSettings(map[string]string{updater.SettingSelfUpdateEnabled: tc.stored}); err != nil {
			t.Fatal(err)
		}
		got, answered := source()
		if answered != tc.answered || got != tc.want {
			t.Fatalf("stored %q -> (%v, %v), want (%v, %v)", tc.stored, got, answered, tc.want, tc.answered)
		}
	}
}

func TestSelfUpdateSettingSource_UnreadableStore(t *testing.T) {
	db := setupTestDB(t) // no settings table
	if _, ok := selfUpdateSettingSource(db)(); ok {
		t.Fatal("an unreadable store must not answer")
	}
}
