package bgp

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/HopStat/HopStat/internal/domain"
	"github.com/HopStat/HopStat/internal/target"
)

func (m *SessionManager) LocalAS() uint32 {
	return m.globalAS
}

type neighborDefaultConfig struct {
	neighborIP     string
	ipv6NeighborIP string
	defaultRouteAS uint32
}

func (m *SessionManager) neighborsWithDefaultRouteAS(nodeID int64) []neighborDefaultConfig {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var out []neighborDefaultConfig
	for neighborID := range m.nodeNeighbors[nodeID] {
		entry, ok := m.neighbors[neighborID]
		if !ok || entry.neighbor == nil || entry.neighbor.DefaultRouteAS == 0 {
			continue
		}
		out = append(out, neighborDefaultConfig{
			neighborIP:     entry.neighborIP,
			ipv6NeighborIP: entry.ipv6NeighborIP,
			defaultRouteAS: entry.neighbor.DefaultRouteAS,
		})
	}
	return out
}

func defaultEntryForNeighbor(defaults []*domain.BGPRouteEntry, neighborIP, ipv6NeighborIP string) *domain.BGPRouteEntry {
	var best *domain.BGPRouteEntry
	for _, d := range defaults {
		if d.NeighborIP != neighborIP && d.NeighborIP != ipv6NeighborIP {
			continue
		}
		if best == nil || d.Best {
			best = d
		}
	}
	return best
}

func (m *SessionManager) synthesizeDefaultRoutesForNode(ctx context.Context, nodeID int64, normalized string, nodeNameForNeighbor func(string) string) (*domain.BGPResult, error) {
	candidates := m.neighborsWithDefaultRouteAS(nodeID)
	if len(candidates) == 0 {
		raw := fmt.Sprintf("no matching BGP routes for %s", normalized)
		return &domain.BGPResult{Raw: raw}, nil
	}

	defaults, err := m.LookupRoute(ctx, nodeID, DefaultRoutePrefix(normalized))
	if err != nil {
		return nil, err
	}

	var routes []domain.BGPRoute
	var rawLines []string
	for _, nb := range candidates {
		fb := RouteFallback{LocalAS: m.globalAS, DefaultRouteAS: nb.defaultRouteAS}
		entry := defaultEntryForNeighbor(defaults, nb.neighborIP, nb.ipv6NeighborIP)
		var synthEntries []*domain.BGPRouteEntry
		if entry != nil {
			synthEntries = []*domain.BGPRouteEntry{entry}
		}
		partial := SynthesizeDefaultRouteResult(normalized, synthEntries, fb)
		for _, route := range partial.Routes {
			if nodeNameForNeighbor != nil {
				if entry != nil && entry.NeighborIP != "" {
					route.NodeName = nodeNameForNeighbor(entry.NeighborIP)
				} else if nb.neighborIP != "" {
					route.NodeName = nodeNameForNeighbor(nb.neighborIP)
				}
			}
			routes = append(routes, route)
		}
		if partial.Raw != "" {
			rawLines = append(rawLines, strings.TrimSpace(partial.Raw))
		}
	}

	SortRoutesBestFirst(routes)
	return &domain.BGPResult{
		Routes: routes,
		Raw:    strings.Join(rawLines, "\n"),
	}, nil
}

func (m *SessionManager) BuildRouteResult(ctx context.Context, nodeID int64, prefix string, nodeNameForNeighbor func(string) string) (*domain.BGPResult, error) {
	normalized, err := target.NormalizeBGPLookup(ctx, prefix)
	if err != nil {
		if errors.Is(err, domain.ErrDNSNotFound) {
			return nil, domain.ErrDNSNotFound
		}
		return nil, domain.ErrInvalidTarget
	}
	entries, err := m.LookupRoute(ctx, nodeID, normalized)
	if err != nil {
		return nil, err
	}
	if len(entries) == 0 {
		return m.synthesizeDefaultRoutesForNode(ctx, nodeID, normalized, nodeNameForNeighbor)
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Best != entries[j].Best {
			return entries[i].Best
		}
		return false
	})
	br := &domain.BGPResult{Raw: formatRouteEntries(entries)}
	for _, entry := range entries {
		prefix := normalizeRoutePrefix(entry.Prefix)
		viaDefault := false
		if isBareIPQuery(normalized) && isDefaultRoutePrefix(entry.Prefix) {
			prefix = displayPrefix(normalized)
			viaDefault = true
		}
		route := domain.BGPRoute{
			Prefix:          prefix,
			NextHop:         entry.NextHop,
			ASPath:          parseASPath(entry.ASPath),
			Origin:          entry.Origin,
			LocalPref:       parseUint(entry.LocalPref),
			MED:             parseUint(entry.MED),
			Age:             entry.Age,
			Best:            entry.Best,
			Communities:     append([]string(nil), entry.Communities...),
			ViaDefaultRoute: viaDefault,
		}
		if nodeNameForNeighbor != nil {
			route.NodeName = nodeNameForNeighbor(entry.NeighborIP)
		}
		br.Routes = append(br.Routes, route)
	}
	return br, nil
}

func SortRoutesBestFirst(routes []domain.BGPRoute) {
	sort.Slice(routes, func(i, j int) bool {
		if routes[i].Best != routes[j].Best {
			return routes[i].Best
		}
		return false
	})
}

// ensureBestAmongEntries marks the best path per prefix when node filtering removed
// GoBGP's global best flag (only the first path in the RIB gets Best=true).
func ensureBestAmongEntries(entries []*domain.BGPRouteEntry) {
	if len(entries) == 0 {
		return
	}
	byPrefix := make(map[string][]*domain.BGPRouteEntry)
	for _, e := range entries {
		byPrefix[e.Prefix] = append(byPrefix[e.Prefix], e)
	}
	for _, group := range byPrefix {
		if len(group) == 1 {
			group[0].Best = true
			continue
		}
		// Multiple paths for the same prefix: GoBGP's global Best flag answers "best
		// across every neighbor", which is not the question here — once the set is
		// filtered to one node, the active path is whichever the decision process picks
		// among that node's own paths.
		best := 0
		for i := 1; i < len(group); i++ {
			if morePreferred(entryPreference(group[i]), entryPreference(group[best])) {
				best = i
			}
		}
		for i, e := range group {
			e.Best = i == best
		}
	}
}

// EnsureBestAmongRoutes marks one route per prefix when none carry the best flag
// (e.g. CLI parser output without active path markers).
func EnsureBestAmongRoutes(routes []domain.BGPRoute) {
	if len(routes) == 0 {
		return
	}
	byPrefix := make(map[string][]int)
	for i := range routes {
		byPrefix[routes[i].Prefix] = append(byPrefix[routes[i].Prefix], i)
	}
	for _, idxs := range byPrefix {
		if len(idxs) == 1 {
			routes[idxs[0]].Best = true
			continue
		}
		marked := -1
		for _, i := range idxs {
			if routes[i].Best {
				if marked >= 0 {
					marked = -1
					break
				}
				marked = i
			}
		}
		for _, i := range idxs {
			routes[i].Best = false
		}
		// A vendor's own active-path marker beats anything we can reconstruct.
		if marked >= 0 {
			routes[marked].Best = true
			continue
		}
		best := idxs[0]
		for _, i := range idxs[1:] {
			if morePreferred(routePreference(&routes[i]), routePreference(&routes[best])) {
				best = i
			}
		}
		routes[best].Best = true
	}
}

// BestRoute returns the selected route for a prefix, preferring an explicit best flag.
func BestRoute(routes []domain.BGPRoute) *domain.BGPRoute {
	if len(routes) == 0 {
		return nil
	}
	for i := range routes {
		if routes[i].Best {
			return &routes[i]
		}
	}
	return &routes[0]
}

// HasRouteASPath reports whether the BGP result includes a usable AS path from a route.
func HasRouteASPath(br *domain.BGPResult) bool {
	if br == nil {
		return false
	}
	route := BestRoute(br.Routes)
	return route != nil && len(route.ASPath) > 0
}

func formatRouteEntries(entries []*domain.BGPRouteEntry) string {
	var buf strings.Builder
	for _, e := range entries {
		fmt.Fprintf(&buf, "%-20s via %s  AS_PATH: %s  Origin: %s", e.Prefix, e.NextHop, e.ASPath, e.Origin)
		if len(e.Communities) > 0 {
			fmt.Fprintf(&buf, "  Communities: %s", strings.Join(e.Communities, " "))
		}
		if e.Age != "" {
			fmt.Fprintf(&buf, "  Age: %s", e.Age)
		}
		if e.Best {
			buf.WriteString("  [best]")
		}
		buf.WriteByte('\n')
	}
	return buf.String()
}

func parseASPath(raw string) []uint32 {
	if raw == "" {
		return nil
	}
	var asns []uint32
	for _, part := range strings.FieldsFunc(raw, func(r rune) bool {
		return r == ' ' || r == '[' || r == ']' || r == ','
	}) {
		if v, err := strconv.ParseUint(part, 10, 32); err == nil {
			asns = append(asns, uint32(v))
		}
	}
	return asns
}

func parseUint(s string) uint32 {
	v, _ := strconv.ParseUint(s, 10, 32)
	return uint32(v)
}
