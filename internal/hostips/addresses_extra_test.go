package hostips

import (
	"net"
	"testing"
)

type fakeAddr struct{}

func (fakeAddr) Network() string { return "fake" }
func (fakeAddr) String() string  { return "fake" }

func TestExtractIPAndHelpers(t *testing.T) {
	if extractIP(fakeAddr{}) != nil {
		t.Fatal("expected nil for unknown addr type")
	}

	ipNet := &net.IPNet{IP: net.ParseIP("192.168.1.1"), Mask: net.CIDRMask(24, 32)}
	if got := extractIP(ipNet); got == nil || got.String() != "192.168.1.1" {
		t.Fatalf("IPNet extract = %v", got)
	}
	ipAddr := &net.IPAddr{IP: net.ParseIP("2001:db8::1")}
	if got := extractIP(ipAddr); got == nil {
		t.Fatal("expected IPAddr extract")
	}

	if usableIPv4Peering(net.ParseIP("127.0.0.1")) {
		t.Fatal("loopback v4 should be unusable")
	}
	if usableIPv6Peering(net.ParseIP("ff02::1")) {
		t.Fatal("link-local multicast v6 should be unusable")
	}
	if !usableIPv6Peering(net.ParseIP("2001:db8::1")) {
		t.Fatal("global v6 should be usable")
	}
}

func TestSortAddresses(t *testing.T) {
	list := []Address{
		{IP: "10.0.0.2", Interface: "eth0"},
		{IP: "10.0.0.1", Interface: "eth0"},
		{IP: "10.0.0.3", Interface: "eth1"},
	}
	sortAddresses(list)
	if list[0].IP != "10.0.0.1" || list[2].Interface != "eth1" {
		t.Fatalf("sorted = %+v", list)
	}
}

func TestListReturnsEmptySlices(t *testing.T) {
	v4, v6, err := List()
	if err != nil {
		t.Fatal(err)
	}
	if v4 == nil || v6 == nil {
		t.Fatal("expected non-nil empty slices")
	}
}
