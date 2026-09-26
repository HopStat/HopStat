package turnstile

import (
	"database/sql"
	"strings"
	"testing"

	"github.com/HopStat/HopStat/internal/config"
	"github.com/HopStat/HopStat/internal/store"
	"github.com/HopStat/HopStat/internal/store/queries"
	_ "modernc.org/sqlite"
)

func TestParseHostnames(t *testing.T) {
	got := ParseHostnames(" lg.example ,\nlg.example\tother.example ")
	if strings.Join(got, ",") != "lg.example,other.example" {
		t.Fatalf("hosts = %#v", got)
	}
	if ParseHostnames("  ") != nil && len(ParseHostnames("  ")) != 0 {
		t.Fatal("blank list should be empty")
	}
}

func TestEffectiveAndStatus(t *testing.T) {
	fallback := config.TurnstileConfig{SiteKey: " file-key ", Secret: " file-secret ", Hostnames: []string{" file.example "}}
	fromFile := Effective(nil, fallback)
	if fromFile.SiteKey != "file-key" || fromFile.Secret != "file-secret" || fromFile.Hostnames[0] != "file.example" {
		t.Fatalf("fallback = %#v", fromFile)
	}
	status := StatusFrom(nil, fallback)
	if !status.Configured || !status.SecretSet || status.SiteKey != "file-key" {
		t.Fatalf("status = %#v", status)
	}

	cleared := Effective(map[string]string{SettingCleared: "1", SettingSiteKey: "panel"}, fallback)
	if cleared.SiteKey != "" || cleared.Secret != "" {
		t.Fatalf("cleared should disable the file, got %#v", cleared)
	}
	off := StatusFrom(map[string]string{SettingCleared: "1"}, fallback)
	if off.Configured || off.Hostnames == nil {
		t.Fatalf("cleared status = %#v", off)
	}

	stored := Effective(map[string]string{
		SettingSiteKey:   "panel-key",
		SettingSecret:    "panel-secret",
		SettingHostnames: "lg.example",
	}, fallback)
	if stored.Secret != "panel-secret" || stored.Hostnames[0] != "lg.example" {
		t.Fatalf("stored = %#v", stored)
	}
}

func TestSettingsFromUpdate(t *testing.T) {
	cleared, err := SettingsFromUpdate(Update{Clear: true}, map[string]string{SettingSecret: "keep"})
	if err != nil || cleared[SettingCleared] != "1" || cleared[SettingSecret] != "" {
		t.Fatalf("clear = %#v err = %v", cleared, err)
	}

	current := map[string]string{SettingSecret: " stored "}
	kept, err := SettingsFromUpdate(Update{SiteKey: " site ", Hostnames: "lg.example\nother.example"}, current)
	if err != nil {
		t.Fatal(err)
	}
	if kept[SettingSecret] != "" || kept[SettingSiteKey] != "site" || kept[SettingHostnames] != "lg.example,other.example" || kept[SettingCleared] != "" {
		t.Fatalf("kept = %#v", kept)
	}

	replaced, err := SettingsFromUpdate(Update{SiteKey: "site", Secret: " new ", Hostnames: "lg.example"}, current)
	if err != nil || replaced[SettingSecret] != "new" {
		t.Fatalf("replaced = %#v err = %v", replaced, err)
	}

	off, err := SettingsFromUpdate(Update{}, map[string]string{})
	if err != nil || off[SettingCleared] != "1" {
		t.Fatalf("empty form = %#v err = %v", off, err)
	}

	if _, err := SettingsFromUpdate(Update{SiteKey: "only-key"}, map[string]string{}); err == nil {
		t.Fatal("expected partial config to be rejected")
	}
}

func openSettingsDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err := store.Migrate(db); err != nil {
		t.Fatal(err)
	}
	return db
}

func TestSyncSettings(t *testing.T) {
	db := openSettingsDB(t)
	q := queries.New(db)
	cfg := config.TurnstileConfig{SiteKey: " site ", Secret: " secret ", Hostnames: []string{" lg.example ", "lg.example"}}
	if err := SyncSettings(q, cfg); err != nil {
		t.Fatal(err)
	}
	stored, err := q.GetSettings()
	if err != nil {
		t.Fatal(err)
	}
	if stored[SettingSiteKey] != "site" || stored[SettingSecret] != "secret" || stored[SettingHostnames] != "lg.example" {
		t.Fatalf("stored = %#v", stored)
	}
	if err := SyncSettings(q, config.TurnstileConfig{SiteKey: "other", Secret: "other", Hostnames: []string{"other.example"}}); err != nil {
		t.Fatal(err)
	}
	stored, _ = q.GetSettings()
	if stored[SettingSiteKey] != "site" {
		t.Fatal("sync should not overwrite the panel")
	}

	if err := q.SetSettings(map[string]string{SettingCleared: "1"}); err != nil {
		t.Fatal(err)
	}
	if err := SyncSettings(q, cfg); err != nil {
		t.Fatal(err)
	}
	stored, _ = q.GetSettings()
	if stored[SettingSiteKey] != "site" {
		t.Fatal("a cleared choice must not be seeded again")
	}

	if err := SyncSettings(q, config.TurnstileConfig{}); err != nil {
		t.Fatal(err)
	}
}

func TestSyncSettingsUnreadable(t *testing.T) {
	db := openSettingsDB(t)
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if err := SyncSettings(queries.New(db), config.TurnstileConfig{SiteKey: "k", Secret: "s", Hostnames: []string{"h"}}); err == nil {
		t.Fatal("expected an error")
	}
}

func TestSyncSettingsWriteError(t *testing.T) {
	db := openSettingsDB(t)
	if _, err := db.Exec(`CREATE TRIGGER settings_block BEFORE INSERT ON settings BEGIN SELECT RAISE(ABORT, 'blocked'); END`); err != nil {
		t.Fatal(err)
	}
	err := SyncSettings(queries.New(db), config.TurnstileConfig{SiteKey: "k", Secret: "s", Hostnames: []string{"h"}})
	if err == nil {
		t.Fatal("expected a write error")
	}
}
