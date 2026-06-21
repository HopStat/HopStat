package handler

import (
	"context"

	"github.com/HopStat/HopStat/internal/sitecache"
)

type dbSettingsProvider struct{}

func (p *dbSettingsProvider) GetSettings(ctx context.Context) (map[string]string, error) {
	_ = ctx
	return sitecache.AllSettings(), nil
}
