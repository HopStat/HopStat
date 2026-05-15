package standalone

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"net"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/HopStat/HopStat/internal/circuitbreaker"
	"github.com/HopStat/HopStat/internal/config"
	"github.com/HopStat/HopStat/internal/domain"
	"github.com/HopStat/HopStat/internal/parser"
)

func getOnLine(ctx context.Context) func(string) {
	return domain.GetOnLine(ctx)
}

type Driver struct {
	node           *domain.Node
	cfg            *config.Config
	parser         parser.OutputParser
	circuitBreaker *circuitbreaker.CircuitBreaker
}

func NewDriver(node *domain.Node, cfg *config.Config) (*Driver, error) {
	return &Driver{
		node:           node,
		cfg:            cfg,
		parser:         parser.GetParser("generic"),
		circuitBreaker: circuitbreaker.New(5, 30*time.Second),
	}, nil
}

// resolveTarget resolves a hostname to an IP. If target is already an IP, returns it unchanged.
// All resolved addresses are checked against the blocklist to prevent DNS-based bypasses.
func resolveTarget(ctx context.Context, target string) (string, error) {
	if ip := net.ParseIP(target); ip != nil {
		if isBlockedIP(ip) {
			return "", fmt.Errorf("target %s is not allowed", target)
		}
		return target, nil
	}
	resolver := net.Resolver{}
	ips, err := resolver.LookupIPAddr(ctx, target)
	if err != nil || len(ips) == 0 {
		return "", fmt.Errorf("cannot resolve %s: %w", target, err)
	}
	for _, addr := range ips {
		if isBlockedIP(addr.IP) {
			return "", fmt.Errorf("resolved target contains blocked address %s", addr.IP)
		}
	}
	return ips[0].IP.String(), nil
}

func isBlockedIP(ip net.IP) bool {
	if ip.IsLoopback() || ip.IsUnspecified() || ip.IsPrivate() {
		return true
	}
	if ip.IsLinkLocalUnicast() || ip.IsMulticast() {
		return true
	}
	if ip4 := ip.To4(); ip4 != nil {
		// Link-local IPv4
		if ip4[0] == 169 && ip4[1] == 254 {
			return true
		}
		// 0.0.0.0/8
		if ip4[0] == 0 {
			return true
		}
		// CGNAT / Shared Address Space (RFC 6598) — 100.64.0.0/10
		if ip4[0] == 100 && ip4[1] >= 64 && ip4[1] <= 127 {
			return true
		}
	}
	return false
}

func (d *Driver) Capabilities() []domain.CommandType {
	return d.node.EnabledCmds
}

func (d *Driver) TestConnection(ctx context.Context) error {
	return d.circuitBreaker.Call(func() error {
		return nil
	})
}

func (d *Driver) Ping(ctx context.Context, target string, count int) (*domain.PingResult, error) {
	if !d.node.CanExecute(domain.CmdPing) {
		return nil, domain.ErrCommandDisabled
	}
	resolved, err := resolveTarget(ctx, target)
	if err != nil {
		return nil, domain.ErrInvalidTarget
	}

	out, err := d.execOrStream(ctx, "ping", "-c", fmt.Sprint(count), "-W", "2", resolved)
	if err != nil {
		return &domain.PingResult{Raw: out, PacketsSent: count, PacketLoss: 100}, nil
	}
	return d.parser.ParsePing(out)
}

func (d *Driver) Traceroute(ctx context.Context, target string, maxHops int) (*domain.TracerouteResult, error) {
	if !d.node.CanExecute(domain.CmdTraceroute) {
		return nil, domain.ErrCommandDisabled
	}
	resolved, err := resolveTarget(ctx, target)
	if err != nil {
		return nil, domain.ErrInvalidTarget
	}

	out, err := d.execOrStream(ctx, "traceroute", "-m", fmt.Sprint(maxHops), "-w", "1", resolved)
	if err != nil {
		return &domain.TracerouteResult{Raw: out}, nil
	}
	return d.parser.ParseTraceroute(out)
}

func (d *Driver) MTR(ctx context.Context, target string, cycles int) (*domain.MTRResult, error) {
	if !d.node.CanExecute(domain.CmdMTR) {
		return nil, domain.ErrCommandDisabled
	}
	resolved, err := resolveTarget(ctx, target)
	if err != nil {
		return nil, domain.ErrInvalidTarget
	}

	out, err := d.execOrStream(ctx, "mtr", "-r", "-c", fmt.Sprint(cycles), resolved)
	if err != nil {
		return &domain.MTRResult{Raw: out}, nil
	}
	return d.parser.ParseMTR(out)
}

func (d *Driver) BGPRoute(ctx context.Context, prefix string) (*domain.BGPResult, error) {
	if !d.node.CanExecute(domain.CmdBGPRoute) {
		return nil, domain.ErrCommandDisabled
	}
	// Resolve if it's a hostname (only for non-CIDR targets)
	if !strings.Contains(prefix, "/") && net.ParseIP(prefix) == nil {
		resolved, err := resolveTarget(ctx, prefix)
		if err != nil {
			return nil, domain.ErrInvalidTarget
		}
		prefix = resolved
	} else if strings.Contains(prefix, "/") {
		if _, _, err := net.ParseCIDR(prefix); err != nil {
			return nil, domain.ErrInvalidTarget
		}
	}

	if out, err := d.execCmd(ctx, "birdc", "show", "route", "for", prefix); err == nil && out != "" {
		p := parser.GetParser("bird")
		result, parseErr := p.ParseBGPRoute(out)
		if parseErr == nil && len(result.Routes) > 0 {
			return result, nil
		}
	}

	if out, err := d.execCmd(ctx, "vtysh", "-c", fmt.Sprintf("show ip bgp %s", prefix)); err == nil && out != "" {
		p := parser.GetParser("cisco")
		return p.ParseBGPRoute(out)
	}

	return nil, fmt.Errorf("no BGP routing daemon available (tried birdc, vtysh)")
}

func (d *Driver) ASPath(ctx context.Context, asn uint32) (*domain.ASPathResult, error) {
	if !d.node.CanExecute(domain.CmdASPath) {
		return nil, domain.ErrCommandDisabled
	}
	if asn < 1 || asn > 4294967295 {
		return nil, domain.ErrInvalidTarget
	}

	if out, err := d.execCmd(ctx, "birdc", "show", "route", "all", "where", fmt.Sprintf("bgp_path ~ [= %d =]", asn)); err == nil && out != "" {
		p := parser.GetParser("bird")
		result, parseErr := p.ParseBGPRoute(out)
		if parseErr == nil {
			asnResult := &domain.ASPathResult{
				ASN: asn,
				Raw: result.Raw,
			}
			for _, route := range result.Routes {
				asnResult.Prefixes = append(asnResult.Prefixes, domain.ASPathEntry{
					Prefix: route.Prefix,
					ASPath: route.ASPath,
				})
			}
			return asnResult, nil
		}
	}

	if out, err := d.execCmd(ctx, "vtysh", "-c", fmt.Sprintf("show ip bgp regexp ^%d_", asn)); err == nil && out != "" {
		p := parser.GetParser("cisco")
		result, parseErr := p.ParseBGPRoute(out)
		if parseErr == nil {
			asnResult := &domain.ASPathResult{
				ASN: asn,
				Raw: result.Raw,
			}
			for _, route := range result.Routes {
				asnResult.Prefixes = append(asnResult.Prefixes, domain.ASPathEntry{
					Prefix: route.Prefix,
					ASPath: route.ASPath,
				})
			}
			return asnResult, nil
		}
	}

	raw := fmt.Sprintf("AS%d — no local BGP data available (tried birdc, vtysh)", asn)
	return &domain.ASPathResult{
		ASN: asn,
		Raw: raw,
	}, nil
}

// execOrStream uses execCmdStream if an onLine callback is in the context, otherwise falls back to execCmd.
func (d *Driver) execOrStream(ctx context.Context, name string, args ...string) (string, error) {
	if onLine := getOnLine(ctx); onLine != nil {
		return d.execCmdStream(ctx, onLine, name, args...)
	}
	return d.execCmd(ctx, name, args...)
}

func (d *Driver) execCmd(ctx context.Context, name string, args ...string) (string, error) {
	var outBytes []byte
	var lastErr error
	err := d.circuitBreaker.Call(func() error {
		cmd := exec.CommandContext(ctx, name, args...)
		outBytes, lastErr = cmd.CombinedOutput()
		return lastErr
	})
	if err == circuitbreaker.ErrCircuitOpen {
		return "", err
	}
	return string(outBytes), lastErr
}

func (d *Driver) execCmdStream(ctx context.Context, onLine func(string), name string, args ...string) (string, error) {
	var buf bytes.Buffer
	var lastErr error

	err := d.circuitBreaker.Call(func() error {
		cmd := exec.CommandContext(ctx, name, args...)
		stdout, err := cmd.StdoutPipe()
		if err != nil {
			return err
		}
		stderr, err := cmd.StderrPipe()
		if err != nil {
			stdout.Close()
			return err
		}

		if err := cmd.Start(); err != nil {
			return err
		}

		var mu sync.Mutex
		var wg sync.WaitGroup

		writeLine := func(line string) {
			mu.Lock()
			buf.WriteString(line)
			buf.WriteByte('\n')
			mu.Unlock()
			onLine(line)
		}

		wg.Add(2)
		go func() {
			defer wg.Done()
			scanner := bufio.NewScanner(stdout)
			for scanner.Scan() {
				writeLine(scanner.Text())
			}
		}()
		go func() {
			defer wg.Done()
			scanner := bufio.NewScanner(stderr)
			for scanner.Scan() {
				writeLine(scanner.Text())
			}
		}()

		wg.Wait()
		lastErr = cmd.Wait()
		return lastErr
	})

	if err == circuitbreaker.ErrCircuitOpen {
		return "", err
	}
	return buf.String(), lastErr
}

func parseInt(s string) int {
	n, _ := strconv.Atoi(strings.TrimSpace(s))
	return n
}
