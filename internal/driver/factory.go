package driver

import (
	"fmt"

	"github.com/yourorg/lg-looking-glass/internal/config"
	"github.com/yourorg/lg-looking-glass/internal/domain"
	"github.com/yourorg/lg-looking-glass/internal/driver/lgnode"
	"github.com/yourorg/lg-looking-glass/internal/driver/standalone"
)

func NewDriver(node *domain.Node, cfg *config.Config) (NodeDriver, error) {
	switch node.Type {
	case domain.NodeTypeStandalone:
		return standalone.NewDriver(node, cfg)
	case domain.NodeTypeLGNode:
		return lgnode.NewDriver(node, cfg)
	default:
		return nil, fmt.Errorf("unknown node type: %s", node.Type)
	}
}