package standalone

import (
	"context"
	"testing"
	"time"

	"github.com/HopStat/HopStat/internal/domain"
)

func TestNewDriver(t *testing.T) {
	node := &domain.Node{
		Name:        "test",
		Type:        domain.NodeTypeStandalone,
		Active:      true,
		EnabledCmds: []domain.CommandType{domain.CmdPing, domain.CmdTraceroute},
	}
	drv, err := NewDriver(node, nil)
	if err != nil {
		t.Fatalf("NewDriver error: %v", err)
	}
	if drv == nil {
		t.Fatal("expected non-nil driver")
	}
}

func TestDriverCapabilities(t *testing.T) {
	cmds := []domain.CommandType{domain.CmdPing, domain.CmdTraceroute, domain.CmdMTR}
	node := &domain.Node{EnabledCmds: cmds}
	drv, _ := NewDriver(node, nil)

	caps := drv.Capabilities()
	if len(caps) != 3 {
		t.Errorf("expected 3 capabilities, got %d", len(caps))
	}
}

func TestDriverTestConnection(t *testing.T) {
	node := &domain.Node{EnabledCmds: []domain.CommandType{domain.CmdPing}}
	drv, _ := NewDriver(node, nil)

	if err := drv.TestConnection(context.Background()); err != nil {
		t.Errorf("TestConnection error: %v", err)
	}
}

func TestDriverPingDisabled(t *testing.T) {
	node := &domain.Node{EnabledCmds: []domain.CommandType{}}
	drv, _ := NewDriver(node, nil)

	_, err := drv.Ping(context.Background(), "8.8.8.8", 3)
	if err != domain.ErrCommandDisabled {
		t.Errorf("expected ErrCommandDisabled, got %v", err)
	}
}

func TestDriverPingInvalidTarget(t *testing.T) {
	node := &domain.Node{EnabledCmds: []domain.CommandType{domain.CmdPing}}
	drv, _ := NewDriver(node, nil)

	_, err := drv.Ping(context.Background(), "not-an-ip", 3)
	if err != domain.ErrInvalidTarget {
		t.Errorf("expected ErrInvalidTarget, got %v", err)
	}
}

func TestDriverTracerouteDisabled(t *testing.T) {
	node := &domain.Node{EnabledCmds: []domain.CommandType{}}
	drv, _ := NewDriver(node, nil)

	_, err := drv.Traceroute(context.Background(), "8.8.8.8", 30)
	if err != domain.ErrCommandDisabled {
		t.Errorf("expected ErrCommandDisabled, got %v", err)
	}
}

func TestDriverMTRDisabled(t *testing.T) {
	node := &domain.Node{EnabledCmds: []domain.CommandType{}}
	drv, _ := NewDriver(node, nil)

	_, err := drv.MTR(context.Background(), "8.8.8.8", 5)
	if err != domain.ErrCommandDisabled {
		t.Errorf("expected ErrCommandDisabled, got %v", err)
	}
}

func TestDriverBGPRouteDisabled(t *testing.T) {
	node := &domain.Node{EnabledCmds: []domain.CommandType{}}
	drv, _ := NewDriver(node, nil)

	_, err := drv.BGPRoute(context.Background(), "1.1.1.0/24")
	if err != domain.ErrCommandDisabled {
		t.Errorf("expected ErrCommandDisabled, got %v", err)
	}
}

func TestDriverBGPRouteInvalidPrefix(t *testing.T) {
	node := &domain.Node{EnabledCmds: []domain.CommandType{domain.CmdBGPRoute}}
	drv, _ := NewDriver(node, nil)

	_, err := drv.BGPRoute(context.Background(), "not-a-prefix")
	if err != domain.ErrInvalidTarget {
		t.Errorf("expected ErrInvalidTarget, got %v", err)
	}
}

func TestDriverBGPRouteAcceptsBareIP(t *testing.T) {
	node := &domain.Node{EnabledCmds: []domain.CommandType{domain.CmdBGPRoute}}
	drv, _ := NewDriver(node, nil)

	// Should not return ErrInvalidTarget for a valid bare IP
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, err := drv.BGPRoute(ctx, "1.1.1.1")
	// Will fail because no birdc/vtysh installed, but should not be ErrInvalidTarget
	if err == domain.ErrInvalidTarget {
		t.Error("bare IP should be accepted as valid target for BGP route")
	}
}

func TestDriverASPathDisabled(t *testing.T) {
	node := &domain.Node{EnabledCmds: []domain.CommandType{}}
	drv, _ := NewDriver(node, nil)

	_, err := drv.ASPath(context.Background(), 13335)
	if err != domain.ErrCommandDisabled {
		t.Errorf("expected ErrCommandDisabled, got %v", err)
	}
}

func TestDriverASPathInvalidASN(t *testing.T) {
	node := &domain.Node{EnabledCmds: []domain.CommandType{domain.CmdASPath}}
	drv, _ := NewDriver(node, nil)

	_, err := drv.ASPath(context.Background(), 0)
	if err != domain.ErrInvalidTarget {
		t.Errorf("expected ErrInvalidTarget for ASN 0, got %v", err)
	}
}
