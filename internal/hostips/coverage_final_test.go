package hostips

import (
	"errors"
	"net"
	"testing"
)

func TestListErrorPaths(t *testing.T) {
	t.Run("interfaces error", func(t *testing.T) {
		old := listHostInterfaces
		listHostInterfaces = func() ([]net.Interface, error) { return nil, errors.New("ifaces failed") }
		t.Cleanup(func() { listHostInterfaces = old })
		if _, _, err := List(); err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("skip bad addrs and duplicates", func(t *testing.T) {
		old := listHostInterfaces
		ip := net.ParseIP("192.0.2.10")
		listHostInterfaces = func() ([]net.Interface, error) {
			return []net.Interface{
				{
					Name:  "eth0",
					Flags: net.FlagUp,
				},
				{
					Name:  "eth1",
					Flags: net.FlagUp,
				},
			}, nil
		}
		t.Cleanup(func() { listHostInterfaces = old })

		oldAddrs := getInterfaceAddrs
		getInterfaceAddrs = func(iface net.Interface) ([]net.Addr, error) {
			switch iface.Name {
			case "eth0":
				return []net.Addr{
					fakeAddr{},
					&net.IPNet{IP: ip, Mask: net.CIDRMask(32, 32)},
					&net.IPNet{IP: ip, Mask: net.CIDRMask(32, 32)},
					&net.IPNet{IP: net.ParseIP("127.0.0.1"), Mask: net.CIDRMask(8, 32)},
				}, nil
			case "eth1":
				return nil, errors.New("addr err")
			default:
				return nil, nil
			}
		}
		t.Cleanup(func() { getInterfaceAddrs = oldAddrs })

		v4, v6, err := List()
		if err != nil {
			t.Fatal(err)
		}
		if len(v4) != 1 || v4[0].IP != "192.0.2.10" {
			t.Fatalf("v4 = %+v", v4)
		}
		if len(v6) != 0 {
			t.Fatalf("v6 = %+v", v6)
		}
	})
}


func TestListSkipsUnusableIPv6(t *testing.T) {
	oldIfaces := listHostInterfaces
	oldAddrs := getInterfaceAddrs
	listHostInterfaces = func() ([]net.Interface, error) {
		return []net.Interface{{Name: "eth0", Flags: net.FlagUp}}, nil
	}
	getInterfaceAddrs = func(net.Interface) ([]net.Addr, error) {
		return []net.Addr{
			&net.IPNet{IP: net.ParseIP("ff02::1"), Mask: net.CIDRMask(128, 128)},
		}, nil
	}
	t.Cleanup(func() {
		listHostInterfaces = oldIfaces
		getInterfaceAddrs = oldAddrs
	})
	_, v6, err := List()
	if err != nil {
		t.Fatal(err)
	}
	if len(v6) != 0 {
		t.Fatalf("v6 = %+v", v6)
	}
}

// The same address is often bound to more than one interface, and it must be listed once.
// Injected rather than read from the host: whether the machine running the tests happens
// to have a duplicate decides nothing here.
func TestListReportsAnAddressBoundTwiceOnlyOnce(t *testing.T) {
	oldIfaces := listHostInterfaces
	oldAddrs := getInterfaceAddrs
	listHostInterfaces = func() ([]net.Interface, error) {
		return []net.Interface{
			{Name: "eth0", Flags: net.FlagUp},
			{Name: "eth1", Flags: net.FlagUp},
		}, nil
	}
	getInterfaceAddrs = func(net.Interface) ([]net.Addr, error) {
		return []net.Addr{
			&net.IPNet{IP: net.ParseIP("192.0.2.1"), Mask: net.CIDRMask(32, 32)},
			&net.IPNet{IP: net.ParseIP("2001:db8::1"), Mask: net.CIDRMask(128, 128)},
		}, nil
	}
	t.Cleanup(func() {
		listHostInterfaces = oldIfaces
		getInterfaceAddrs = oldAddrs
	})

	v4, v6, err := List()
	if err != nil {
		t.Fatal(err)
	}
	if len(v4) != 1 || v4[0].IP != "192.0.2.1" {
		t.Fatalf("v4 = %+v, want the address once", v4)
	}
	if len(v6) != 1 || v6[0].IP != "2001:db8::1" {
		t.Fatalf("v6 = %+v, want the address once", v6)
	}
	// The interface it is reported against is the first one carrying it.
	if v4[0].Interface != "eth0" || v6[0].Interface != "eth0" {
		t.Fatalf("interfaces = %q / %q, want eth0", v4[0].Interface, v6[0].Interface)
	}
}

func TestListSkipsNilNormalizedIP(t *testing.T) {
	oldIfaces := listHostInterfaces
	oldAddrs := getInterfaceAddrs
	oldNorm := normalizeHostIP
	listHostInterfaces = func() ([]net.Interface, error) {
		return []net.Interface{{Name: "eth0", Flags: net.FlagUp}}, nil
	}
	getInterfaceAddrs = func(net.Interface) ([]net.Addr, error) {
		return []net.Addr{&net.IPNet{IP: net.ParseIP("192.0.2.1"), Mask: net.CIDRMask(32, 32)}}, nil
	}
	normalizeHostIP = func(net.IP) net.IP { return nil }
	t.Cleanup(func() {
		listHostInterfaces = oldIfaces
		getInterfaceAddrs = oldAddrs
		normalizeHostIP = oldNorm
	})
	v4, _, err := List()
	if err != nil || len(v4) != 0 {
		t.Fatalf("v4=%+v err=%v", v4, err)
	}
}

func TestListSkipsDuplicateIPv4(t *testing.T) {
	oldIfaces := listHostInterfaces
	oldAddrs := getInterfaceAddrs
	ip := net.ParseIP("198.51.100.5")
	listHostInterfaces = func() ([]net.Interface, error) {
		return []net.Interface{{Name: "eth0", Flags: net.FlagUp}}, nil
	}
	getInterfaceAddrs = func(net.Interface) ([]net.Addr, error) {
		return []net.Addr{
			&net.IPNet{IP: ip, Mask: net.CIDRMask(32, 32)},
			&net.IPNet{IP: ip, Mask: net.CIDRMask(32, 32)},
		}, nil
	}
	t.Cleanup(func() {
		listHostInterfaces = oldIfaces
		getInterfaceAddrs = oldAddrs
	})
	v4, _, err := List()
	if err != nil {
		t.Fatal(err)
	}
	if len(v4) != 1 {
		t.Fatalf("v4 = %+v", v4)
	}
}

func TestListUsesGetInterfaceAddrsHook(t *testing.T) {
	oldIfaces := listHostInterfaces
	oldAddrs := getInterfaceAddrs
	listHostInterfaces = func() ([]net.Interface, error) {
		return []net.Interface{{Name: "eth0", Flags: net.FlagUp}}, nil
	}
	getInterfaceAddrs = func(net.Interface) ([]net.Addr, error) {
		return []net.Addr{
			&net.IPNet{IP: net.ParseIP("2001:db8::1"), Mask: net.CIDRMask(128, 128)},
			&net.IPNet{IP: net.ParseIP("fe80::1"), Mask: net.CIDRMask(64, 128)},
		}, nil
	}
	t.Cleanup(func() {
		listHostInterfaces = oldIfaces
		getInterfaceAddrs = oldAddrs
	})
	_, v6, err := List()
	if err != nil {
		t.Fatal(err)
	}
	if len(v6) != 2 {
		t.Fatalf("v6 = %+v", v6)
	}
}
