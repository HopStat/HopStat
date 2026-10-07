package lgnode

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/HopStat/HopStat/internal/target"
)

// agentDial is the dialer seam, so tests can observe which address was chosen without
// opening a socket.
type agentDial interface {
	DialContext(ctx context.Context, network, addr string) (net.Conn, error)
}

var (
	// agentLookupIP and agentDialer are test seams: the first makes DNS answers
	// scriptable, the second keeps the test off the real network.
	agentLookupIP           = net.DefaultResolver.LookupIPAddr
	agentDialer   agentDial = &net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}
)

// agentDialContext resolves the destination and re-checks every address it is about to
// reach against the same SSRF policy that validated the URL when the node was saved.
//
// Validating at save time is not enough: the agent URL keeps its hostname, and the
// stock transport resolves that name again on every connection. A host that resolved to
// a public address when it was saved can resolve to cloud metadata or an internal
// address by the time a query runs, and nothing on this path would notice. So the name is
// resolved here, each answer is checked, and the connection is made to a validated
// address literal rather than back to the name — otherwise the dialer would re-resolve
// and the check would again describe a moment that has already passed.
func agentDialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	// SplitHostPort strips the brackets from an IPv6 literal, so host is bare here.
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}

	if ip := net.ParseIP(host); ip != nil {
		if !target.AllowedAgentIP(ip) {
			return nil, fmt.Errorf("agent address %s is not allowed", ip)
		}
		return agentDialer.DialContext(ctx, network, addr)
	}

	ips, err := agentLookupIP(ctx, host)
	if err != nil {
		return nil, err
	}
	if len(ips) == 0 {
		return nil, fmt.Errorf("agent host %s resolved to no addresses", host)
	}
	for _, ip := range ips {
		if !target.AllowedAgentIP(ip.IP) {
			return nil, fmt.Errorf("agent host %s resolves to %s, which is not allowed", host, ip.IP)
		}
	}

	// Happy eyeballs would need the kernel to pick, but it would pick from an
	// unvalidated set, so the first address that passed the check is dialled directly.
	return agentDialer.DialContext(ctx, network, net.JoinHostPort(ips[0].IP.String(), port))
}

// agentTransport is the transport every agent client uses. Each new connection is
// validated by agentDialContext, including the ones made for redirects and the
// streaming client.
func agentTransport() *http.Transport {
	return &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		DialContext:           agentDialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          10,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: time.Second,
	}
}
