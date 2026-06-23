package driver

import (
	"fmt"

	"github.com/HopStat/HopStat/internal/config"
	"github.com/HopStat/HopStat/internal/domain"
	"github.com/HopStat/HopStat/internal/driver/lgnode"
	"github.com/HopStat/HopStat/internal/driver/standalone"
)

func NewDriver(node *domain.Node, cfg *config.Config) (NodeDriver, error) {
	switch node.Type {
	case domain.NodeTypeStandalone:
		// Run commands in-process. Do not loop back through the local agent HTTP
		// API: same-process self-requests can deadlock until query timeout.
		return standalone.NewDriver(node, cfg)
	case domain.NodeTypeLGNode:
		return lgnode.NewDriver(node, cfg)
	default:
		return nil, fmt.Errorf("unknown node type: %s", node.Type)
	}
}