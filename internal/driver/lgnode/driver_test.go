package lgnode

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/yourorg/lg-looking-glass/internal/domain"
)

func TestNewDriver(t *testing.T) {
	node := &domain.Node{
		Name:       "test-agent",
		Type:       domain.NodeTypeLGNode,
		AgentURL:   "http://localhost:9090",
		AgentToken: "test-token",
	}
	drv, err := NewDriver(node, nil)
	if err != nil {
		t.Fatalf("NewDriver error: %v", err)
	}
	if drv == nil {
		t.Fatal("expected non-nil driver")
	}
}

func TestDriverTestConnection(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/agent/v1/health" {
			json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	node := &domain.Node{
		Type:        domain.NodeTypeLGNode,
		AgentURL:    server.URL,
		AgentToken:  "test-token",
		EnabledCmds: []domain.CommandType{domain.CmdPing},
	}
	drv, _ := NewDriver(node, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := drv.TestConnection(ctx); err != nil {
		t.Errorf("TestConnection error: %v", err)
	}
}

func TestDriverTestConnectionFailure(t *testing.T) {
	node := &domain.Node{
		Type:        domain.NodeTypeLGNode,
		AgentURL:    "http://127.0.0.1:1",
		AgentToken:  "test-token",
		EnabledCmds: []domain.CommandType{domain.CmdPing},
	}
	drv, _ := NewDriver(node, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := drv.TestConnection(ctx); err == nil {
		t.Error("expected error for unreachable server")
	}
}

func TestDriverPing(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-token" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		json.NewEncoder(w).Encode(domain.PingResult{
			PacketsSent: 5,
			PacketsRecv: 5,
			PacketLoss:  0,
			MinRTT:      1.0,
			AvgRTT:      2.0,
			MaxRTT:      3.0,
			Raw:         "ping output",
		})
	}))
	defer server.Close()

	node := &domain.Node{
		Type:        domain.NodeTypeLGNode,
		AgentURL:    server.URL,
		AgentToken:  "test-token",
		EnabledCmds: []domain.CommandType{domain.CmdPing},
	}
	drv, _ := NewDriver(node, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result, err := drv.Ping(ctx, "8.8.8.8", 5)
	if err != nil {
		t.Fatalf("Ping error: %v", err)
	}
	if result.PacketsSent != 5 {
		t.Errorf("expected 5 sent, got %d", result.PacketsSent)
	}
}

func TestDriverPingDisabled(t *testing.T) {
	node := &domain.Node{
		Type:        domain.NodeTypeLGNode,
		AgentURL:    "http://localhost:9090",
		AgentToken:  "test-token",
		EnabledCmds: []domain.CommandType{},
	}
	drv, _ := NewDriver(node, nil)

	_, err := drv.Ping(context.Background(), "8.8.8.8", 5)
	if err != domain.ErrCommandDisabled {
		t.Errorf("expected ErrCommandDisabled, got %v", err)
	}
}

func TestDriverCapabilities(t *testing.T) {
	cmds := []domain.CommandType{domain.CmdPing, domain.CmdTraceroute}
	node := &domain.Node{EnabledCmds: cmds}
	drv, _ := NewDriver(node, nil)

	caps := drv.Capabilities()
	if len(caps) != 2 {
		t.Errorf("expected 2 capabilities, got %d", len(caps))
	}
}
