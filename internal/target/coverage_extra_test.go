package target

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/HopStat/HopStat/internal/domain"
)

func TestAllowedAgentHostDNSFailure(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if AllowedAgentHost(ctx, "invalid.invalid.invalid") {
		t.Fatal("expected DNS failure to reject host")
	}
}

func TestAllowedAgentHostLocalInterfaceIP(t *testing.T) {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		t.Skip("interface addrs unavailable")
	}
	for _, a := range addrs {
		ipnet, ok := a.(*net.IPNet)
		if !ok || ipnet.IP.IsLoopback() {
			continue
		}
		if isLocalInterfaceIP(ipnet.IP) && !AllowedAgentHost(context.Background(), ipnet.IP.String()) {
			t.Fatalf("expected local interface IP %s to be allowed", ipnet.IP)
		}
		return
	}
	t.Skip("no non-loopback interface address")
}

func TestResolveHostBlockedResolved(t *testing.T) {
	// localhost resolves to loopback which IsBlockedIP rejects in ResolveHost path
	_, err := ResolveHost(context.Background(), "localhost")
	if err == nil {
		t.Fatal("expected blocked resolved address")
	}
}

func TestValidateQueryTargetInvalidCIDR(t *testing.T) {
	_, err := ValidateQueryTarget(context.Background(), "bgp_route", "not-a-cidr/24")
	if !errors.Is(err, domain.ErrInvalidTarget) {
		t.Fatalf("err = %v", err)
	}
}

func TestValidateQueryTargetUnsafe(t *testing.T) {
	_, err := ValidateQueryTarget(context.Background(), "ping", "8.8.8.8|cat")
	if !errors.Is(err, domain.ErrInvalidTarget) {
		t.Fatalf("err = %v", err)
	}
}

func TestIsBlockedIPLinkLocalUnicast(t *testing.T) {
	if !IsBlockedIP(net.ParseIP("fe80::1")) {
		t.Fatal("expected link-local ipv6 blocked")
	}
}

func TestValidateQueryTargetResolveBlocked(t *testing.T) {
	_, err := ValidateQueryTarget(context.Background(), "ping", "localhost")
	if !errors.Is(err, domain.ErrInvalidTarget) {
		t.Fatalf("err = %v", err)
	}
}

func TestAllowedAgentHostEmptyAfterTrim(t *testing.T) {
	if AllowedAgentHost(context.Background(), "   ") {
		t.Fatal("expected false for whitespace host")
	}
}

func TestIsBlockedIPZeroNetwork(t *testing.T) {
	if !IsBlockedIP(net.ParseIP("0.1.2.3")) {
		t.Fatal("expected 0.0.0.0/8 blocked")
	}
}

func TestResolveHostBlockedResolvedAddress(t *testing.T) {
	_, err := ResolveHost(context.Background(), "127.0.0.1")
	if err == nil {
		t.Fatal("expected blocked loopback")
	}
}

func TestValidateQueryTargetNonDNSResolveError(t *testing.T) {
	_, err := ValidateQueryTarget(context.Background(), "ping", "127.0.0.1")
	if !errors.Is(err, domain.ErrInvalidTarget) {
		t.Fatalf("err = %v", err)
	}
}
