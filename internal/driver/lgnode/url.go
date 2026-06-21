package lgnode

import (
	"net"
	"net/url"
	"strings"
)

var interfaceAddrs = net.InterfaceAddrs

// resolveLocalAgentURL rewrites agent URLs that point at this machine's own
// interface IPs to 127.0.0.1, avoiding NAT hairpin timeouts on health checks.
func resolveLocalAgentURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return raw
	}

	host := strings.Trim(strings.ToLower(u.Hostname()), "[]")
	if host == "localhost" || host == "127.0.0.1" || host == "::1" {
		return raw
	}
	if !isLocalHost(host) {
		return raw
	}

	port := u.Port()
	if port == "" {
		switch u.Scheme {
		case "https":
			port = "443"
		default:
			port = "80"
		}
	}
	u.Host = net.JoinHostPort("127.0.0.1", port)
	return u.String()
}

func isLocalHost(host string) bool {
	hostIP := net.ParseIP(host)
	if hostIP == nil {
		return false
	}
	if hostIP.IsLoopback() {
		return true
	}
	ifaces, err := interfaceAddrs()
	if err != nil {
		return false
	}
	for _, a := range ifaces {
		ipnet, ok := a.(*net.IPNet)
		if !ok {
			continue
		}
		if ipnet.IP.Equal(hostIP) {
			return true
		}
	}
	return false
}
