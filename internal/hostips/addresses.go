package hostips

import (
	"net"
	"sort"
	"strings"
)

var (
	listHostInterfaces = net.Interfaces
	getInterfaceAddrs  = func(iface net.Interface) ([]net.Addr, error) { return iface.Addrs() }
	normalizeHostIP    = normalizeIP
)

// Address is a local IP bound to a network interface.
type Address struct {
	IP        string `json:"ip"`
	Interface string `json:"interface"`
	LinkLocal bool   `json:"link_local,omitempty"`
}

// List returns local addresses suitable for BGP peering selection.
func List() (ipv4, ipv6 []Address, err error) {
	ifaces, err := listHostInterfaces()
	if err != nil {
		return nil, nil, err
	}

	seen4 := make(map[string]struct{})
	seen6 := make(map[string]struct{})

	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		ifName := strings.TrimSpace(iface.Name)
		addrs, err := getInterfaceAddrs(iface)
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			ip := extractIP(addr)
			if ip == nil {
				continue
			}
			ip = normalizeHostIP(ip)
			if ip == nil {
				continue
			}

			if v4 := ip.To4(); v4 != nil {
				if !usableIPv4Peering(v4) {
					continue
				}
				text := v4.String()
				if _, ok := seen4[text]; ok {
					continue
				}
				seen4[text] = struct{}{}
				ipv4 = append(ipv4, Address{IP: text, Interface: ifName})
				continue
			}

			if !usableIPv6Peering(ip) {
				continue
			}
			text := ip.String()
			if _, ok := seen6[text]; ok {
				continue
			}
			seen6[text] = struct{}{}
			ipv6 = append(ipv6, Address{
				IP:        text,
				Interface: ifName,
				LinkLocal: ip.IsLinkLocalUnicast(),
			})
		}
	}

	sortAddresses(ipv4)
	sortAddresses(ipv6)
	if ipv4 == nil {
		ipv4 = []Address{}
	}
	if ipv6 == nil {
		ipv6 = []Address{}
	}
	return ipv4, ipv6, nil
}

// AppendUnique adds value to list when missing and keeps the slice sorted.
func AppendUnique(list []Address, ip string) []Address {
	for _, item := range list {
		if item.IP == ip {
			return list
		}
	}
	list = append(list, Address{IP: ip})
	sortAddresses(list)
	return list
}

func sortAddresses(list []Address) {
	sort.Slice(list, func(i, j int) bool {
		if list[i].Interface != list[j].Interface {
			return list[i].Interface < list[j].Interface
		}
		return list[i].IP < list[j].IP
	})
}

func normalizeIP(ip net.IP) net.IP {
	if v4 := ip.To4(); v4 != nil {
		return v4
	}
	return ip.To16()
}

func extractIP(addr net.Addr) net.IP {
	switch v := addr.(type) {
	case *net.IPNet:
		return v.IP
	case *net.IPAddr:
		return v.IP
	default:
		return nil
	}
}

func usableIPv4Peering(ip net.IP) bool {
	if ip == nil || ip.IsUnspecified() || ip.IsLoopback() || ip.IsLinkLocalUnicast() {
		return false
	}
	return true
}

func usableIPv6Peering(ip net.IP) bool {
	if ip == nil || ip.IsUnspecified() || ip.IsLoopback() || ip.IsLinkLocalMulticast() {
		return false
	}
	return true
}
