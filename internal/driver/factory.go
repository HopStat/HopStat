package driver

import (
	"fmt"

	"github.com/HopStat/HopStat/internal/config"
	"github.com/HopStat/HopStat/internal/domain"
	"github.com/HopStat/HopStat/internal/driver/lgnode"
	"github.com/HopStat/HopStat/internal/driver/standalone"
)

func NewDriver(node *domain.Node, cfg *config.Config) (NodeDriver, error) {
	if node.Type == domain.NodeTypeStandalone && node.AgentToken != "" && cfg != nil && cfg.Server.Port > 0 {
		proxy := *node
		proxy.Type = domain.NodeTypeLGNode
		proxy.AgentURL = fmt.Sprintf("http://127.0.0.1:%d", cfg.Server.Port)
		return lgnode.NewDriver(&proxy, cfg)
	}

	switch node.Type {
	case domain.NodeTypeStandalone:
		return standalone.NewDriver(node, cfg)
	case domain.NodeTypeLGNode:
		return lgnode.NewDriver(node, cfg)
	default:
		return nil, fmt.Errorf("unknown node type: %s", node.Type)
	}
}