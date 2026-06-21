package standalone

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/HopStat/HopStat/internal/circuitbreaker"
	"github.com/HopStat/HopStat/internal/config"
	"github.com/HopStat/HopStat/internal/domain"
	"github.com/HopStat/HopStat/internal/parser"
	targetpkg "github.com/HopStat/HopStat/internal/target"
)

var commandContext = exec.CommandContext

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
func resolveTarget(ctx context.Context, target string) (string, error) {
	return targetpkg.ResolveHost(ctx, target)
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
		if errors.Is(err, domain.ErrDNSNotFound) {
			return nil, domain.ErrDNSNotFound
		}
		return nil, domain.ErrInvalidTarget
	}

	out, err := d.execOrStream(ctx, "ping", "-c", fmt.Sprint(count), "-W", "2", resolved)
	if err != nil {
		parsed, _ := d.parser.ParsePing(out)
		if parsed.PacketsSent == 0 {
			parsed.PacketsSent = count
		}
		if parsed.PacketsRecv == 0 && parsed.PacketLoss == 0 {
			parsed.PacketLoss = 100
		}
		return parsed, nil
	}
	return d.parser.ParsePing(out)
}

func (d *Driver) Traceroute(ctx context.Context, target string, maxHops int) (*domain.TracerouteResult, error) {
	if !d.node.CanExecute(domain.CmdTraceroute) {
		return nil, domain.ErrCommandDisabled
	}
	resolved, err := resolveTarget(ctx, target)
	if err != nil {
		if errors.Is(err, domain.ErrDNSNotFound) {
			return nil, domain.ErrDNSNotFound
		}
		return nil, domain.ErrInvalidTarget
	}

	out, err := d.execOrStream(ctx, "traceroute", "-m", fmt.Sprint(maxHops), "-w", "1", resolved)
	if err != nil {
		return &domain.TracerouteResult{Raw: out}, nil
	}
	return d.parser.ParseTraceroute(out)
}

func (d *Driver) BGPRoute(ctx context.Context, prefix string) (*domain.BGPResult, error) {
	if !d.node.CanExecute(domain.CmdBGPRoute) {
		return nil, domain.ErrCommandDisabled
	}
	// Resolve if it's a hostname (only for non-CIDR targets)
	if !strings.Contains(prefix, "/") && net.ParseIP(prefix) == nil {
		resolved, err := resolveTarget(ctx, prefix)
		if err != nil {
			if errors.Is(err, domain.ErrDNSNotFound) {
				return nil, domain.ErrDNSNotFound
			}
			return nil, domain.ErrInvalidTarget
		}
		prefix = resolved
	} else if strings.Contains(prefix, "/") {
		if _, _, err := net.ParseCIDR(prefix); err != nil {
			return nil, domain.ErrInvalidTarget
		}
	}

	if _, err := exec.LookPath("birdc"); err == nil {
		if out, err := d.execCmd(ctx, "birdc", "show", "route", "for", prefix); err == nil && out != "" {
			p := parser.GetParser("bird")
			result, parseErr := p.ParseBGPRoute(out)
			if parseErr == nil && len(result.Routes) > 0 {
				return result, nil
			}
		}
	}

	if _, err := exec.LookPath("vtysh"); err == nil {
		if out, err := d.execCmd(ctx, "vtysh", "-c", fmt.Sprintf("show ip bgp %s", prefix)); err == nil && out != "" {
			p := parser.GetParser("cisco")
			result, parseErr := p.ParseBGPRoute(out)
			if parseErr == nil && len(result.Routes) > 0 {
				return result, nil
			}
		}
	}

	return &domain.BGPResult{
		Raw: fmt.Sprintf("no BGP data for %s", prefix),
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
		cmd := commandContext(ctx, name, args...)
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
		cmd := commandContext(ctx, name, args...)
		stdout, err := cmd.StdoutPipe()
		if err != nil {
			return err
		}
		stderr, err := cmd.StderrPipe()
		if err != nil {
			stdout.Close()
			return err
		}
		defer stdout.Close()
		defer stderr.Close()

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
