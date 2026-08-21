package bgp

import (
	"context"
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
)

// BuildNodeASPaths reports how each given node reaches the target, for the network map.
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
	// With fewer than two nodes the map only repeats the AS path map above it.
	if m == nil || len(nodes) < 2 {
		return nil
	}

	normalized, err := target.NormalizeBGPLookup(ctx, prefix)
	if err != nil {
		return nil
	}

	ctx, cancel := context.WithTimeout(ctx, nodeMapTimeout)
	defer cancel()

	paths := make([]domain.NodeASPath, len(nodes))
	sem := make(chan struct{}, nodeMapFanout)
	var wg sync.WaitGroup

	for i, node := range nodes {
		wg.Add(1)
		go func(idx int, n MapNode) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			paths[idx] = m.nodeASPath(ctx, n, normalized, localAS, nodeNameForNeighbor)
		}(i, node)
	}
	wg.Wait()

	return paths
}

// nodeASPath resolves a single node's best path, mirroring the engine's own BGP lookup so
// the queried node's row on the map matches the result shown above it.
func (m *SessionManager) nodeASPath(
	ctx context.Context,
	node MapNode,
	normalized string,
	localAS uint32,
	nodeNameForNeighbor func(string) string,
) domain.NodeASPath {
	entry := domain.NodeASPath{NodeID: node.ID, NodeName: node.Name, NoRoute: true}

	br, err := m.BuildRouteResult(ctx, node.ID, normalized, nodeNameForNeighbor)
	if err != nil || br == nil {
		return entry
	}
	if node.Type != domain.NodeTypeLGNode {
		ApplyOriginASPathToRoutes(br.Routes, localAS)
	}

	route := BestRoute(br.Routes)
	if route == nil || len(route.ASPath) == 0 {
		return entry
	}

	entry.NoRoute = false
	entry.Prefix = route.Prefix
	entry.ASPath = append([]uint32(nil), route.ASPath...)
	entry.ViaDefaultRoute = route.ViaDefaultRoute
	return entry
}
