// Package audit keeps the audit log from growing without bound.
package audit

import (
	"context"
	"log/slog"
	"strconv"
	"time"

	"github.com/HopStat/HopStat/internal/domain"
)

// SettingRetentionDays is how many days of audit history to keep. Zero means keep
// everything, which is a deliberate choice rather than a missing value — so it has to be
// asked for explicitly.
const SettingRetentionDays = "audit_retention_days"

// KeepForever is the retention value that switches pruning off.
const KeepForever = 0

// MaxRetentionDays is ten years. Beyond that the setting is indistinguishable from
// KeepForever and only invites typos.
const MaxRetentionDays = 3650

// sqliteTimestampLayout is what CURRENT_TIMESTAMP writes into created_at. The delete
// compares text, so the cutoff has to be written the same way or it matches nothing.
const sqliteTimestampLayout = "2006-01-02 15:04:05"

// pruneInterval is how often the log is trimmed. Audit rows are small and the column is
// indexed, so there is nothing to gain from running this more often.
var pruneInterval = 6 * time.Hour

// PruneInterval is how often Run trims the log.
func PruneInterval() time.Duration { return pruneInterval }

// RetentionFromSettings reads the stored retention, falling back to the config value when
// nothing is stored or the stored value is not a number.
func RetentionFromSettings(settings map[string]string, fallback int) int {
	raw, ok := settings[SettingRetentionDays]
	if !ok || raw == "" {
		return fallback
	}
	days, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	return days
}

type Pruner struct {
	repo      domain.AuditRepository
	retention func() int
	now       func() time.Time
}

func NewPruner(repo domain.AuditRepository) *Pruner {
	return &Pruner{repo: repo, now: func() time.Time { return time.Now().UTC() }}
}

// SetRetention supplies the retention in days, read on every pass so a change in the admin
// panel applies without a restart.
func (p *Pruner) SetRetention(fn func() int) {
	p.retention = fn
}

func (p *Pruner) retentionDays() int {
	if p.retention == nil {
		return KeepForever
	}
	days := p.retention()
	if days < KeepForever {
		return KeepForever
	}
	if days > MaxRetentionDays {
		return MaxRetentionDays
	}
	return days
}

// PruneOnce deletes entries older than the retention window and reports how many went.
// Returns zero without touching the database when retention is off.
func (p *Pruner) PruneOnce(ctx context.Context) (int64, error) {
	days := p.retentionDays()
	if days == KeepForever {
		return 0, nil
	}

	cutoff := p.now().AddDate(0, 0, -days).Format(sqliteTimestampLayout)
	removed, err := p.repo.Cleanup(ctx, cutoff)
	if err != nil {
		return 0, err
	}
	if removed > 0 {
		slog.Info("audit log pruned", "removed", removed, "older_than", cutoff, "retention_days", days)
	}
	return removed, nil
}

// Run prunes once at startup, then on every tick, until the context is cancelled.
func (p *Pruner) Run(ctx context.Context, interval time.Duration) {
	slog.Info("audit log pruner starting", "interval", interval, "retention_days", p.retentionDays())

	p.pruneLogged(ctx)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.pruneLogged(ctx)
		}
	}
}

func (p *Pruner) pruneLogged(ctx context.Context) {
	if _, err := p.PruneOnce(ctx); err != nil {
		slog.Error("audit log prune failed", "error", err)
	}
}
