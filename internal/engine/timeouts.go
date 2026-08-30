package engine

import (
	"context"
	"strconv"
	"time"

	"github.com/HopStat/HopStat/internal/domain"
)

// Settings keys for the query timeouts. They live in the database so they can be changed
// in the admin panel; the values in config.yaml seed them and remain the fallback.
const (
	SettingQueryTimeoutSec      = "query_timeout_sec"
	SettingTracerouteTimeoutSec = "traceroute_timeout_sec"
)

// Bounds for a stored timeout. Below the floor a query cannot finish; above the ceiling a
// wedged run would hold a pool slot for longer than any operator means to allow.
const (
	MinTimeoutSec = 1
	MaxTimeoutSec = 600
)

// settingSeconds reads a duration in seconds from the settings map, falling back when the
// value is absent, unparseable or out of range — a bad row must not break queries.
func settingSeconds(settings map[string]string, key string, fallback int) int {
	raw, ok := settings[key]
	if !ok {
		return fallback
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < MinTimeoutSec || n > MaxTimeoutSec {
		return fallback
	}
	return n
}

// timeoutFor is resolved per query rather than captured at construction, so a change in
// the admin panel applies to the next query instead of the next restart.
func (e *QueryEngine) timeoutFor(ctx context.Context, command domain.CommandType) time.Duration {
	defaultSec := e.cfg.DefaultTimeoutSec
	tracerouteSec := e.cfg.TracerouteTimeoutSec

	if e.settings != nil {
		if stored, err := e.settings.GetSettings(ctx); err == nil {
			defaultSec = settingSeconds(stored, SettingQueryTimeoutSec, defaultSec)
			tracerouteSec = settingSeconds(stored, SettingTracerouteTimeoutSec, tracerouteSec)
		}
	}

	if command == domain.CmdTraceroute {
		return time.Duration(tracerouteSec) * time.Second
	}
	return time.Duration(defaultSec) * time.Second
}
