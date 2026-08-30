package updater

import "testing"

func TestSelfUpdateAllowed_FallsBackToConfig(t *testing.T) {
	if !New("r", "v1", true).selfUpdateAllowed() {
		t.Fatal("config-enabled updater reported disabled")
	}
	if New("r", "v1", false).selfUpdateAllowed() {
		t.Fatal("config-disabled updater reported enabled")
	}
}

func TestSelfUpdateAllowed_LiveSourceWins(t *testing.T) {
	u := New("r", "v1", true)
	u.SetEnabledSource(func() (bool, bool) { return false, true })
	if u.selfUpdateAllowed() {
		t.Fatal("the stored setting should override the config value")
	}

	u = New("r", "v1", false)
	u.SetEnabledSource(func() (bool, bool) { return true, true })
	if !u.selfUpdateAllowed() {
		t.Fatal("the stored setting should override the config value")
	}
}

func TestSelfUpdateAllowed_NoStoredAnswerKeepsConfig(t *testing.T) {
	u := New("r", "v1", true)
	u.SetEnabledSource(func() (bool, bool) { return false, false })
	if !u.selfUpdateAllowed() {
		t.Fatal("an absent stored setting must leave the config value in place")
	}
}

func TestApply_RefusesWhenDisabledLive(t *testing.T) {
	u := New("r", "v1", true)
	u.SetEnabledSource(func() (bool, bool) { return false, true })
	if err := u.Apply(t.Context()); err == nil {
		t.Fatal("Apply ran with self-update switched off in the panel")
	}
}
