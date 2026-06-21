package target

import (
	"context"
	"net"
	"net/url"
	"strings"
	"time"
)

var (
	interfaceAddrs       = net.InterfaceAddrs
	ipIsLinkLocalUnicast = func(ip net.IP) bool { return ip.IsLinkLocalUnicast() }
)

var lookupHostAddrs = func(ctx context.Context, host string) ([]net.IPAddr, error) {
	return net.DefaultResolver.LookupIPAddr(ctx, host)
}

// ValidAgentURL checks scheme/host and blocks SSRF to private, link-local, and metadata IPs.
// Loopback and this host's own interface addresses remain allowed for co-located agents.
func ValidAgentURL(raw string) bool {
	if raw == "" {
		return true
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return false
	}
	return AllowedAgentHost(context.Background(), u.Hostname())
}

// AllowedAgentHost validates a hostname or IP for lg_node agent_url targets.
func AllowedAgentHost(ctx context.Context, host string) bool {
	host = strings.Trim(strings.ToLower(strings.TrimSpace(host)), "[]")
	if host == "" {
		return false
	}
	if host == "localhost" {
		return true
	}

	if ip := net.ParseIP(host); ip != nil {
		return allowedAgentIP(ip)
	}

	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	addrs, err := lookupHostAddrs(ctx, host)
	if err != nil || len(addrs) == 0 {
		return false
	}
	for _, addr := range addrs {
		if !allowedAgentIP(addr.IP) {
			return false
		}
	}
	return true
}

func allowedAgentIP(ip net.IP) bool {
	if ip.IsLoopback() {
		return true
	}
	if isLocalInterfaceIP(ip) {
		return true
	}
	if IsBlockedIP(ip) {
		return false
	}
	return true
}

func isLocalInterfaceIP(ip net.IP) bool {
	ifaces, err := interfaceAddrs()
	if err != nil {
		return false
	}
	for _, a := range ifaces {
		ipnet, ok := a.(*net.IPNet)
		if !ok {
			continue
		}
		if ipnet.IP.Equal(ip) {
			return true
		}
	}
	return false
}
