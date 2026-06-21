package driver

import (
	"context"

	"github.com/HopStat/HopStat/internal/domain"
)

type NodeDriver interface {
	BGPRoute(ctx context.Context, prefix string) (*domain.BGPResult, error)
	Ping(ctx context.Context, target string, count int) (*domain.PingResult, error)
	Traceroute(ctx context.Context, target string, maxHops int) (*domain.TracerouteResult, error)
	Capabilities() []domain.CommandType
	TestConnection(ctx context.Context) error
}