package bgp

import (
	"fmt"
	"net"
	"strings"

	"github.com/HopStat/HopStat/internal/domain"
)

type RouteFallback struct {
	LocalAS        uint32
	DefaultRouteAS uint32
}

func NewRouteFallback(localAS, defaultRouteAS uint32) *RouteFallback {
	if localAS == 0 || defaultRouteAS == 0 {
		return nil
	}
	return &RouteFallback{
		LocalAS:        localAS,
		DefaultRouteAS: defaultRouteAS,
	}
}

func DefaultRoutePrefix(normalized string) string {
	if ip := targetIP(normalized); ip != nil && ip.To4() == nil {
		return "::/0"
	}
	return "0.0.0.0/0"
}

func targetIP(normalized string) net.IP {
	if strings.Contains(normalized, "/") {
		ip, _, err := net.ParseCIDR(normalized)
		if err == nil {
			return ip
		}
	}
	return net.ParseIP(normalized)
}

func displayPrefix(normalized string) string {
	if strings.Contains(normalized, "/") {
		return normalized
	}
	ip := net.ParseIP(normalized)
	if ip == nil {
		return normalized
	}
	if ip.To4() == nil {
		return normalized + "/128"
	}
	return normalized + "/32"
}

func (fb RouteFallback) ASPath() []uint32 {
	return []uint32{fb.LocalAS, fb.DefaultRouteAS}
}

func (fb RouteFallback) originPrefix(viaDefaultRoute bool) []uint32 {
	if fb.LocalAS == 0 {
		return nil
	}
	if viaDefaultRoute {
		return fb.ASPath()
	}
	return []uint32{fb.LocalAS}
}

// PrependOriginASPath prefixes the local AS on learned routes, or local+upstream on default-route fallbacks.
func PrependOriginASPath(path []uint32, fb *RouteFallback, viaDefaultRoute bool) []uint32 {
	if fb == nil {
		return append([]uint32(nil), path...)
	}
	prefix := fb.originPrefix(viaDefaultRoute)
	if len(prefix) == 0 {
		return append([]uint32(nil), path...)
	}
	if len(path) == 0 {
		return append([]uint32(nil), prefix...)
	}
	i := 0
	for i < len(prefix) && i < len(path) && prefix[i] == path[i] {
		i++
	}
	out := make([]uint32, 0, len(prefix)+len(path)-i)
	out = append(out, prefix...)
	out = append(out, path[i:]...)
	return out
}

// ApplyOriginASPathToRoutes ensures each route's AS path starts with the peering local AS.
func ApplyOriginASPathToRoutes(routes []domain.BGPRoute, localAS uint32) {
	if localAS == 0 || len(routes) == 0 {
		return
	}
	fb := RouteFallback{LocalAS: localAS}
	for i := range routes {
		if len(routes[i].ASPath) > 0 && routes[i].ASPath[0] == localAS {
			continue
		}
		routes[i].ASPath = PrependOriginASPath(routes[i].ASPath, &fb, false)
	}
}

func SynthesizeDefaultRouteResult(queriedPrefix string, defaults []*domain.BGPRouteEntry, fb RouteFallback) *domain.BGPResult {
	route := domain.BGPRoute{
		Prefix:          displayPrefix(queriedPrefix),
		ASPath:          fb.ASPath(),
		ViaDefaultRoute: true,
		Best:            true,
	}

	defaultLabel := DefaultRoutePrefix(queriedPrefix)
	if len(defaults) > 0 {
		entry := defaults[0]
		for _, e := range defaults {
			if e.Best {
				entry = e
				break
			}
		}
		if entry.Prefix != "" {
			defaultLabel = entry.Prefix
		}
		route.NextHop = entry.NextHop
		route.Origin = entry.Origin
		route.LocalPref = parseUint(entry.LocalPref)
		route.MED = parseUint(entry.MED)
		route.Age = entry.Age
		route.Best = entry.Best
		route.Communities = append([]string(nil), entry.Communities...)
	}

	asPathStr := fmt.Sprintf("%d %d", fb.LocalAS, fb.DefaultRouteAS)
	raw := fmt.Sprintf("%-20s via %s  AS_PATH: %s  Origin: %s  [default route %s]",
		route.Prefix, route.NextHop, asPathStr, route.Origin, defaultLabel)

	return &domain.BGPResult{
		Routes: []domain.BGPRoute{route},
		Raw:    raw,
	}
}
