package agent

import (
	"context"
	"fmt"
	"net"
	"os/exec"
	"strings"

	"github.com/HopStat/HopStat/internal/domain"
	"github.com/HopStat/HopStat/internal/parser"
	"github.com/HopStat/HopStat/internal/target"
)

func isBlockedIP(ip net.IP) bool {
	if ip.IsLoopback() || ip.IsUnspecified() || ip.IsPrivate() {
		return true
	}
	if ip.IsLinkLocalUnicast() || ip.IsMulticast() {
		return true
	}
	// CGNAT / Shared Address Space (RFC 6598)
	if ip4 := ip.To4(); ip4 != nil {
		if ip4[0] == 100 && ip4[1] >= 64 && ip4[1] <= 127 {
			return true
		}
		if ip4[0] == 0 {
			return true
		}
	}
	return false
}

func isValidTarget(target string) bool {
	if strings.ContainsAny(target, ";|&`$(){}[]!><\n\r") || len(target) > 253 {
		return false
	}
	// If it's already an IP, check the blocklist directly
	if ip := net.ParseIP(target); ip != nil {
		return !isBlockedIP(ip)
	}
	// For hostnames: resolve all addresses and verify none are blocked
	addrs, err := net.LookupHost(target)
	if err != nil || len(addrs) == 0 {
		return false
	}
	for _, addr := range addrs {
		if ip := net.ParseIP(addr); ip != nil && isBlockedIP(ip) {
			return false
		}
	}
	return true
}

func runPing(ctx context.Context, target string, count int) (*domain.PingResult, error) {
	if !isValidTarget(target) {
		return nil, fmt.Errorf("invalid target: %s", target)
	}
	cmd := exec.CommandContext(ctx, "ping", "-c", fmt.Sprint(count), "-W", "2", target)
	out, err := cmd.CombinedOutput()
	raw := string(out)
	p := &parser.GenericParser{}
	if err != nil {
		if raw == "" {
			return &domain.PingResult{Raw: raw, PacketsSent: count, PacketLoss: 100}, err
		}
		result, _ := p.ParsePing(raw)
		if result.PacketsSent == 0 {
			result.PacketsSent = count
		}
		if result.PacketsRecv == 0 && result.PacketLoss == 0 {
			result.PacketLoss = 100
		}
		return result, nil
	}
	return p.ParsePing(raw)
}

func runTraceroute(ctx context.Context, target string, maxHops int) (*domain.TracerouteResult, error) {
	if !isValidTarget(target) {
		return nil, fmt.Errorf("invalid target: %s", target)
	}
	cmd := exec.CommandContext(ctx, "traceroute", "-m", fmt.Sprint(maxHops), "-w", "1", target)
	out, err := cmd.CombinedOutput()
	raw := string(out)
	if err != nil && raw == "" {
		return &domain.TracerouteResult{Raw: raw}, err
	}
	p := &parser.GenericParser{}
	return p.ParseTraceroute(raw)
}

func runBGPRoute(ctx context.Context, prefix string) (*domain.BGPResult, error) {
	resolved, err := target.NormalizeBGPLookup(ctx, prefix)
	if err != nil {
		return nil, err
	}
	prefix = resolved

	// Try birdc first
	cmd := exec.CommandContext(ctx, "birdc", "show", "route", "for", prefix)
	out, err := cmd.CombinedOutput()
	raw := string(out)
	if err == nil && strings.TrimSpace(raw) != "" {
		p := parser.GetParser("bird")
		result, parseErr := p.ParseBGPRoute(raw)
		if parseErr == nil && len(result.Routes) > 0 {
			return result, nil
		}
	}

	// Try vtysh
	cmd = exec.CommandContext(ctx, "vtysh", "-c", fmt.Sprintf("show ip bgp %s", prefix))
	out, err = cmd.CombinedOutput()
	raw = string(out)
	if err == nil && strings.TrimSpace(raw) != "" {
		p := parser.GetParser("cisco")
		return p.ParseBGPRoute(raw)
	}

	return &domain.BGPResult{
		Raw:    fmt.Sprintf("no BGP data for %s", prefix),
		Routes: nil,
	}, nil
}
