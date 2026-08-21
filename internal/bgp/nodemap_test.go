package bgp

import (
	"context"
	"fmt"
	"testing"
	"time"

	api "github.com/osrg/gobgp/v3/api"
	"github.com/osrg/gobgp/v3/pkg/apiutil"
	"github.com/osrg/gobgp/v3/pkg/packet/bgp"

	"github.com/HopStat/HopStat/internal/config"
	"github.com/HopStat/HopStat/internal/domain"
)

// mapTestManager starts a manager with two nodes, each owning one neighbor IP, so
// per-node RIB filtering has something to filter on.
func mapTestManager(t *testing.T, localAS uint32) (*SessionManager, context.Context) {
	t.Helper()
	mgr, ctx := startTestManager(t, config.BGPConfig{LocalAS: localAS, RouterID: "127.0.0.1"})
	mgr.mu.Lock()
	mgr.neighbors[1] = &neighborEntry{
		neighbor:   &domain.BGPNeighbor{ID: 1, NodeID: 1, NeighborIP: "10.0.0.1"},
		neighborIP: "10.0.0.1",
	}
	mgr.neighbors[2] = &neighborEntry{
		neighbor:   &domain.BGPNeighbor{ID: 2, NodeID: 2, NeighborIP: "10.0.0.2"},
		neighborIP: "10.0.0.2",
	}
	mgr.nodeNeighbors[1] = map[int64]struct{}{1: {}}
	mgr.nodeNeighbors[2] = map[int64]struct{}{2: {}}
	mgr.mu.Unlock()
	return mgr, ctx
}

// serveRIB installs a RIB stub returning one path per neighbor, so each node sees a
// different AS path for the same prefix.
func serveRIB(t *testing.T, prefix string, addr string, masklen uint8, byNeighbor map[string][]uint32) {
	t.Helper()
	old := lookupListPathHook
	lookupListPathHook = func(_ context.Context, _ *api.ListPathRequest, fn func(*api.Destination)) error {
		var paths []*api.Path
		for _, nip := range []string{"10.0.0.1", "10.0.0.2"} {
			asPath, ok := byNeighbor[nip]
			if !ok {
				continue
			}
			attrs := []bgp.PathAttributeInterface{
				bgp.NewPathAttributeOrigin(bgp.BGP_ORIGIN_ATTR_TYPE_IGP),
				bgp.NewPathAttributeNextHop(nip),
			}
			if len(asPath) > 0 {
				attrs = append(attrs, bgp.NewPathAttributeAsPath(
					[]bgp.AsPathParamInterface{bgp.NewAs4PathParam(2, asPath)},
				))
			}
			path, err := apiutil.NewPath(bgp.NewIPAddrPrefix(masklen, addr), false, attrs, time.Now())
			if err != nil {
				t.Fatalf("NewPath: %v", err)
			}
			path.NeighborIp = nip
			path.Best = true
			paths = append(paths, path)
		}
		fn(&api.Destination{Prefix: prefix, Paths: paths})
		return nil
	}
	t.Cleanup(func() { lookupListPathHook = old })
}

func twoMapNodes() []MapNode {
	return []MapNode{
		{ID: 1, Name: "BURSA", Type: domain.NodeTypeStandalone},
		{ID: 2, Name: "SOFIA", Type: domain.NodeTypeStandalone},
	}
}

func TestBuildNodeASPathsSkipsWhenThereIsNothingToRead(t *testing.T) {
	var nilMgr *SessionManager
	if got := nilMgr.BuildNodeASPaths(context.Background(), twoMapNodes(), "8.8.8.0/24", 0, nil); got != nil {
		t.Fatalf("nil manager returned %+v", got)
	}

	mgr, ctx := mapTestManager(t, 9121)
	if got := mgr.BuildNodeASPaths(ctx, nil, "8.8.8.0/24", 9121, nil); got != nil {
		t.Fatalf("no nodes returned %+v", got)
	}

	// Whether one node is enough for a map is the engine's call, not this function's:
	// it answers for whatever it is given.
	one := []MapNode{{ID: 1, Name: "BURSA", Type: domain.NodeTypeStandalone}}
	if got := mgr.BuildNodeASPaths(ctx, one, "8.8.8.0/24", 9121, nil); len(got) != 1 {
		t.Fatalf("single node returned %+v", got)
	}
}

func TestBuildNodeASPathsInvalidTarget(t *testing.T) {
	mgr, ctx := mapTestManager(t, 9121)
	if got := mgr.BuildNodeASPaths(ctx, twoMapNodes(), "   ", 9121, nil); got != nil {
		t.Fatalf("invalid target returned %+v", got)
	}
}

func TestBuildNodeASPathsReportsPerNodePaths(t *testing.T) {
	mgr, ctx := mapTestManager(t, 9121)
	serveRIB(t, "8.8.8.0/24", "8.8.8.0", 24, map[string][]uint32{
		"10.0.0.1": {3356, 15169},
		"10.0.0.2": {6939, 15169},
	})

	names := func(ip string) string { return "peer-" + ip }
	paths := mgr.BuildNodeASPaths(ctx, twoMapNodes(), "8.8.8.0/24", 9121, names)

	if len(paths) != 2 {
		t.Fatalf("paths = %+v", paths)
	}
	// Order follows the input slice, not goroutine completion.
	if paths[0].NodeName != "BURSA" || paths[1].NodeName != "SOFIA" {
		t.Fatalf("node order = %q, %q", paths[0].NodeName, paths[1].NodeName)
	}
	for i, want := range [][]uint32{{9121, 3356, 15169}, {9121, 6939, 15169}} {
		if fmt.Sprint(paths[i].ASPath) != fmt.Sprint(want) {
			t.Fatalf("path[%d] = %v, want %v", i, paths[i].ASPath, want)
		}
		if paths[i].NoRoute {
			t.Fatalf("path[%d] marked NoRoute", i)
		}
		if paths[i].Prefix != "8.8.8.0/24" {
			t.Fatalf("path[%d] prefix = %q", i, paths[i].Prefix)
		}
		if paths[i].NodeID != int64(i+1) {
			t.Fatalf("path[%d] node id = %d", i, paths[i].NodeID)
		}
	}
}

func TestBuildNodeASPathsLeavesAgentPathsAlone(t *testing.T) {
	mgr, ctx := mapTestManager(t, 9121)
	serveRIB(t, "8.8.8.0/24", "8.8.8.0", 24, map[string][]uint32{
		"10.0.0.1": {3356, 15169},
		"10.0.0.2": {6939, 15169},
	})

	nodes := twoMapNodes()
	nodes[1].Type = domain.NodeTypeLGNode
	paths := mgr.BuildNodeASPaths(ctx, nodes, "8.8.8.0/24", 9121, nil)

	if fmt.Sprint(paths[0].ASPath) != fmt.Sprint([]uint32{9121, 3356, 15169}) {
		t.Fatalf("standalone path = %v", paths[0].ASPath)
	}
	if fmt.Sprint(paths[1].ASPath) != fmt.Sprint([]uint32{6939, 15169}) {
		t.Fatalf("agent path = %v, local AS must not be prepended", paths[1].ASPath)
	}
}

func TestBuildNodeASPathsMarksLookupFailureAsNoRoute(t *testing.T) {
	mgr, ctx := mapTestManager(t, 9121)
	old := lookupListPathHook
	lookupListPathHook = func(context.Context, *api.ListPathRequest, func(*api.Destination)) error {
		return fmt.Errorf("rib unavailable")
	}
	defer func() { lookupListPathHook = old }()

	paths := mgr.BuildNodeASPaths(ctx, twoMapNodes(), "8.8.8.0/24", 9121, nil)
	if len(paths) != 2 {
		t.Fatalf("paths = %+v", paths)
	}
	for i, p := range paths {
		if !p.NoRoute || len(p.ASPath) > 0 {
			t.Fatalf("path[%d] = %+v, want NoRoute", i, p)
		}
		if p.NodeID == 0 || p.NodeName == "" {
			t.Fatalf("path[%d] lost its node identity: %+v", i, p)
		}
	}
}

func TestBuildNodeASPathsMarksEmptyPathAsNoRoute(t *testing.T) {
	mgr, ctx := mapTestManager(t, 9121)
	// No AS_PATH attribute, and agent nodes so the local AS is not prepended either.
	serveRIB(t, "8.8.8.0/24", "8.8.8.0", 24, map[string][]uint32{
		"10.0.0.1": {},
		"10.0.0.2": {6939, 15169},
	})

	nodes := twoMapNodes()
	nodes[0].Type = domain.NodeTypeLGNode
	nodes[1].Type = domain.NodeTypeLGNode
	paths := mgr.BuildNodeASPaths(ctx, nodes, "8.8.8.0/24", 9121, nil)

	if !paths[0].NoRoute {
		t.Fatalf("empty AS path should be NoRoute: %+v", paths[0])
	}
	if paths[1].NoRoute {
		t.Fatalf("second node should still have a path: %+v", paths[1])
	}
}

func TestBuildNodeASPathsKeepsDefaultRouteFlag(t *testing.T) {
	mgr, ctx := mapTestManager(t, 9121)
	serveRIB(t, "0.0.0.0/0", "0.0.0.0", 0, map[string][]uint32{
		"10.0.0.1": {6453},
		"10.0.0.2": {174},
	})

	paths := mgr.BuildNodeASPaths(ctx, twoMapNodes(), "213.146.165.165", 9121, nil)
	if !paths[0].ViaDefaultRoute {
		t.Fatalf("expected via-default-route entry: %+v", paths[0])
	}
	if paths[0].Prefix != "213.146.165.165/32" {
		t.Fatalf("prefix = %q", paths[0].Prefix)
	}
}

// serveMultiPathRIB gives one neighbor several paths for the same prefix, the way a node
// with more than one upstream sees a backup route.
func serveMultiPathRIB(t *testing.T, byNeighbor map[string][][]uint32) {
	t.Helper()
	old := lookupListPathHook
	lookupListPathHook = func(_ context.Context, _ *api.ListPathRequest, fn func(*api.Destination)) error {
		var paths []*api.Path
		for _, nip := range []string{"10.0.0.1", "10.0.0.2"} {
			for i, asPath := range byNeighbor[nip] {
				attrs := []bgp.PathAttributeInterface{
					bgp.NewPathAttributeOrigin(bgp.BGP_ORIGIN_ATTR_TYPE_IGP),
					bgp.NewPathAttributeNextHop(nip),
					bgp.NewPathAttributeAsPath(
						[]bgp.AsPathParamInterface{bgp.NewAs4PathParam(2, asPath)},
					),
				}
				path, err := apiutil.NewPath(bgp.NewIPAddrPrefix(24, "8.8.8.0"), false, attrs, time.Now())
				if err != nil {
					t.Fatalf("NewPath: %v", err)
				}
				path.NeighborIp = nip
				path.Best = i == 0 // GoBGP marks only the selected path
				paths = append(paths, path)
			}
		}
		fn(&api.Destination{Prefix: "8.8.8.0/24", Paths: paths})
		return nil
	}
	t.Cleanup(func() { lookupListPathHook = old })
}

func TestBuildNodeASPathsIncludesBackupPaths(t *testing.T) {
	mgr, ctx := mapTestManager(t, 9121)
	serveMultiPathRIB(t, map[string][][]uint32{
		"10.0.0.1": {{3356, 15169}, {6939, 174, 15169}},
		"10.0.0.2": {{8866, 15169}},
	})

	paths := mgr.BuildNodeASPaths(ctx, twoMapNodes(), "8.8.8.0/24", 9121, nil)

	var bursa []domain.NodeASPath
	for _, p := range paths {
		if p.NodeID == 1 {
			bursa = append(bursa, p)
		}
	}
	if len(bursa) != 2 {
		t.Fatalf("expected a selected route plus one backup, got %+v", bursa)
	}
	if !bursa[0].Best {
		t.Fatalf("first path should be the selected one: %+v", bursa[0])
	}
	if bursa[1].Best {
		t.Fatalf("backup must not be flagged best: %+v", bursa[1])
	}
	if fmt.Sprint(bursa[1].ASPath) != fmt.Sprint([]uint32{9121, 6939, 174, 15169}) {
		t.Fatalf("backup path = %v", bursa[1].ASPath)
	}
}

func TestBuildNodeASPathsDropsDuplicateAndExcessPaths(t *testing.T) {
	mgr, ctx := mapTestManager(t, 9121)
	serveMultiPathRIB(t, map[string][][]uint32{
		"10.0.0.1": {
			{3356, 15169},
			{3356, 15169}, // same path from another session — no extra information
			{6939, 15169},
			{1299, 15169},
			{174, 15169},
			{2914, 15169}, // beyond maxNodeMapPaths
		},
		"10.0.0.2": {{8866, 15169}},
	})

	paths := mgr.BuildNodeASPaths(ctx, twoMapNodes(), "8.8.8.0/24", 9121, nil)

	count := 0
	seen := map[string]bool{}
	for _, p := range paths {
		if p.NodeID != 1 {
			continue
		}
		count++
		key := fmt.Sprint(p.ASPath)
		if seen[key] {
			t.Fatalf("duplicate path emitted: %v", p.ASPath)
		}
		seen[key] = true
	}
	if count != maxNodeMapPaths {
		t.Fatalf("paths for node 1 = %d, want %d", count, maxNodeMapPaths)
	}
}

func TestBuildNodeASPathsSkipsPathlessRoutes(t *testing.T) {
	mgr, ctx := mapTestManager(t, 9121)
	// An agent node so the local AS is not prepended onto the empty path.
	serveRIB(t, "8.8.8.0/24", "8.8.8.0", 24, map[string][]uint32{
		"10.0.0.1": {},
		"10.0.0.2": {8866, 15169},
	})
	nodes := twoMapNodes()
	nodes[0].Type = domain.NodeTypeLGNode

	paths := mgr.BuildNodeASPaths(ctx, nodes, "8.8.8.0/24", 9121, nil)
	if !paths[0].NoRoute {
		t.Fatalf("a route with no AS path is not a usable path: %+v", paths[0])
	}
}
