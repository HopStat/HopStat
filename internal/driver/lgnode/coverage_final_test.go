package lgnode

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/HopStat/HopStat/internal/domain"
)

func TestResolveLocalAgentURLLocalRewrite(t *testing.T) {
	old := interfaceAddrs
	interfaceAddrs = func() ([]net.Addr, error) {
		return []net.Addr{&net.IPNet{IP: net.ParseIP("192.168.50.1"), Mask: net.CIDRMask(32, 32)}}, nil
	}
	t.Cleanup(func() { interfaceAddrs = old })

	got := resolveLocalAgentURL("http://192.168.50.1:8080/path")
	if !strings.Contains(got, "127.0.0.1:8080") {
		t.Fatalf("got = %q", got)
	}
	if got := resolveLocalAgentURL("https://192.168.50.1/path"); !strings.Contains(got, "127.0.0.1:443") {
		t.Fatalf("got = %q", got)
	}
	if got := resolveLocalAgentURL("http://127.0.0.1:8080"); got != "http://127.0.0.1:8080" {
		t.Fatalf("got = %q", got)
	}
}

func TestIsLocalHostBranches(t *testing.T) {
	if !isLocalHost("127.0.0.1") {
		t.Fatal("expected loopback")
	}
	old := interfaceAddrs
	interfaceAddrs = func() ([]net.Addr, error) { return nil, errors.New("ifaces failed") }
	t.Cleanup(func() { interfaceAddrs = old })
	if isLocalHost("192.168.1.1") {
		t.Fatal("expected false when ifaces fail")
	}
	interfaceAddrs = func() ([]net.Addr, error) {
		return []net.Addr{&net.IPNet{IP: net.ParseIP("10.0.0.5"), Mask: net.CIDRMask(32, 32)}}, nil
	}
	if !isLocalHost("10.0.0.5") {
		t.Fatal("expected local interface match")
	}
}

func TestDriverTestConnectionSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	node := &domain.Node{Type: domain.NodeTypeLGNode, AgentURL: server.URL, AgentToken: "tok"}
	drv, _ := NewDriver(node, nil)
	if err := drv.TestConnection(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestDriverDoAgentRequestDecodeError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "not-json")
	}))
	defer server.Close()
	node := &domain.Node{
		Type: domain.NodeTypeLGNode, AgentURL: server.URL, AgentToken: "tok",
		EnabledCmds: []domain.CommandType{domain.CmdBGPRoute},
	}
	drv, _ := NewDriver(node, nil)
	_, err := drv.BGPRoute(context.Background(), "10.0.0.0/24")
	if err == nil {
		t.Fatal("expected decode error")
	}
}

func TestDriverDoAgentRequestNetworkError(t *testing.T) {
	node := &domain.Node{
		Type: domain.NodeTypeLGNode, AgentURL: "http://127.0.0.1:1", AgentToken: "tok",
		EnabledCmds: []domain.CommandType{domain.CmdBGPRoute},
	}
	drv, _ := NewDriver(node, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	_, err := drv.BGPRoute(ctx, "10.0.0.0/24")
	if err == nil {
		t.Fatal("expected network error")
	}
}

func TestDriverStreamInvalidResultJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "event: result\ndata: not-json\n\n")
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
		t.Fatal("expected invalid result json")
	}
}

func TestReadSSEScannerError(t *testing.T) {
	err := readSSE(badReader{}, func(string, string) error { return nil })
	if err == nil {
		t.Fatal("expected scanner error")
	}
}

func TestReadSSEOnEventError(t *testing.T) {
	err := readSSE(strings.NewReader("event: output\ndata: {\"line\":\"x\"}\n\n"), func(string, string) error {
		return errors.New("handler failed")
	})
	if err == nil {
		t.Fatal("expected handler error")
	}
}

func TestDriverStreamEmptyEventWithLine(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		payload, _ := json.Marshal(domain.PingResult{PacketsRecv: 1})
		fmt.Fprintf(w, "data: {\"line\":\"hop\"}\n\nevent: result\ndata: %s\n\n", payload)
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
		t.Fatalf("result=%+v lines=%v err=%v", result, lines, err)
	}
}

func TestDriverTestConnectionInvalidURL(t *testing.T) {
	node := &domain.Node{Type: domain.NodeTypeLGNode, AgentURL: "://bad", AgentToken: "tok"}
	drv, _ := NewDriver(node, nil)
	if err := drv.TestConnection(context.Background()); err == nil {
		t.Fatal("expected invalid url error")
	}
}

func TestIsLocalHostNonIPNetAddr(t *testing.T) {
	old := interfaceAddrs
	interfaceAddrs = func() ([]net.Addr, error) {
		return []net.Addr{mockAddr{}}, nil
	}
	t.Cleanup(func() { interfaceAddrs = old })
	if isLocalHost("192.168.1.1") {
		t.Fatal("expected false for non-ipnet addr")
	}
}

type mockAddr struct{}

func (mockAddr) Network() string { return "mock" }
func (mockAddr) String() string  { return "192.168.1.1" }

func TestResolveLocalAgentURLExternal(t *testing.T) {
	got := resolveLocalAgentURL("http://example.com:8080/x")
	if got != "http://example.com:8080/x" {
		t.Fatalf("got=%q", got)
	}
}

func TestDriverDoAgentRequestMarshalError(t *testing.T) {
	old := jsonMarshal
	jsonMarshal = func(v interface{}) ([]byte, error) { return nil, errors.New("marshal failed") }
	t.Cleanup(func() { jsonMarshal = old })

	node := &domain.Node{
		Type: domain.NodeTypeLGNode, AgentURL: "http://127.0.0.1:1", AgentToken: "tok",
		EnabledCmds: []domain.CommandType{domain.CmdBGPRoute},
	}
	drv, _ := NewDriver(node, nil)
	_, err := drv.BGPRoute(context.Background(), "10.0.0.0/24")
	if err == nil {
		t.Fatal("expected marshal error")
	}
}

func TestDriverDoAgentRequestNewRequestError(t *testing.T) {
	old := newHTTPRequest
	newHTTPRequest = func(context.Context, string, string, io.Reader) (*http.Request, error) {
		return nil, errors.New("new request failed")
	}
	t.Cleanup(func() { newHTTPRequest = old })

	node := &domain.Node{
		Type: domain.NodeTypeLGNode, AgentURL: "http://127.0.0.1:1", AgentToken: "tok",
		EnabledCmds: []domain.CommandType{domain.CmdBGPRoute},
	}
	drv, _ := NewDriver(node, nil)
	_, err := drv.BGPRoute(context.Background(), "10.0.0.0/24")
	if err == nil {
		t.Fatal("expected new request error")
	}
}

func TestResolveLocalAgentURLBracketedIPv6(t *testing.T) {
	got := resolveLocalAgentURL("http://[::1]:8080/health")
	if got != "http://[::1]:8080/health" {
		t.Fatalf("got=%q", got)
	}
}

type badReader struct{}

func (badReader) Read([]byte) (int, error) { return 0, errors.New("read failed") }

func TestDriverStreamMarshalError(t *testing.T) {
	old := jsonMarshal
	jsonMarshal = func(v interface{}) ([]byte, error) { return nil, errors.New("marshal failed") }
	t.Cleanup(func() { jsonMarshal = old })

	node := &domain.Node{
		Type: domain.NodeTypeLGNode, AgentURL: "http://127.0.0.1:1", AgentToken: "tok",
		EnabledCmds: []domain.CommandType{domain.CmdPing},
	}
	drv, _ := NewDriver(node, nil)
	ctx := domain.WithOnLine(context.Background(), func(string) {})
	_, err := drv.Ping(ctx, "8.8.8.8", 1)
	if err == nil {
		t.Fatal("expected marshal error")
	}
}

func TestDriverStreamNewRequestError(t *testing.T) {
	old := newHTTPRequest
	newHTTPRequest = func(context.Context, string, string, io.Reader) (*http.Request, error) {
		return nil, errors.New("new request failed")
	}
	t.Cleanup(func() { newHTTPRequest = old })

	node := &domain.Node{
		Type: domain.NodeTypeLGNode, AgentURL: "http://127.0.0.1:1", AgentToken: "tok",
		EnabledCmds: []domain.CommandType{domain.CmdPing},
	}
	drv, _ := NewDriver(node, nil)
	ctx := domain.WithOnLine(context.Background(), func(string) {})
	_, err := drv.Ping(ctx, "8.8.8.8", 1)
	if err == nil {
		t.Fatal("expected new request error")
	}
}

func TestDriverStreamNetworkError(t *testing.T) {
	node := &domain.Node{
		Type: domain.NodeTypeLGNode, AgentURL: "http://127.0.0.1:1", AgentToken: "tok",
		EnabledCmds: []domain.CommandType{domain.CmdPing},
	}
	drv, _ := NewDriver(node, nil)
	ctx, cancel := context.WithTimeout(domain.WithOnLine(context.Background(), func(string) {}), 200*time.Millisecond)
	defer cancel()
	_, err := drv.Ping(ctx, "8.8.8.8", 1)
	if err == nil {
		t.Fatal("expected stream network error")
	}
}

func TestDriverStreamSkipsNonLineData(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: ping stats\n\n")
		payload, _ := json.Marshal(domain.PingResult{PacketsRecv: 1})
		fmt.Fprintf(w, "event: result\ndata: %s\n\n", payload)
	}))
	defer server.Close()
	node := &domain.Node{
		Type: domain.NodeTypeLGNode, AgentURL: server.URL, AgentToken: "tok",
		EnabledCmds: []domain.CommandType{domain.CmdPing},
	}
	drv, _ := NewDriver(node, nil)
	ctx := domain.WithOnLine(context.Background(), func(string) {})
	result, err := drv.Ping(ctx, "8.8.8.8", 1)
	if err != nil || result.PacketsRecv != 1 {
		t.Fatalf("result=%+v err=%v", result, err)
	}
}

func TestDriverStreamInvalidOutputJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "event: output\ndata: {\"line\":1}\n\n")
		payload, _ := json.Marshal(domain.PingResult{PacketsRecv: 1})
		fmt.Fprintf(w, "event: result\ndata: %s\n\n", payload)
	}))
	defer server.Close()
	node := &domain.Node{
		Type: domain.NodeTypeLGNode, AgentURL: server.URL, AgentToken: "tok",
		EnabledCmds: []domain.CommandType{domain.CmdPing},
	}
	drv, _ := NewDriver(node, nil)
	ctx := domain.WithOnLine(context.Background(), func(string) {})
	result, err := drv.Ping(ctx, "8.8.8.8", 1)
	if err != nil || result.PacketsRecv != 1 {
		t.Fatalf("result=%+v err=%v", result, err)
	}
}

func TestResolveLocalAgentURLDefaultHTTPPort(t *testing.T) {
	old := interfaceAddrs
	interfaceAddrs = func() ([]net.Addr, error) {
		return []net.Addr{&net.IPNet{IP: net.ParseIP("192.168.50.1"), Mask: net.CIDRMask(32, 32)}}, nil
	}
	t.Cleanup(func() { interfaceAddrs = old })

	got := resolveLocalAgentURL("http://192.168.50.1/path")
	if !strings.Contains(got, "127.0.0.1:80") {
		t.Fatalf("got = %q", got)
	}
}
