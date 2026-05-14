package driver

import (
	"testing"

	"github.com/yourorg/lg-looking-glass/internal/domain"
)

func TestNewDriverStandalone(t *testing.T) {
	node := &domain.Node{
		Type:        domain.NodeTypeStandalone,
		EnabledCmds: []domain.CommandType{domain.CmdPing},
	}
	drv, err := NewDriver(node, nil)
	if err != nil {
		t.Fatalf("NewDriver error: %v", err)
	}
	if drv == nil {
		t.Fatal("expected non-nil driver")
	}
}

func TestNewDriverLGNode(t *testing.T) {
	node := &domain.Node{
		Type:        domain.NodeTypeLGNode,
		AgentURL:    "http://localhost:9090",
		AgentToken:  "test-token",
		EnabledCmds: []domain.CommandType{domain.CmdPing},
	}
	drv, err := NewDriver(node, nil)
	if err != nil {
		t.Fatalf("NewDriver error: %v", err)
	}
	if drv == nil {
		t.Fatal("expected non-nil driver")
	}
}
