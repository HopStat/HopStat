package turnstile

import (
	"strings"
	"testing"
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
	empty := Effective(nil)
	if empty.SiteKey != "" || empty.Secret != "" || len(empty.Hostnames) != 0 {
		t.Fatalf("empty = %#v", empty)
	}
	off := StatusFrom(nil)
	if off.Configured || off.SecretSet || off.Hostnames == nil {
		t.Fatalf("empty status = %#v", off)
	}

	stored := Effective(map[string]string{
		SettingSiteKey:   " panel-key ",
		SettingSecret:    " panel-secret ",
		SettingHostnames: "lg.example",
	})
	if stored.SiteKey != "panel-key" || stored.Secret != "panel-secret" || stored.Hostnames[0] != "lg.example" {
		t.Fatalf("stored = %#v", stored)
	}
	status := StatusFrom(map[string]string{
		SettingSiteKey:   "panel-key",
		SettingSecret:    "panel-secret",
		SettingHostnames: "lg.example",
	})
	if !status.Configured || !status.SecretSet || status.SiteKey != "panel-key" {
		t.Fatalf("status = %#v", status)
	}

	cleared := Effective(map[string]string{SettingCleared: "1", SettingSiteKey: "panel", SettingSecret: "secret"})
	if cleared.SiteKey != "" || cleared.Secret != "" {
		t.Fatalf("cleared = %#v", cleared)
	}
	clearedStatus := StatusFrom(map[string]string{SettingCleared: "1"})
	if clearedStatus.Configured || clearedStatus.Hostnames == nil {
		t.Fatalf("cleared status = %#v", clearedStatus)
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
