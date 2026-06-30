package target

import (
	"context"
	"errors"
	"net"
	"testing"

	"github.com/HopStat/HopStat/internal/domain"
)

type fakeNetAddr struct{}

func (fakeNetAddr) Network() string { return "fake" }
func (fakeNetAddr) String() string  { return "fake" }

func TestAllowedAgentHostBlockedResolvedIP(t *testing.T) {
	old := lookupHostAddrs
	lookupHostAddrs = func(ctx context.Context, host string) ([]net.IPAddr, error) {
		return []net.IPAddr{{IP: net.ParseIP("169.254.169.254")}}, nil
	}
	t.Cleanup(func() { lookupHostAddrs = old })
	if AllowedAgentHost(context.Background(), "metadata.example") {
		t.Fatal("expected blocked resolved IP to reject host")
	}
}

func TestIsLocalInterfaceIPNonIPNet(t *testing.T) {
	old := interfaceAddrs
	interfaceAddrs = func() ([]net.Addr, error) {
		return []net.Addr{fakeNetAddr{}}, nil
	}
	t.Cleanup(func() { interfaceAddrs = old })
	if isLocalInterfaceIP(net.ParseIP("192.0.2.1")) {
		t.Fatal("expected false for non-IPNet interface address")
	}
}

func TestIsBlockedIPMetadata(t *testing.T) {
	if !IsBlockedIP(net.ParseIP("169.254.169.254")) {
		t.Fatal("expected metadata IP blocked")
	}
}

func TestValidateQueryTargetSuccessfulResolve(t *testing.T) {
	old := lookupHostAddrs
	lookupHostAddrs = func(ctx context.Context, host string) ([]net.IPAddr, error) {
		return []net.IPAddr{{IP: net.ParseIP("8.8.8.8")}}, nil
	}
	t.Cleanup(func() { lookupHostAddrs = old })
	got, err := ValidateQueryTarget(context.Background(), "ping", "example.test")
	if err != nil || got != "8.8.8.8" {
		t.Fatalf("got=%q err=%v", got, err)
	}
}

func TestIsBlockedIPMetadata169(t *testing.T) {
	if !IsBlockedIP(net.ParseIP("169.254.169.254")) {
		t.Fatal("expected link-local metadata blocked")
	}
}

func TestIsLocalInterfaceIPAddrsError(t *testing.T) {
	old := interfaceAddrs
	interfaceAddrs = func() ([]net.Addr, error) { return nil, errors.New("ifaces failed") }
	t.Cleanup(func() { interfaceAddrs = old })
	if isLocalInterfaceIP(net.ParseIP("192.0.2.1")) {
		t.Fatal("expected false on interface addrs error")
	}
}

func TestIsBlockedIPLinkLocal169Block(t *testing.T) {
	old := ipIsLinkLocalUnicast
	ipIsLinkLocalUnicast = func(net.IP) bool { return false }
	t.Cleanup(func() { ipIsLinkLocalUnicast = old })
	if !IsBlockedIP(net.ParseIP("169.254.1.1")) {
		t.Fatal("expected 169.254 block")
	}
}

func TestNormalizeBGPLookupBlockedHostCIDR(t *testing.T) {
	_, err := NormalizeBGPLookup(context.Background(), "127.0.0.1/32")
	if err == nil {
		t.Fatal("expected blocked IP error for loopback /32")
	}
}

func TestValidateQueryTargetBGPDNSNotFound(t *testing.T) {
	old := lookupHostAddrs
	lookupHostAddrs = func(ctx context.Context, host string) ([]net.IPAddr, error) {
		return nil, errors.New("no such host")
	}
	t.Cleanup(func() { lookupHostAddrs = old })
	_, err := ValidateQueryTarget(context.Background(), "bgp_route", "invalid.bgp.host.example")
	if !errors.Is(err, domain.ErrDNSNotFound) {
		t.Fatalf("expected ErrDNSNotFound, got %v", err)
	}
}

func TestIsLocalInterfaceIPMatch(t *testing.T) {
	ip := net.ParseIP("203.0.113.10")
	old := interfaceAddrs
	interfaceAddrs = func() ([]net.Addr, error) {
		return []net.Addr{&net.IPNet{IP: ip, Mask: net.CIDRMask(32, 32)}}, nil
	}
	t.Cleanup(func() { interfaceAddrs = old })
	if !isLocalInterfaceIP(ip) {
		t.Fatal("expected matching interface IP")
	}
}
