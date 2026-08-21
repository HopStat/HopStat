package bgp

import (
	"context"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/HopStat/HopStat/internal/domain"
	"github.com/HopStat/HopStat/internal/target"
)

// MapNode identifies one node to include in the multi-node network map.
type MapNode struct {
	ID   int64
	Name string
	Type domain.NodeType
}

const (
	// nodeMapFanout bounds concurrent RIB lookups the way enrichHops bounds GeoIP lookups.
	nodeMapFanout = 8
	// nodeMapTimeout keeps the map from eating the whole query budget if the RIB stalls.
	nodeMapTimeout = 8 * time.Second
	// maxNodeMapPaths bounds how many paths one node contributes — its selected route plus
	// a few backups. A node carrying a dozen alternates would bury the map in edges.
	maxNodeMapPaths = 4
)

// BuildNodeASPaths reports how each given node reaches the target, for the network map.
// Each node contributes its selected route and any backup paths it also holds.
//
// The target is resolved once and the normalized literal handed to every lookup, so a
// hostname costs one DNS round trip and every node is answering about the same prefix.
// Per-node failures become NoRoute entries rather than failing the whole map: a partial
// comparison is more useful than none.
func (m *SessionManager) BuildNodeASPaths(
	ctx context.Context,
	nodes []MapNode,
	prefix string,
	localAS uint32,
	nodeNameForNeighbor func(string) string,
) []domain.NodeASPath {
	if m == nil || len(nodes) == 0 {
		return nil
	}

	normalized, err := target.NormalizeBGPLookup(ctx, prefix)
	if err != nil {
		return nil
	}

	ctx, cancel := context.WithTimeout(ctx, nodeMapTimeout)
	defer cancel()

	perNode := make([][]domain.NodeASPath, len(nodes))
	sem := make(chan struct{}, nodeMapFanout)
	var wg sync.WaitGroup

	for i, node := range nodes {
		wg.Add(1)
		go func(idx int, n MapNode) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			perNode[idx] = m.nodeASPaths(ctx, n, normalized, localAS, nodeNameForNeighbor)
		}(i, node)
	}
	wg.Wait()

	// Flatten in node order — the goroutines finish in whatever order they like.
	var paths []domain.NodeASPath
	for _, entries := range perNode {
		paths = append(paths, entries...)
	}
	return paths
}

// nodeASPaths resolves one node's paths from the shared RIB — the selected route first,
// then the backups it would fall back to. Mirrors the engine's own BGP lookup so the
// queried node's selected route on the map matches the result shown above it.
func (m *SessionManager) nodeASPaths(
	ctx context.Context,
	node MapNode,
	normalized string,
	localAS uint32,
	nodeNameForNeighbor func(string) string,
) []domain.NodeASPath {
	br, err := m.BuildRouteResult(ctx, node.ID, normalized, nodeNameForNeighbor)
	if err != nil || br == nil {
		return NoRouteNodeASPaths(node.ID, node.Name)
	}
	if node.Type != domain.NodeTypeLGNode {
		ApplyOriginASPathToRoutes(br.Routes, localAS)
	}
	return RoutesToNodeASPaths(node.ID, node.Name, br.Routes)
}

// NoRouteNodeASPaths keeps a node on the map when it has nothing for the target —
// "this node cannot reach it" is part of the comparison.
func NoRouteNodeASPaths(nodeID int64, nodeName string) []domain.NodeASPath {
	return []domain.NodeASPath{{NodeID: nodeID, NodeName: nodeName, NoRoute: true}}
}

// RoutesToNodeASPaths turns one node's routes into map entries: the selected route first,
// then its backups, de-duplicated by AS path and capped at maxNodeMapPaths.
func RoutesToNodeASPaths(nodeID int64, nodeName string, routes []domain.BGPRoute) []domain.NodeASPath {
	SortRoutesBestFirst(routes)
	selected := BestRoute(routes)

	var paths []domain.NodeASPath
	seen := map[string]bool{}

	for i := range routes {
		route := &routes[i]
		if len(route.ASPath) == 0 {
			continue
		}
		// Two neighbors often advertise the same path; drawing it twice adds no information.
		key := asPathKey(route.ASPath)
		if seen[key] {
			continue
		}
		seen[key] = true

		paths = append(paths, domain.NodeASPath{
			NodeID:          nodeID,
			NodeName:        nodeName,
			Prefix:          route.Prefix,
			ASPath:          append([]uint32(nil), route.ASPath...),
			Best:            route == selected,
			ViaDefaultRoute: route.ViaDefaultRoute,
		})
		if len(paths) == maxNodeMapPaths {
			break
		}
	}

	if len(paths) == 0 {
		return NoRouteNodeASPaths(nodeID, nodeName)
	}
	return paths
}

func asPathKey(path []uint32) string {
	parts := make([]string, len(path))
	for i, asn := range path {
		parts[i] = strconv.FormatUint(uint64(asn), 10)
	}
	return strings.Join(parts, " ")
}
