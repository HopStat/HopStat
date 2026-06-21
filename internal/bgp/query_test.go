package bgp

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/HopStat/HopStat/internal/config"
	"github.com/HopStat/HopStat/internal/domain"
)

func TestEnsureBestAmongEntries(t *testing.T) {
	entries := []*domain.BGPRouteEntry{
		{Prefix: "1.1.1.0/24", NeighborIP: "10.0.0.1", Best: false},
		{Prefix: "1.1.1.0/24", NeighborIP: "10.0.0.2", Best: false},
	}
	ensureBestAmongEntries(entries)
	if !entries[0].Best {
		t.Fatal("expected first filtered entry to be marked best")
	}
	if entries[1].Best {
		t.Fatal("expected only one best entry")
	}

	keep := []*domain.BGPRouteEntry{
		{Prefix: "8.8.8.0/24", Best: false},
		{Prefix: "8.8.8.0/24", Best: true},
	}
	ensureBestAmongEntries(keep)
	if !keep[0].Best {
		t.Fatal("expected first entry to be marked best for multi-path prefix")
	}
	if keep[1].Best {
		t.Fatal("expected second entry not to remain best")
	}
}

func TestEnsureBestAmongRoutes(t *testing.T) {
	routes := []domain.BGPRoute{
		{Prefix: "203.0.113.0/24"},
		{Prefix: "203.0.113.0/24"},
	}
	EnsureBestAmongRoutes(routes)
	if !routes[0].Best {
		t.Fatal("expected first route marked best")
	}
	if routes[1].Best {
		t.Fatal("expected second route not best")
	}

	marked := []domain.BGPRoute{
		{Prefix: "8.8.8.0/24", Best: true, ASPath: []uint32{43260, 204457, 15169}},
		{Prefix: "8.8.8.0/24", Best: false, ASPath: []uint32{43260, 44901, 15169}},
	}
	EnsureBestAmongRoutes(marked)
	if !marked[0].Best {
		t.Fatal("expected explicit best route to remain selected")
	}
	if marked[1].Best {
		t.Fatal("expected alternate route not best")
	}
}

func TestBestRoute(t *testing.T) {
	routes := []domain.BGPRoute{
		{Prefix: "8.8.8.0/24", ASPath: []uint32{43260, 44901, 15169}},
		{Prefix: "8.8.8.0/24", Best: true, ASPath: []uint32{43260, 204457, 15169}},
	}
	got := BestRoute(routes)
	if got == nil || !got.Best || got.ASPath[1] != 204457 {
		t.Fatalf("BestRoute = %+v", got)
	}
	if BestRoute(nil) != nil {
		t.Fatal("expected nil for empty routes")
	}
}

func TestEnsureBestAmongEntriesSingle(t *testing.T) {
	single := []*domain.BGPRouteEntry{{Prefix: "1.0.0.0/24", Best: false}}
	ensureBestAmongEntries(single)
	if !single[0].Best {
		t.Fatal("expected single entry marked best")
	}
	ensureBestAmongEntries(nil)
}

func TestEnsureBestAmongRoutesAmbiguousBest(t *testing.T) {
	routes := []domain.BGPRoute{
		{Prefix: "1.1.1.0/24", Best: true},
		{Prefix: "1.1.1.0/24", Best: true},
	}
	EnsureBestAmongRoutes(routes)
	if !routes[0].Best || routes[1].Best {
		t.Fatalf("ambiguous best routes = %+v", routes)
	}
}

func TestFormatRouteEntriesAndParseASPath(t *testing.T) {
	raw := formatRouteEntries([]*domain.BGPRouteEntry{{
		Prefix:      "8.8.8.8/32",
		NextHop:     "10.0.0.1",
		ASPath:      "[65001, 15169]",
		Origin:      "IGP",
		Communities: []string{"65001:100"},
		Age:         "1h0m0s",
		Best:        true,
	}})
	for _, want := range []string{"8.8.8.8/32", "Communities:", "[best]"} {
		if !strings.Contains(raw, want) {
			t.Fatalf("raw = %q, missing %q", raw, want)
		}
	}
	got := parseASPath("[65001, 15169, 13335]")
	if len(got) != 3 || got[0] != 65001 || got[2] != 13335 {
		t.Fatalf("parseASPath = %v", got)
	}
	if parseASPath("") != nil {
		t.Fatal("empty as path should be nil")
	}
}

func TestSortRoutesBestFirst(t *testing.T) {
	routes := []domain.BGPRoute{
		{Prefix: "1.0.0.0/24", Best: false},
		{Prefix: "2.0.0.0/24", Best: true},
	}
	SortRoutesBestFirst(routes)
	if !routes[0].Best {
		t.Fatal("expected best route first")
	}
}

func TestNeighborsWithDefaultRouteAS(t *testing.T) {
	mgr := NewSessionManager(config.BGPConfig{LocalAS: 65001})
	mgr.mu.Lock()
	mgr.neighbors[1] = &neighborEntry{
		neighbor:   &domain.BGPNeighbor{ID: 1, NodeID: 9, DefaultRouteAS: 6453, NeighborIP: "10.0.0.1"},
		neighborIP: "10.0.0.1",
	}
	mgr.neighbors[2] = &neighborEntry{
		neighbor:   &domain.BGPNeighbor{ID: 2, NodeID: 9, NeighborIP: "10.0.0.2"},
		neighborIP: "10.0.0.2",
	}
	mgr.nodeNeighbors[9] = map[int64]struct{}{1: {}, 2: {}}
	mgr.mu.Unlock()

	got := mgr.neighborsWithDefaultRouteAS(9)
	if len(got) != 1 || got[0].defaultRouteAS != 6453 {
		t.Fatalf("defaults = %+v", got)
	}
}

func TestBuildRouteResult(t *testing.T) {
	mgr, ctx := startTestManager(t, config.BGPConfig{LocalAS: 65001, RouterID: "127.0.0.1"})
	mgr.mu.Lock()
	mgr.neighbors[1] = &neighborEntry{neighbor: testNeighbor(1, 3, "10.0.0.2"), neighborIP: "10.0.0.2"}
	mgr.nodeNeighbors[3] = map[int64]struct{}{1: {}}
	mgr.mu.Unlock()
	addGlobalRoute(t, ctx, mgr, "203.0.113.0", "10.0.0.2", nil)

	nameFn := func(ip string) string { return "node-" + ip }
	result, err := mgr.BuildRouteResult(ctx, 0, "203.0.113.0/24", nameFn)
	if err != nil {
		t.Fatalf("BuildRouteResult: %v", err)
	}
	if len(result.Routes) == 0 {
		t.Fatalf("result = %+v", result)
	}

	_, err = mgr.BuildRouteResult(ctx, 0, "not-a-valid-prefix/", nameFn)
	if !errors.Is(err, domain.ErrInvalidTarget) {
		t.Fatalf("invalid target err = %v", err)
	}
}

func TestSynthesizeDefaultRoutesForNode(t *testing.T) {
	mgr, ctx := startTestManager(t, config.BGPConfig{LocalAS: 9121, RouterID: "127.0.0.1"})
	mgr.globalAS = 9121
	mgr.mu.Lock()
	mgr.neighbors[1] = &neighborEntry{
		neighbor: &domain.BGPNeighbor{
			ID: 1, NodeID: 4, RemoteAS: 6453, DefaultRouteAS: 6453, NeighborIP: "10.0.0.1",
		},
		neighborIP: "10.0.0.1",
	}
	mgr.nodeNeighbors[4] = map[int64]struct{}{1: {}}
	mgr.mu.Unlock()

	nameFn := func(ip string) string { return "peer-" + ip }
	result, err := mgr.synthesizeDefaultRoutesForNode(ctx, 4, "8.8.8.8", nameFn)
	if err != nil {
		t.Fatalf("synthesizeDefaultRoutesForNode: %v", err)
	}
	if len(result.Routes) != 1 || !result.Routes[0].ViaDefaultRoute {
		t.Fatalf("synthesized = %+v", result)
	}

	emptyMgr := NewSessionManager(config.BGPConfig{LocalAS: 9121})
	emptyResult, err := emptyMgr.synthesizeDefaultRoutesForNode(ctx, 99, "8.8.8.8", nil)
	if err != nil {
		t.Fatalf("empty synthesize: %v", err)
	}
	if !strings.Contains(emptyResult.Raw, "no matching BGP routes") {
		t.Fatalf("raw = %q", emptyResult.Raw)
	}
}

func TestBuildRouteResultDNSNotFound(t *testing.T) {
	mgr, ctx := startTestManager(t, config.BGPConfig{LocalAS: 65001, RouterID: "127.0.0.1"})
	_, err := mgr.BuildRouteResult(ctx, 0, "missing.invalid.example", nil)
	if !errors.Is(err, domain.ErrDNSNotFound) {
		t.Fatalf("dns err = %v", err)
	}
}

func TestDefaultEntryForNeighborIPv6(t *testing.T) {
	defaults := []*domain.BGPRouteEntry{{NeighborIP: "2001:db8::2", Prefix: "::/0", Best: true}}
	if got := defaultEntryForNeighbor(defaults, "10.0.0.1", "2001:db8::2"); got == nil || got.NeighborIP != "2001:db8::2" {
		t.Fatalf("ipv6 default entry = %+v", got)
	}
}

func TestLocalASGetter(t *testing.T) {
	mgr := NewSessionManager(config.BGPConfig{LocalAS: 65001})
	mgr.globalAS = 65001
	if mgr.LocalAS() != 65001 {
		t.Fatalf("LocalAS = %d", mgr.LocalAS())
	}
}

func TestBuildRouteResultWithDefaultEntry(t *testing.T) {
	mgr, ctx := startTestManager(t, config.BGPConfig{LocalAS: 9121, RouterID: "127.0.0.1"})
	mgr.globalAS = 9121
	mgr.mu.Lock()
	mgr.neighbors[1] = &neighborEntry{
		neighbor: &domain.BGPNeighbor{ID: 1, NodeID: 6, DefaultRouteAS: 6453, NeighborIP: "10.0.0.1"},
		neighborIP: "10.0.0.1",
	}
	mgr.nodeNeighbors[6] = map[int64]struct{}{1: {}}
	mgr.mu.Unlock()
	addGlobalRoute(t, ctx, mgr, "0.0.0.0", "10.0.0.1", nil)

	result, err := mgr.BuildRouteResult(ctx, 6, "1.2.3.4", nil)
	if err != nil {
		t.Fatalf("BuildRouteResult default: %v", err)
	}
	if len(result.Routes) == 0 {
		t.Fatal("expected synthesized default route")
	}
	_ = time.Second
}
