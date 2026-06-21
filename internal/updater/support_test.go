package updater

import "testing"

func TestSelfUpdateSupportedDisabledByEnv(t *testing.T) {
	t.Setenv("LG_UPDATE_ENABLED", "false")
	supported, reason := SelfUpdateSupported()
	if supported {
		t.Fatal("expected disabled via env")
	}
	if reason == "" {
		t.Fatal("expected reason")
	}
}

func TestStatusSelfUpdateDisabledInConfig(t *testing.T) {
	u := New("HopStat/HopStat", "v1.0.0", false)
	// Don't call Status - needs network. Check fields via zero logic:
	if u.enabled {
		t.Fatal("expected disabled")
	}
}
