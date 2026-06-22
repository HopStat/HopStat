package target

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"

	"github.com/HopStat/HopStat/internal/domain"
)

func IsBlockedIP(ip net.IP) bool {
	if ip.IsLoopback() || ip.IsUnspecified() || ip.IsPrivate() {
		return true
	}
	if ipIsLinkLocalUnicast(ip) || ip.IsMulticast() {
		return true
	}
	if ip4 := ip.To4(); ip4 != nil {
		if ip4[0] == 169 && ip4[1] == 254 {
			return true
		}
		if ip4[0] == 0 {
			return true
		}
		if ip4[0] == 100 && ip4[1] >= 64 && ip4[1] <= 127 {
			return true
		}
	}
	return false
}

// ResolveHost resolves a hostname to an IP. Bare IPs are returned unchanged after blocklist checks.
func ResolveHost(ctx context.Context, target string) (string, error) {
	target = strings.TrimSpace(target)
	if target == "" {
		return "", fmt.Errorf("empty target")
	}
	if ip := net.ParseIP(target); ip != nil {
		if IsBlockedIP(ip) {
			return "", fmt.Errorf("target %s is not allowed", target)
		}
		return ip.String(), nil
	}
	ips, err := lookupHostAddrs(ctx, target)
	if err != nil || len(ips) == 0 {
		return "", fmt.Errorf("%w: %s", domain.ErrDNSNotFound, target)
	}
	for _, addr := range ips {
		if IsBlockedIP(addr.IP) {
			return "", fmt.Errorf("resolved target contains blocked address %s", addr.IP)
		}
	}
	return ips[0].IP.String(), nil
}

func isUnsafeTarget(target string) bool {
	return strings.ContainsAny(target, ";|&`$(){}[]!><\n\r") || len(target) > 253
}

// Host-length BGP prefixes (/32, /128) are normalized to a bare IP so lookup
// uses longest-prefix match instead of an exact host route that may be missing.
func normalizeBGPLookupTarget(target string) (string, error) {
	if !strings.Contains(target, "/") {
		return target, nil
	}
	ip, ipNet, err := net.ParseCIDR(target)
	if err != nil {
		return "", fmt.Errorf("invalid prefix: %s", target)
	}
	ones, bits := ipNet.Mask.Size()
	if ones == bits {
		if IsBlockedIP(ip) {
			return "", fmt.Errorf("target %s is not allowed", ip)
		}
		return ip.String(), nil
	}
	return target, nil
}

// ValidateQueryTarget resolves hostnames to IPs before query execution.
// CIDR prefixes (BGP) are validated without DNS lookup.
func ValidateQueryTarget(ctx context.Context, command, target string) (string, error) {
	target = strings.TrimSpace(target)
	if target == "" || isUnsafeTarget(target) {
		return "", domain.ErrInvalidTarget
	}

	if command == string(domain.CmdBGPRoute) {
		normalized, err := NormalizeBGPLookup(ctx, target)
		if err != nil {
			if errors.Is(err, domain.ErrDNSNotFound) {
				return "", domain.ErrDNSNotFound
			}
			return "", domain.ErrInvalidTarget
		}
		return normalized, nil
	}

	if ip := net.ParseIP(target); ip != nil {
		if IsBlockedIP(ip) {
			return "", domain.ErrInvalidTarget
		}
		return ip.String(), nil
	}

	resolved, err := ResolveHost(ctx, target)
	if err != nil {
		if errors.Is(err, domain.ErrDNSNotFound) {
			return "", domain.ErrDNSNotFound
		}
		return "", domain.ErrInvalidTarget
	}
	return resolved, nil
}

// NormalizeBGPLookup accepts a CIDR, IP, or hostname and returns a value suitable for BGP lookup.
func NormalizeBGPLookup(ctx context.Context, target string) (string, error) {
	target = strings.TrimSpace(target)
	if target == "" {
		return "", fmt.Errorf("empty target")
	}
	if strings.Contains(target, "/") {
		normalized, err := normalizeBGPLookupTarget(target)
		if err != nil {
			return "", err
		}
		return normalized, nil
	}
	if ip := net.ParseIP(target); ip != nil {
		if IsBlockedIP(ip) {
			return "", fmt.Errorf("target %s is not allowed", target)
		}
		return ip.String(), nil
	}
	return ResolveHost(ctx, target)
}
