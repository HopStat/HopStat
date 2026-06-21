package lgnode

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/HopStat/HopStat/internal/domain"
)

func TestDriverTraceroute(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/agent/v1/traceroute" {
			json.NewEncoder(w).Encode(domain.TracerouteResult{
				Hops: []domain.Hop{{Number: 1, IP: "1.1.1.1"}},
				Raw:  "trace",
			})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	node := &domain.Node{
		Type: domain.NodeTypeLGNode, AgentURL: server.URL, AgentToken: "tok",
		EnabledCmds: []domain.CommandType{domain.CmdTraceroute},
	}
	drv, _ := NewDriver(node, nil)
	result, err := drv.Traceroute(context.Background(), "8.8.8.8", 30)
	if err != nil || len(result.Hops) != 1 {
		t.Fatalf("Traceroute = %+v err %v", result, err)
	}
}

func TestDriverTracerouteStreamFallback(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/agent/v1/traceroute/stream" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if r.URL.Path == "/agent/v1/traceroute" {
			json.NewEncoder(w).Encode(domain.TracerouteResult{Raw: "ok"})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	node := &domain.Node{
		Type: domain.NodeTypeLGNode, AgentURL: server.URL, AgentToken: "tok",
		EnabledCmds: []domain.CommandType{domain.CmdTraceroute},
	}
	drv, _ := NewDriver(node, nil)
	ctx := domain.WithOnLine(context.Background(), func(string) {})
	result, err := drv.Traceroute(ctx, "8.8.8.8", 30)
	if err != nil || result.Raw != "ok" {
		t.Fatalf("Traceroute fallback = %+v err %v", result, err)
	}
}

func TestDriverBGPRoute(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(domain.BGPResult{Raw: "bgp"})
	}))
	defer server.Close()

	node := &domain.Node{
		Type: domain.NodeTypeLGNode, AgentURL: server.URL, AgentToken: "tok",
		EnabledCmds: []domain.CommandType{domain.CmdBGPRoute},
	}
	drv, _ := NewDriver(node, nil)
	result, err := drv.BGPRoute(context.Background(), "8.8.8.0/24")
	if err != nil || result.Raw != "bgp" {
		t.Fatalf("BGPRoute = %+v err %v", result, err)
	}
}

func TestDriverStreamErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)
		fmt.Fprintf(w, "event: error\ndata: {\"error\":\"boom\"}\n\n")
		flusher.Flush()
	}))
	defer server.Close()

	node := &domain.Node{
		Type: domain.NodeTypeLGNode, AgentURL: server.URL, AgentToken: "tok",
		EnabledCmds: []domain.CommandType{domain.CmdPing},
	}
	drv, _ := NewDriver(node, nil)
	ctx := domain.WithOnLine(context.Background(), func(string) {})
	_, err := drv.Ping(ctx, "8.8.8.8", 1)
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("err = %v", err)
	}
}

func TestDriverStreamNoResult(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "event: output\ndata: {\"line\":\"only\"}\n\n")
	}))
	defer server.Close()

	node := &domain.Node{
		Type: domain.NodeTypeLGNode, AgentURL: server.URL, AgentToken: "tok",
		EnabledCmds: []domain.CommandType{domain.CmdPing},
	}
	drv, _ := NewDriver(node, nil)
	ctx := domain.WithOnLine(context.Background(), func(string) {})
	_, err := drv.Ping(ctx, "8.8.8.8", 1)
	if err == nil || !strings.Contains(err.Error(), "without result") {
		t.Fatalf("err = %v", err)
	}
}

func TestDriverAgentRequestFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	node := &domain.Node{
		Type: domain.NodeTypeLGNode, AgentURL: server.URL, AgentToken: "tok",
		EnabledCmds: []domain.CommandType{domain.CmdPing},
	}
	drv, _ := NewDriver(node, nil)
	_, err := drv.Ping(context.Background(), "8.8.8.8", 1)
	if err == nil {
		t.Fatal("expected agent failure")
	}
}

func TestDriverTracerouteDisabled(t *testing.T) {
	node := &domain.Node{Type: domain.NodeTypeLGNode, EnabledCmds: nil}
	drv, _ := NewDriver(node, nil)
	if _, err := drv.Traceroute(context.Background(), "8.8.8.8", 30); err != domain.ErrCommandDisabled {
		t.Fatalf("err = %v", err)
	}
}

func TestDriverBGPRouteDisabled(t *testing.T) {
	node := &domain.Node{Type: domain.NodeTypeLGNode, EnabledCmds: nil}
	drv, _ := NewDriver(node, nil)
	if _, err := drv.BGPRoute(context.Background(), "8.8.8.0/24"); err != domain.ErrCommandDisabled {
		t.Fatalf("err = %v", err)
	}
}

func TestParseOutputLineInvalid(t *testing.T) {
	if _, err := parseOutputLine("not-json"); err == nil {
		t.Fatal("expected parse error")
	}
}

func TestParseStreamErrorInvalid(t *testing.T) {
	err := parseStreamError("not-json")
	if err == nil || err.Error() != "agent stream failed" {
		t.Fatalf("err = %v", err)
	}
	err = parseStreamError(`{"error":""}`)
	if err == nil || err.Error() != "agent stream failed" {
		t.Fatalf("err = %v", err)
	}
}

func TestReadSSE_DefaultEvent(t *testing.T) {
	body := "data: {\"line\":\"x\"}\n\nevent: output\ndata: {\"line\":\"y\"}\n\n"
	var lines []string
	if err := readSSE(strings.NewReader(body), func(event, data string) error {
		if event == "output" || event == "" {
			line, _ := parseOutputLine(data)
			if line != "" {
				lines = append(lines, line)
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if len(lines) < 1 {
		t.Fatalf("lines = %v", lines)
	}
}

func TestResolveLocalAgentURLHTTPS(t *testing.T) {
	got := resolveLocalAgentURL("https://example.com")
	if got != "https://example.com" {
		t.Fatalf("got = %q", got)
	}
	got = resolveLocalAgentURL("://bad")
	if got != "://bad" {
		t.Fatalf("invalid url = %q", got)
	}
}

func TestIsLocalHostNonIP(t *testing.T) {
	if isLocalHost("not-an-ip") {
		t.Fatal("expected false")
	}
}

func TestDriverTestConnectionBadStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()
	node := &domain.Node{
		Type: domain.NodeTypeLGNode, AgentURL: server.URL, AgentToken: "tok",
		EnabledCmds: []domain.CommandType{domain.CmdPing},
	}
	drv, _ := NewDriver(node, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := drv.TestConnection(ctx); err == nil {
		t.Fatal("expected health failure")
	}
}

func TestDriverPingStreamSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)
		fmt.Fprintf(w, "event: output\ndata: {\"line\":\"hop\"}\n\n")
		flusher.Flush()
		payload, _ := json.Marshal(domain.PingResult{PacketsRecv: 1, Raw: "hop"})
		fmt.Fprintf(w, "event: result\ndata: %s\n\n", payload)
		flusher.Flush()
	}))
	defer server.Close()

	node := &domain.Node{
		Type: domain.NodeTypeLGNode, AgentURL: server.URL, AgentToken: "tok",
		EnabledCmds: []domain.CommandType{domain.CmdPing},
	}
	drv, _ := NewDriver(node, nil)
	var lines []string
	ctx := domain.WithOnLine(context.Background(), func(line string) { lines = append(lines, line) })
	result, err := drv.Ping(ctx, "8.8.8.8", 1)
	if err != nil || result.PacketsRecv != 1 || len(lines) != 1 {
		t.Fatalf("Ping stream = %+v lines=%v err=%v", result, lines, err)
	}
}

func TestDriverStreamBadStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer server.Close()
	node := &domain.Node{
		Type: domain.NodeTypeLGNode, AgentURL: server.URL, AgentToken: "tok",
		EnabledCmds: []domain.CommandType{domain.CmdPing},
	}
	drv, _ := NewDriver(node, nil)
	ctx := domain.WithOnLine(context.Background(), func(string) {})
	_, err := drv.Ping(ctx, "8.8.8.8", 1)
	if err == nil {
		t.Fatal("expected stream failure")
	}
}
