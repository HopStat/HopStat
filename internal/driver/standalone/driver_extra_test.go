package standalone

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/HopStat/HopStat/internal/circuitbreaker"
	"github.com/HopStat/HopStat/internal/domain"
)

func withFakeBin(t *testing.T, scripts map[string]string) {
	t.Helper()
	dir := t.TempDir()
	for name, body := range scripts {
		path := filepath.Join(dir, name)
		content := "#!/bin/sh\n" + body + "\n"
		if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func TestGetOnLine(t *testing.T) {
	called := false
	ctx := domain.WithOnLine(context.Background(), func(string) { called = true })
	if fn := getOnLine(ctx); fn == nil {
		t.Fatal("expected callback")
	} else {
		fn("x")
	}
	if !called {
		t.Fatal("callback not invoked")
	}
	if getOnLine(context.Background()) != nil {
		t.Fatal("expected nil without callback")
	}
}

func TestDriverPingSuccess(t *testing.T) {
	withFakeBin(t, map[string]string{
		"ping": `echo "--- 8.8.8.8 ping statistics ---
5 packets transmitted, 5 received, 0% packet loss
rtt min/avg/max = 1.0/2.0/3.0 ms"`,
	})
	node := &domain.Node{EnabledCmds: []domain.CommandType{domain.CmdPing}}
	drv, _ := NewDriver(node, nil)
	result, err := drv.Ping(context.Background(), "8.8.8.8", 5)
	if err != nil {
		t.Fatalf("Ping: %v", err)
	}
	if result.PacketsRecv != 5 {
		t.Fatalf("PacketsRecv = %d", result.PacketsRecv)
	}
}

func TestDriverPingWithStream(t *testing.T) {
	withFakeBin(t, map[string]string{
		"ping": `echo "line1"; echo "line2"`,
	})
	node := &domain.Node{EnabledCmds: []domain.CommandType{domain.CmdPing}}
	drv, _ := NewDriver(node, nil)
	var lines []string
	ctx := domain.WithOnLine(context.Background(), func(line string) {
		lines = append(lines, line)
	})
	_, err := drv.Ping(ctx, "8.8.8.8", 1)
	if err != nil {
		t.Fatalf("Ping stream: %v", err)
	}
	if len(lines) == 0 {
		t.Fatal("expected streamed lines")
	}
}

func TestDriverPingExecErrorParsesOutput(t *testing.T) {
	withFakeBin(t, map[string]string{
		"ping": `echo "--- ping statistics ---
3 packets transmitted, 0 received, 100% packet loss"; exit 1`,
	})
	node := &domain.Node{EnabledCmds: []domain.CommandType{domain.CmdPing}}
	drv, _ := NewDriver(node, nil)
	result, err := drv.Ping(context.Background(), "8.8.8.8", 3)
	if err != nil {
		t.Fatalf("Ping: %v", err)
	}
	if result.PacketLoss != 100 {
		t.Fatalf("PacketLoss = %v", result.PacketLoss)
	}
}

func TestDriverPingExecErrorUnparseable(t *testing.T) {
	withFakeBin(t, map[string]string{
		"ping": `echo '%%%'; exit 1`,
	})
	node := &domain.Node{EnabledCmds: []domain.CommandType{domain.CmdPing}}
	drv, _ := NewDriver(node, nil)
	result, err := drv.Ping(context.Background(), "8.8.8.8", 4)
	if err != nil {
		t.Fatalf("Ping: %v", err)
	}
	if result.PacketLoss != 100 {
		t.Fatalf("PacketLoss = %v", result.PacketLoss)
	}
}

func TestDriverTracerouteSuccessAndError(t *testing.T) {
	withFakeBin(t, map[string]string{
		"traceroute": `echo " 1  1.1.1.1 (1.1.1.1)  1.000 ms"`,
	})
	node := &domain.Node{EnabledCmds: []domain.CommandType{domain.CmdTraceroute}}
	drv, _ := NewDriver(node, nil)
	result, err := drv.Traceroute(context.Background(), "8.8.8.8", 30)
	if err != nil {
		t.Fatalf("Traceroute: %v", err)
	}
	if len(result.Hops) == 0 {
		t.Fatalf("hops = %+v", result)
	}

	withFakeBin(t, map[string]string{
		"traceroute": `echo "failed"; exit 1`,
	})
	drv, _ = NewDriver(node, nil)
	result, err = drv.Traceroute(context.Background(), "8.8.8.8", 30)
	if err != nil {
		t.Fatalf("Traceroute error path: %v", err)
	}
	if result.Raw == "" {
		t.Fatal("expected raw output on error")
	}
}

func TestDriverTracerouteStream(t *testing.T) {
	withFakeBin(t, map[string]string{
		"traceroute": `echo " 1  1.1.1.1 (1.1.1.1)  1.000 ms"`,
	})
	node := &domain.Node{EnabledCmds: []domain.CommandType{domain.CmdTraceroute}}
	drv, _ := NewDriver(node, nil)
	var lines []string
	ctx := domain.WithOnLine(context.Background(), func(line string) {
		lines = append(lines, line)
	})
	result, err := drv.Traceroute(ctx, "8.8.8.8", 30)
	if err != nil {
		t.Fatalf("Traceroute stream: %v", err)
	}
	if len(lines) == 0 {
		t.Fatal("expected streamed lines")
	}
	if len(result.Hops) == 0 {
		t.Fatal("expected parsed hops")
	}
}

func TestDriverTracerouteDNSAndInvalid(t *testing.T) {
	node := &domain.Node{EnabledCmds: []domain.CommandType{domain.CmdTraceroute}}
	drv, _ := NewDriver(node, nil)
	if _, err := drv.Traceroute(context.Background(), "127.0.0.1", 5); err != domain.ErrInvalidTarget {
		t.Fatalf("err = %v", err)
	}
	if _, err := drv.Traceroute(context.Background(), "missing.invalid.example", 5); err != domain.ErrDNSNotFound {
		t.Fatalf("err = %v", err)
	}
}

func TestDriverBGPRouteVtysh(t *testing.T) {
	withFakeBin(t, map[string]string{
		"vtysh": `echo "BGP table version is 1, local router ID is 1.1.1.1
Status codes: s suppressed, d damped, h history, * valid, > best, i internal
   Network          Next Hop            Metric LocPrf Weight Path
*> 1.1.1.0/24       10.0.0.1                 0             65001 i"`,
	})
	node := &domain.Node{EnabledCmds: []domain.CommandType{domain.CmdBGPRoute}}
	drv, _ := NewDriver(node, nil)
	result, err := drv.BGPRoute(context.Background(), "1.1.1.0/24")
	if err != nil {
		t.Fatalf("BGPRoute vtysh: %v", err)
	}
	if len(result.Routes) == 0 {
		t.Fatalf("routes = %+v", result)
	}
}

func TestDriverBGPRouteBird(t *testing.T) {
	withFakeBin(t, map[string]string{
		"birdc": `cat <<'EOF'
* 8.8.8.0/24              10.183.1.25                  100        204457 15169 I
EOF`,
	})
	node := &domain.Node{EnabledCmds: []domain.CommandType{domain.CmdBGPRoute}}
	drv, _ := NewDriver(node, nil)
	result, err := drv.BGPRoute(context.Background(), "8.8.8.0/24")
	if err != nil {
		t.Fatalf("BGPRoute bird: %v", err)
	}
	if len(result.Routes) == 0 {
		t.Fatalf("routes = %+v raw=%q", result.Routes, result.Raw)
	}
}

func TestDriverBGPRouteBirdEmptyParse(t *testing.T) {
	withFakeBin(t, map[string]string{
		"birdc": `echo "not bird output"`,
		"vtysh": `echo "BGP table version is 1
   Network          Next Hop
*> 1.1.1.0/24       10.0.0.1                 0             65001 i"`,
	})
	node := &domain.Node{EnabledCmds: []domain.CommandType{domain.CmdBGPRoute}}
	drv, _ := NewDriver(node, nil)
	result, err := drv.BGPRoute(context.Background(), "1.1.1.0/24")
	if err != nil {
		t.Fatalf("BGPRoute: %v", err)
	}
	if len(result.Routes) == 0 {
		t.Fatalf("expected vtysh fallback routes, got %+v", result)
	}
}

func TestDriverBGPRouteNoData(t *testing.T) {
	withFakeBin(t, map[string]string{
		"birdc": "exit 1",
		"vtysh": "exit 1",
	})
	node := &domain.Node{EnabledCmds: []domain.CommandType{domain.CmdBGPRoute}}
	drv, _ := NewDriver(node, nil)
	result, err := drv.BGPRoute(context.Background(), "1.1.1.1")
	if err != nil {
		t.Fatalf("BGPRoute: %v", err)
	}
	if result.Raw == "" {
		t.Fatal("expected raw fallback")
	}
}

func TestExecCmdCircuitOpen(t *testing.T) {
	withFakeBin(t, map[string]string{
		"false": "exit 1",
	})
	node := &domain.Node{EnabledCmds: []domain.CommandType{domain.CmdPing}}
	drv, _ := NewDriver(node, nil)
	for i := 0; i < 5; i++ {
		_, _ = drv.execCmd(context.Background(), "false")
	}
	_, err := drv.execCmd(context.Background(), "false")
	if err != circuitbreaker.ErrCircuitOpen {
		t.Fatalf("err = %v", err)
	}
}

func TestExecCmdStreamCircuitOpen(t *testing.T) {
	withFakeBin(t, map[string]string{
		"false": "exit 1",
	})
	node := &domain.Node{EnabledCmds: []domain.CommandType{domain.CmdPing}}
	drv, _ := NewDriver(node, nil)
	for i := 0; i < 5; i++ {
		_, _ = drv.execCmdStream(context.Background(), func(string) {}, "false")
	}
	_, err := drv.execCmdStream(context.Background(), func(string) {}, "false")
	if err != circuitbreaker.ErrCircuitOpen {
		t.Fatalf("err = %v", err)
	}
}

func TestDriverPingZeroParsedStats(t *testing.T) {
	withFakeBin(t, map[string]string{
		"ping": `echo "--- ping statistics ---
0 packets transmitted, 0 received, 0% packet loss"; exit 1`,
	})
	node := &domain.Node{EnabledCmds: []domain.CommandType{domain.CmdPing}}
	drv, _ := NewDriver(node, nil)
	result, err := drv.Ping(context.Background(), "8.8.8.8", 6)
	if err != nil {
		t.Fatalf("Ping: %v", err)
	}
	if result.PacketsSent != 6 || result.PacketLoss != 100 {
		t.Fatalf("result = %#v", result)
	}
}

func TestDriverBGPRouteHostname(t *testing.T) {
	withFakeBin(t, map[string]string{
		"birdc": "exit 1",
		"vtysh": "exit 1",
	})
	node := &domain.Node{EnabledCmds: []domain.CommandType{domain.CmdBGPRoute}}
	drv, _ := NewDriver(node, nil)
	result, err := drv.BGPRoute(context.Background(), "example.com")
	if err != nil {
		t.Fatalf("BGPRoute hostname: %v", err)
	}
	if result.Raw == "" {
		t.Fatal("expected raw fallback")
	}
}

func TestExecCmdStreamStartError(t *testing.T) {
	node := &domain.Node{EnabledCmds: []domain.CommandType{domain.CmdPing}}
	drv, _ := NewDriver(node, nil)
	out, err := drv.execCmdStream(context.Background(), func(string) {}, "/no/such/command")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "" {
		t.Fatalf("expected empty output, got %q", out)
	}
}

func TestDriverBGPRouteBirdNoRoutes(t *testing.T) {
	withFakeBin(t, map[string]string{
		"birdc": `echo "Network not in table"`,
		"vtysh": "exit 1",
	})
	node := &domain.Node{EnabledCmds: []domain.CommandType{domain.CmdBGPRoute}}
	drv, _ := NewDriver(node, nil)
	result, err := drv.BGPRoute(context.Background(), "8.8.8.0/24")
	if err != nil {
		t.Fatalf("BGPRoute: %v", err)
	}
	if !strings.Contains(result.Raw, "no BGP data") {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestDriverBGPRouteBlockedHostname(t *testing.T) {
	node := &domain.Node{EnabledCmds: []domain.CommandType{domain.CmdBGPRoute}}
	drv, _ := NewDriver(node, nil)
	_, err := drv.BGPRoute(context.Background(), "localhost")
	if err != domain.ErrInvalidTarget {
		t.Fatalf("expected ErrInvalidTarget, got %v", err)
	}
}

func TestDriverPingWithStderrStream(t *testing.T) {
	withFakeBin(t, map[string]string{
		"ping": `echo "stderr line" 1>&2; echo "--- 8.8.8.8 ping statistics ---"; echo "1 packets transmitted, 1 received, 0% packet loss"`,
	})
	node := &domain.Node{EnabledCmds: []domain.CommandType{domain.CmdPing}}
	drv, _ := NewDriver(node, nil)
	var lines []string
	ctx := domain.WithOnLine(context.Background(), func(line string) {
		lines = append(lines, line)
	})
	_, err := drv.Ping(ctx, "8.8.8.8", 1)
	if err != nil {
		t.Fatalf("Ping stream: %v", err)
	}
	if len(lines) < 2 {
		t.Fatalf("expected stdout and stderr lines, got %#v", lines)
	}
}

func TestExecCmdStreamStdoutPipeError(t *testing.T) {
	orig := commandContext
	t.Cleanup(func() { commandContext = orig })
	commandContext = func(ctx context.Context, name string, arg ...string) *exec.Cmd {
		cmd := exec.CommandContext(ctx, name, arg...)
		cmd.Stdout = os.Stdout
		return cmd
	}
	node := &domain.Node{EnabledCmds: []domain.CommandType{domain.CmdPing}}
	drv, _ := NewDriver(node, nil)
	_, _ = drv.execCmdStream(context.Background(), func(string) {}, "echo", "x")
}

func TestExecCmdStreamStderrPipeError(t *testing.T) {
	orig := commandContext
	t.Cleanup(func() { commandContext = orig })
	commandContext = func(ctx context.Context, name string, arg ...string) *exec.Cmd {
		cmd := exec.CommandContext(ctx, name, arg...)
		cmd.Stderr = os.Stderr
		return cmd
	}
	node := &domain.Node{EnabledCmds: []domain.CommandType{domain.CmdPing}}
	drv, _ := NewDriver(node, nil)
	_, _ = drv.execCmdStream(context.Background(), func(string) {}, "echo", "x")
}

func TestDriverPingCircuitStream(t *testing.T) {
	withFakeBin(t, map[string]string{
		"ping": fmt.Sprintf("sleep %d; exit 1", 0),
	})
	node := &domain.Node{EnabledCmds: []domain.CommandType{domain.CmdPing}}
	drv, _ := NewDriver(node, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	ctx = domain.WithOnLine(ctx, func(string) {})
	_, _ = drv.Ping(ctx, "8.8.8.8", 1)
}
