package engine

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/HopStat/HopStat/internal/domain"
)

type mapSettings struct {
	values map[string]string
	err    error
}

func (m mapSettings) GetSettings(context.Context) (map[string]string, error) {
	return m.values, m.err
}

func engineWithSettings(s SettingsProvider) *QueryEngine {
	return &QueryEngine{
		cfg:      &QueryConfig{DefaultTimeoutSec: 30, TracerouteTimeoutSec: 60},
		settings: s,
	}
}

func TestTimeoutFor_FallsBackToConfig(t *testing.T) {
	e := engineWithSettings(nil)

	if got := e.timeoutFor(context.Background(), domain.CmdPing); got != 30*time.Second {
		t.Fatalf("ping timeout = %s, want 30s", got)
	}
	if got := e.timeoutFor(context.Background(), domain.CmdTraceroute); got != 60*time.Second {
		t.Fatalf("traceroute timeout = %s, want 60s", got)
	}
}

func TestTimeoutFor_StoredValuesWin(t *testing.T) {
	e := engineWithSettings(mapSettings{values: map[string]string{
		SettingQueryTimeoutSec:      "12",
		SettingTracerouteTimeoutSec: "45",
	}})

	if got := e.timeoutFor(context.Background(), domain.CmdPing); got != 12*time.Second {
		t.Fatalf("ping timeout = %s, want 12s", got)
	}
	if got := e.timeoutFor(context.Background(), domain.CmdTraceroute); got != 45*time.Second {
		t.Fatalf("traceroute timeout = %s, want 45s", got)
	}
}

func TestTimeoutFor_SettingsErrorKeepsConfig(t *testing.T) {
	e := engineWithSettings(mapSettings{err: errors.New("db down")})

	if got := e.timeoutFor(context.Background(), domain.CmdPing); got != 30*time.Second {
		t.Fatalf("timeout = %s, want the config value when settings cannot be read", got)
	}
}

func TestSettingSeconds_RejectsUnusableValues(t *testing.T) {
	cases := map[string]string{
		"absent":        "",
		"not a number":  "soon",
		"below floor":   "0",
		"above ceiling": "601",
	}
	for name, raw := range cases {
		t.Run(name, func(t *testing.T) {
			values := map[string]string{}
			if name != "absent" {
				values[SettingQueryTimeoutSec] = raw
			}
			if got := settingSeconds(values, SettingQueryTimeoutSec, 30); got != 30 {
				t.Fatalf("settingSeconds(%q) = %d, want the fallback 30", raw, got)
			}
		})
	}
}

func TestSettingSeconds_AcceptsTheBounds(t *testing.T) {
	for _, raw := range []string{"1", "600"} {
		got := settingSeconds(map[string]string{SettingQueryTimeoutSec: raw}, SettingQueryTimeoutSec, 30)
		if got == 30 {
			t.Fatalf("settingSeconds(%q) fell back; the bounds should be inclusive", raw)
		}
	}
}
