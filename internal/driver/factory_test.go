package driver

import (
	"testing"

	"github.com/HopStat/HopStat/internal/config"
	"github.com/HopStat/HopStat/internal/domain"
	"github.com/HopStat/HopStat/internal/driver/standalone"
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

func TestNewDriverStandaloneWithTokenUsesStandaloneDriver(t *testing.T) {
	node := &domain.Node{
		Type:        domain.NodeTypeStandalone,
		AgentToken:  "secret-token",
		EnabledCmds: []domain.CommandType{domain.CmdPing},
	}
	cfg := &config.Config{Server: config.ServerConfig{Port: 8080}}
	drv, err := NewDriver(node, cfg)
	if err != nil {
		t.Fatalf("NewDriver error: %v", err)
	}
	if _, ok := drv.(*standalone.Driver); !ok {
		t.Fatalf("expected standalone driver for tokenized standalone node, got %T", drv)
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
