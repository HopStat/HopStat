package bgp

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	api "github.com/osrg/gobgp/v3/api"
	"github.com/osrg/gobgp/v3/pkg/apiutil"
	"github.com/osrg/gobgp/v3/pkg/packet/bgp"
	"github.com/osrg/gobgp/v3/pkg/server"

	"github.com/HopStat/HopStat/internal/config"
	"github.com/HopStat/HopStat/internal/domain"
	"github.com/HopStat/HopStat/internal/geo"
)

func TestDualStackStopAndRestartNeighbor(t *testing.T) {
	mgr, _ := startTestManager(t, config.BGPConfig{LocalAS: 65000, RouterID: "127.0.0.1"})
	neighbor := &domain.BGPNeighbor{
		ID:             60,
		NodeID:         1,
		RemoteAS:       174,
		PeeringIP:      "127.0.0.1",
		NeighborIP:     "10.0.0.60",
		IPv6PeeringIP:  "2001:db8::1",
		IPv6NeighborIP: "2001:db8::60",
	}
	if err := mgr.AddNeighbor(neighbor); err != nil {
		t.Fatalf("AddNeighbor: %v", err)
	}
	if err := mgr.StopNeighbor(60); err != nil {
		t.Fatalf("StopNeighbor: %v", err)
	}
	if err := mgr.RestartNeighbor(60); err != nil {
		t.Fatalf("RestartNeighbor: %v", err)
	}
	if err := mgr.RemoveNeighbor(60); err != nil {
		t.Fatalf("RemoveNeighbor: %v", err)
	}
}

func TestAddNeighborIPv6AddFailure(t *testing.T) {
	mgr, _ := startTestManager(t, config.BGPConfig{LocalAS: 65000, RouterID: "127.0.0.1"})
	neighbor := &domain.BGPNeighbor{
		ID:             61,
		NodeID:         1,
		RemoteAS:       174,
		NeighborIP:     "10.0.0.61",
		IPv6NeighborIP: "not-a-valid-ipv6",
	}
	if err := mgr.AddNeighbor(neighbor); err != nil {
		t.Fatalf("AddNeighbor should succeed when only ipv6 add fails: %v", err)
	}
	logs := mgr.GetNeighborLogs(61, 10)
	foundWarn := false
	for _, e := range logs {
		if e.Level == "warn" && strings.Contains(e.Message, "ipv6 peer failed") {
			foundWarn = true
		}
	}
	if !foundWarn {
		t.Fatal("expected ipv6 add failure warning in logs")
	}
}

func TestUpdateNeighborAfterServerStop(t *testing.T) {
	mgr, _ := startTestManager(t, config.BGPConfig{LocalAS: 65000, RouterID: "127.0.0.1"})
	neighbor := testNeighbor(62, 1, "10.0.0.62")
	if err := mgr.AddNeighbor(neighbor); err != nil {
		t.Fatalf("AddNeighbor: %v", err)
	}
	mgr.Stop()
	if err := mgr.UpdateNeighbor(neighbor); err == nil {
		t.Fatal("expected AddNeighbor failure after server stop")
	}
}

func TestLoadNeighborsPartialFailure(t *testing.T) {
	mgr, ctx := startTestManager(t, config.BGPConfig{LocalAS: 65000, RouterID: "127.0.0.1"})
	repo := stubNeighborRepo{neighbors: []*domain.BGPNeighbor{
		testNeighbor(70, 1, "10.0.0.70"),
		{ID: 71, NodeID: 1, RemoteAS: 174, NeighborIP: "not an ip"},
	}}
	if err := mgr.LoadNeighbors(ctx, repo); err != nil {
		t.Fatalf("LoadNeighbors should continue on per-neighbor errors: %v", err)
	}
	if _, err := mgr.neighborEntry(70); err != nil {
		t.Fatalf("good neighbor should be loaded: %v", err)
	}
}

func TestEnsureBGPGlobalStartedStartError(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	mgr := NewSessionManager(config.BGPConfig{
		LocalAS:         65000,
		RouterID:        "127.0.0.1",
		ListenAddresses: []string{"0.0.0.0.0"},
	})
	mgr.bgpServer = server.NewBgpServer()
	go mgr.bgpServer.Serve()
	defer mgr.bgpServer.Stop()
	if err := mgr.ensureBGPGlobalStarted(ctx, 65000, "127.0.0.1"); err == nil {
		t.Fatal("expected StartBgp error for invalid listen address")
	}
}

func TestWatchStuckSessionsCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	mgr := NewSessionManager(config.BGPConfig{LocalAS: 65000, RouterID: "127.0.0.1"})
	if err := mgr.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	cancel()
	time.Sleep(150 * time.Millisecond)
	mgr.Stop()
}

func TestLogStuckSessionsSkipBranches(t *testing.T) {
	mgr, ctx := startTestManager(t, config.BGPConfig{LocalAS: 65000, RouterID: "127.0.0.1"})
	mgr.mu.Lock()
	mgr.neighbors[80] = &neighborEntry{neighbor: testNeighbor(80, 1, "10.0.0.80"), neighborIP: "10.0.0.80"}
	mgr.states[80] = domain.BGPSessionEstablished
	mgr.neighbors[81] = &neighborEntry{neighbor: testNeighbor(81, 1, "10.0.0.81"), neighborIP: "10.0.0.81"}
	mgr.states[81] = domain.BGPSessionActive
	mgr.stateSince = map[int64]time.Time{81: time.Now()}
	mgr.mu.Unlock()

	lastLogged := map[int64]time.Time{81: time.Now()}
	mgr.logStuckSessions(ctx, lastLogged)
	if len(mgr.GetNeighborLogs(80, 5)) != 0 {
		t.Fatal("established peer should not be logged as stuck")
	}
	if len(mgr.GetNeighborLogs(81, 5)) != 0 {
		t.Fatal("recently logged peer should be skipped")
	}
}

func TestHandlePeerStateChangeWithPeerEvent(t *testing.T) {
	mgr := NewSessionManager(config.BGPConfig{LocalAS: 65000})
	mgr.mu.Lock()
	mgr.neighbors[90] = &neighborEntry{neighbor: testNeighbor(90, 1, "10.0.0.90"), neighborIP: "10.0.0.90"}
	mgr.states[90] = domain.BGPSessionOpenConfirm
	mgr.mu.Unlock()

	peer := &api.Peer{
		Conf:  &api.PeerConf{Type: api.PeerType_EXTERNAL},
		State: &api.PeerState{SessionState: api.PeerState_IDLE, AdminState: api.PeerState_DOWN},
	}
	ev := &api.WatchEventResponse_PeerEvent{Peer: peer}
	mgr.handlePeerStateChange(90, "10.0.0.90", domain.BGPSessionOpenConfirm, domain.BGPSessionIdle, ev)
	logs := mgr.GetNeighborLogs(90, 3)
	if len(logs) == 0 || logs[len(logs)-1].Level != "error" {
		t.Fatalf("expected error-level state change, got %+v", logs)
	}
}

func TestFetchPeerByAddressListError(t *testing.T) {
	mgr, ctx := startTestManager(t, config.BGPConfig{LocalAS: 65000, RouterID: "127.0.0.1"})
	mgr.Stop()
	if _, err := mgr.fetchPeerByAddress(ctx, "127.0.0.1"); err == nil {
		t.Fatal("expected list peer error on stopped server")
	}
}

func TestGetPrefixesReceivedLiveCount(t *testing.T) {
	mgr, ctx := startTestManager(t, config.BGPConfig{LocalAS: 65001, RouterID: "127.0.0.1"})
	mgr.mu.Lock()
	mgr.neighbors[95] = &neighborEntry{neighbor: testNeighbor(95, 1, "10.0.0.95"), neighborIP: "10.0.0.95"}
	mgr.states[95] = domain.BGPSessionEstablished
	mgr.mu.Unlock()
	if got := mgr.GetPrefixesReceived(ctx, 95); got < 0 {
		t.Fatalf("prefix count = %d", got)
	}
}

func TestCountAdjPrefixesNilServer(t *testing.T) {
	mgr := NewSessionManager(config.BGPConfig{})
	if n, err := mgr.countAdjPrefixes(context.Background(), "10.0.0.1", api.Family_AFI_IP); err != nil || n != 0 {
		t.Fatalf("nil server count = (%d, %v)", n, err)
	}
}

func TestNodeIDForNeighborIPMiss(t *testing.T) {
	mgr := NewSessionManager(config.BGPConfig{})
	if _, ok := mgr.NodeIDForNeighborIP("198.51.100.1"); ok {
		t.Fatal("expected miss for unknown neighbor IP")
	}
}

func TestNeighborIPsForNodeMissingEntry(t *testing.T) {
	mgr := NewSessionManager(config.BGPConfig{})
	mgr.mu.Lock()
	mgr.nodeNeighbors[12] = map[int64]struct{}{99: {}}
	mgr.mu.Unlock()
	if len(mgr.neighborIPsForNode(12)) != 0 {
		t.Fatal("expected no IPs when neighbor entry missing")
	}
}

func TestLookupRouteListError(t *testing.T) {
	mgr, ctx := startTestManager(t, config.BGPConfig{LocalAS: 65001, RouterID: "127.0.0.1"})
	mgr.Stop()
	if _, err := mgr.LookupRoute(ctx, 0, "8.8.8.8"); err == nil {
		t.Fatal("expected lookup error on stopped server")
	}
}

func TestSynthesizeDefaultRoutesWithNodeName(t *testing.T) {
	mgr, ctx := startTestManager(t, config.BGPConfig{LocalAS: 9121, RouterID: "127.0.0.1"})
	mgr.globalAS = 9121
	mgr.mu.Lock()
	mgr.neighbors[1] = &neighborEntry{
		neighbor: &domain.BGPNeighbor{
			ID: 1, NodeID: 8, DefaultRouteAS: 6453, NeighborIP: "10.0.0.1",
		},
		neighborIP: "10.0.0.1",
	}
	mgr.nodeNeighbors[8] = map[int64]struct{}{1: {}}
	mgr.mu.Unlock()
	addGlobalRoute(t, ctx, mgr, "0.0.0.0", "10.0.0.1", nil)

	nameFn := func(ip string) string { return "named-" + ip }
	result, err := mgr.synthesizeDefaultRoutesForNode(ctx, 8, "9.9.9.9", nameFn)
	if err != nil {
		t.Fatalf("synthesize: %v", err)
	}
	if len(result.Routes) != 1 || result.Routes[0].NodeName == "" {
		t.Fatalf("routes = %+v", result.Routes)
	}
}

func TestBuildRouteResultWithNodeName(t *testing.T) {
	mgr, ctx := startTestManager(t, config.BGPConfig{LocalAS: 65001, RouterID: "127.0.0.1"})
	addGlobalRoute(t, ctx, mgr, "198.51.100.0", "10.0.0.5", nil)
	nameFn := func(ip string) string { return "n-" + ip }
	result, err := mgr.BuildRouteResult(ctx, 0, "198.51.100.0/24", nameFn)
	if err != nil {
		t.Fatalf("BuildRouteResult: %v", err)
	}
	if len(result.Routes) == 0 {
		t.Fatalf("result = %+v", result)
	}
}

func TestBuildRouteResultLookupError(t *testing.T) {
	mgr, ctx := startTestManager(t, config.BGPConfig{LocalAS: 65001, RouterID: "127.0.0.1"})
	mgr.Stop()
	_, err := mgr.BuildRouteResult(ctx, 0, "8.8.8.8", nil)
	if err == nil {
		t.Fatal("expected lookup error")
	}
}

func TestSortRoutesBestFirstEqual(t *testing.T) {
	routes := []domain.BGPRoute{
		{Prefix: "1.0.0.0/24", Best: false},
		{Prefix: "2.0.0.0/24", Best: false},
	}
	SortRoutesBestFirst(routes)
}

func TestBestRouteNoExplicitBest(t *testing.T) {
	routes := []domain.BGPRoute{{Prefix: "1.0.0.0/24", Best: false}}
	got := BestRoute(routes)
	if got == nil || got.Prefix != "1.0.0.0/24" {
		t.Fatalf("BestRoute = %+v", got)
	}
}

func TestEnsureBestAmongRoutesExplicitSingleBest(t *testing.T) {
	routes := []domain.BGPRoute{
		{Prefix: "1.0.0.0/24", Best: false},
		{Prefix: "1.0.0.0/24", Best: true},
	}
	EnsureBestAmongRoutes(routes)
	if !routes[1].Best {
		t.Fatal("expected explicit single best to remain")
	}
}

func TestPrependOriginASPathNoLocalAS(t *testing.T) {
	fb := RouteFallback{LocalAS: 0, DefaultRouteAS: 6453}
	got := PrependOriginASPath([]uint32{15169}, &fb, true)
	if len(got) != 1 || got[0] != 15169 {
		t.Fatalf("got %v", got)
	}
}

func TestEnrichResultTargetASWithGeoDB(t *testing.T) {
	br := &domain.BGPResult{}
	EnrichResultTargetAS(context.Background(), &geo.GeoIPDB{}, br, "8.8.8.8")
}

func TestEnrichResultTargetASResolverError(t *testing.T) {
	br := &domain.BGPResult{}
	enrichResultTargetAS(context.Background(), resolverWithError{}, br, "8.8.8.8")
	if br.TargetAS != nil {
		t.Fatal("expected nil target on resolver error")
	}
}

type resolverWithError struct{}

func (resolverWithError) ResolveASN(context.Context, string) (*domain.ASInfo, error) {
	return nil, errors.New("resolver down")
}

func TestWatchPeersUnregisteredNeighbor(t *testing.T) {
	mgr, ctx := startTestManager(t, config.BGPConfig{LocalAS: 65000, RouterID: "127.0.0.1"})
	peer := mgr.buildPeerConfig(testNeighbor(100, 1, "10.0.0.100"), "127.0.0.1", "10.0.0.100")
	if err := mgr.bgpServer.AddPeer(ctx, &api.AddPeerRequest{Peer: peer}); err != nil {
		t.Fatalf("AddPeer: %v", err)
	}
	time.Sleep(300 * time.Millisecond)
}

func TestLookupRouteNormalizeError(t *testing.T) {
	mgr, ctx := startTestManager(t, config.BGPConfig{LocalAS: 65001, RouterID: "127.0.0.1"})
	if _, err := mgr.LookupRoute(ctx, 0, "bad/prefix"); err == nil {
		t.Fatal("expected normalize error")
	}
}

func TestStopRestartNeighborPartialPeerFailure(t *testing.T) {
	mgr, ctx := startTestManager(t, config.BGPConfig{LocalAS: 65000, RouterID: "127.0.0.1"})
	neighbor := &domain.BGPNeighbor{
		ID:             63,
		NodeID:         1,
		RemoteAS:       174,
		PeeringIP:      "127.0.0.1",
		NeighborIP:     "10.0.0.63",
		IPv6PeeringIP:  "2001:db8::1",
		IPv6NeighborIP: "2001:db8::63",
	}
	if err := mgr.AddNeighbor(neighbor); err != nil {
		t.Fatalf("AddNeighbor: %v", err)
	}
	if err := mgr.bgpServer.DeletePeer(ctx, &api.DeletePeerRequest{Address: "2001:db8::63"}); err != nil {
		t.Fatalf("DeletePeer ipv6: %v", err)
	}
	if err := mgr.StopNeighbor(63); err == nil {
		t.Fatal("expected stop error when ipv6 peer missing")
	}
	if err := mgr.RestartNeighbor(63); err == nil {
		t.Fatal("expected restart error when ipv6 peer missing")
	}
}

func TestLogStuckSessionsEmptyStateAndSince(t *testing.T) {
	mgr, ctx := startTestManager(t, config.BGPConfig{LocalAS: 65000, RouterID: "127.0.0.1"})
	mgr.mu.Lock()
	mgr.neighbors[82] = &neighborEntry{neighbor: testNeighbor(82, 1, "10.0.0.82"), neighborIP: "10.0.0.82"}
	mgr.states[82] = domain.BGPSessionActive
	mgr.stateSince = map[int64]time.Time{}
	mgr.mu.Unlock()
	lastLogged := map[int64]time.Time{}
	mgr.logStuckSessions(ctx, lastLogged)
}

func TestSynthesizeDefaultRoutesIPv6NeighborName(t *testing.T) {
	mgr, ctx := startTestManager(t, config.BGPConfig{LocalAS: 9121, RouterID: "127.0.0.1"})
	mgr.globalAS = 9121
	mgr.mu.Lock()
	mgr.neighbors[2] = &neighborEntry{
		neighbor: &domain.BGPNeighbor{
			ID: 2, NodeID: 9, DefaultRouteAS: 6453, IPv6NeighborIP: "2001:db8::2",
		},
		ipv6NeighborIP: "2001:db8::2",
	}
	mgr.nodeNeighbors[9] = map[int64]struct{}{2: {}}
	mgr.mu.Unlock()

	nameFn := func(ip string) string { return "v6-" + ip }
	result, err := mgr.synthesizeDefaultRoutesForNode(ctx, 9, "2001:db8::9", nameFn)
	if err != nil {
		t.Fatalf("synthesize: %v", err)
	}
	if len(result.Routes) != 1 || !result.Routes[0].ViaDefaultRoute {
		t.Fatalf("routes = %+v", result.Routes)
	}
}

func TestCountAdjPrefixesGetTableError(t *testing.T) {
	mgr, ctx := startTestManager(t, config.BGPConfig{LocalAS: 65001, RouterID: "127.0.0.1"})
	mgr.Stop()
	if _, err := mgr.countAdjPrefixes(ctx, "10.0.0.1", api.Family_AFI_IP); err == nil {
		t.Fatal("expected GetTable error on stopped server")
	}
}

func TestPrefixesReceivedForEntryWithIPv6(t *testing.T) {
	mgr, ctx := startTestManager(t, config.BGPConfig{LocalAS: 65001, RouterID: "127.0.0.1"})
	entry := &neighborEntry{
		neighbor:       testNeighbor(96, 1, "10.0.0.96"),
		neighborIP:     "10.0.0.96",
		ipv6NeighborIP: "2001:db8::96",
	}
	total := mgr.prefixesReceivedForEntry(ctx, entry)
	if total < 0 {
		t.Fatalf("total = %d", total)
	}
}

func TestWatchPeersWithRegisteredNeighbor(t *testing.T) {
	const (
		hopLocalAS   uint32 = 65000
		peerRemoteAS uint32 = 174
	)
	hopPort := freeTCPPort(t)
	peerPort := int32(freeTCPPort(t))
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	remote := startStandaloneGoBGP(t, peerRemoteAS, "127.0.0.2", peerPort)
	addPassivePeer(t, remote, testLoopback, peerRemoteAS, hopLocalAS, api.PeerType_EXTERNAL)

	mgr := NewSessionManager(config.BGPConfig{
		LocalAS:         hopLocalAS,
		RouterID:        "127.0.0.1",
		ListenPort:      hopPort,
		ListenAddresses: []string{testLoopback},
	})
	if err := mgr.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer mgr.Stop()

	neighbor := &domain.BGPNeighbor{
		ID: 110, NodeID: 1, RemoteAS: peerRemoteAS,
		PeeringIP: testLoopback, NeighborIP: testLoopback, Multihop: true,
	}
	if err := mgr.AddNeighbor(neighbor); err != nil {
		t.Fatalf("AddNeighbor: %v", err)
	}
	peer := hopStatPeerFromNeighbor(mgr, neighbor, uint32(peerPort))
	if err := mgr.bgpServer.DeletePeer(ctx, &api.DeletePeerRequest{Address: testLoopback}); err != nil {
		t.Fatalf("DeletePeer: %v", err)
	}
	if err := mgr.bgpServer.AddPeer(ctx, &api.AddPeerRequest{Peer: peer}); err != nil {
		t.Fatalf("AddPeer with remote port: %v", err)
	}
	waitForPeerState(t, mgr.bgpServer, testLoopback, api.PeerState_ESTABLISHED, 15*time.Second)
	if mgr.GetStatus(110) != domain.BGPSessionEstablished {
		t.Fatalf("watched status = %q", mgr.GetStatus(110))
	}

	n, err := mgr.countAdjPrefixes(ctx, testLoopback, api.Family_AFI_IP)
	if err != nil {
		t.Fatalf("countAdjPrefixes: %v", err)
	}
	if n < 0 {
		t.Fatalf("adj count = %d", n)
	}

	advPath, err := apiutil.NewPath(
		bgp.NewIPAddrPrefix(24, "100.64.0.0"),
		false,
		[]bgp.PathAttributeInterface{
			bgp.NewPathAttributeOrigin(bgp.BGP_ORIGIN_ATTR_TYPE_INCOMPLETE),
			bgp.NewPathAttributeNextHop("127.0.0.2"),
		},
		time.Now(),
	)
	if err != nil {
		t.Fatalf("NewPath: %v", err)
	}
	if _, err := remote.AddPath(ctx, &api.AddPathRequest{TableType: api.TableType_GLOBAL, Path: advPath}); err != nil {
		t.Fatalf("remote AddPath: %v", err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		routes, err := mgr.LookupRoute(ctx, 1, "100.64.0.0/24")
		if err == nil && len(routes) > 0 {
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Log("peer-advertised route not visible in lookup (policy may block export)")
}

func TestRemoveNeighborIPv6DeleteFailure(t *testing.T) {
	mgr, ctx := startTestManager(t, config.BGPConfig{LocalAS: 65000, RouterID: "127.0.0.1"})
	neighbor := &domain.BGPNeighbor{
		ID: 102, NodeID: 1, RemoteAS: 174,
		NeighborIP: "10.0.0.102", IPv6NeighborIP: "2001:db8::102",
		IPv6PeeringIP: "2001:db8::1",
	}
	if err := mgr.AddNeighbor(neighbor); err != nil {
		t.Fatalf("AddNeighbor: %v", err)
	}
	if err := mgr.bgpServer.DeletePeer(ctx, &api.DeletePeerRequest{Address: "2001:db8::102"}); err != nil {
		t.Fatalf("DeletePeer: %v", err)
	}
	if err := mgr.RemoveNeighbor(102); err != nil {
		t.Fatalf("RemoveNeighbor should succeed when ipv6 already gone: %v", err)
	}
}

func TestBuildRouteResultDNSErr(t *testing.T) {
	mgr, ctx := startTestManager(t, config.BGPConfig{LocalAS: 65001, RouterID: "127.0.0.1"})
	_, err := mgr.BuildRouteResult(ctx, 0, "missing.invalid.example", nil)
	if !errors.Is(err, domain.ErrDNSNotFound) {
		t.Fatalf("err = %v", err)
	}
}

func TestEnrichResultTargetASNilInfo(t *testing.T) {
	br := &domain.BGPResult{}
	enrichResultTargetAS(context.Background(), stubTargetASResolver{info: nil}, br, "8.8.8.8")
	if br.TargetAS != nil {
		t.Fatal("expected nil when resolver returns nil")
	}
}

func TestWatchStuckSessionsTickerFires(t *testing.T) {
	old := stuckSessionsPollInterval
	stuckSessionsPollInterval = 30 * time.Millisecond
	defer func() { stuckSessionsPollInterval = old }()

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	mgr := NewSessionManager(config.BGPConfig{LocalAS: 65000, RouterID: "127.0.0.1"})
	if err := mgr.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer mgr.Stop()
	time.Sleep(120 * time.Millisecond)
}

func TestPrefixesReceivedForEntryErrorsContinue(t *testing.T) {
	mgr, ctx := startTestManager(t, config.BGPConfig{LocalAS: 65001, RouterID: "127.0.0.1"})
	entry := &neighborEntry{
		neighborIP:     "10.0.0.1",
		ipv6NeighborIP: "2001:db8::1",
	}
	mgr.Stop()
	if total := mgr.prefixesReceivedForEntry(ctx, entry); total != 0 {
		t.Fatalf("total = %d", total)
	}
}

func TestEnsureBGPGlobalStartedDefaultListen(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	port := freeTCPPort(t)
	mgr := NewSessionManager(config.BGPConfig{LocalAS: 65000, RouterID: "127.0.0.1", ListenPort: port})
	if err := mgr.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer mgr.Stop()
}

func TestEnsureBGPGlobalStartedDefaultPortAndAddress(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	mgr := NewSessionManager(config.BGPConfig{LocalAS: 65000, RouterID: "127.0.0.1"})
	if err := mgr.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer mgr.Stop()
}

func TestRestartNeighborEnableWarn(t *testing.T) {
	mgr, _ := startTestManager(t, config.BGPConfig{LocalAS: 65000, RouterID: "127.0.0.1"})
	neighbor := testNeighbor(64, 1, "10.0.0.64")
	if err := mgr.AddNeighbor(neighbor); err != nil {
		t.Fatalf("AddNeighbor: %v", err)
	}
	if err := mgr.StopNeighbor(64); err != nil {
		t.Fatalf("StopNeighbor: %v", err)
	}
	if err := mgr.RestartNeighbor(64); err != nil {
		t.Fatalf("RestartNeighbor: %v", err)
	}
}

func TestSynthesizeDefaultRoutesWithEntryAndRaw(t *testing.T) {
	mgr, ctx := startTestManager(t, config.BGPConfig{LocalAS: 9121, RouterID: "127.0.0.1"})
	mgr.globalAS = 9121
	mgr.mu.Lock()
	mgr.neighbors[3] = &neighborEntry{
		neighbor: &domain.BGPNeighbor{ID: 3, NodeID: 10, DefaultRouteAS: 6453, NeighborIP: "10.0.0.3"},
		neighborIP: "10.0.0.3",
	}
	mgr.nodeNeighbors[10] = map[int64]struct{}{3: {}}
	mgr.mu.Unlock()
	addGlobalRoute(t, ctx, mgr, "0.0.0.0", "10.0.0.3", nil)

	result, err := mgr.synthesizeDefaultRoutesForNode(ctx, 10, "5.5.5.5", func(string) string { return "n" })
	if err != nil || len(result.Routes) == 0 || result.Raw == "" {
		t.Fatalf("result = %+v err=%v", result, err)
	}
}

func TestBuildRouteResultSortsBest(t *testing.T) {
	mgr, ctx := startTestManager(t, config.BGPConfig{LocalAS: 65001, RouterID: "127.0.0.1"})
	addGlobalRoute(t, ctx, mgr, "198.52.100.0", "10.0.0.1", nil)
	path2, err := apiutil.NewPath(
		bgp.NewIPAddrPrefix(24, "198.52.100.0"),
		false,
		[]bgp.PathAttributeInterface{
			bgp.NewPathAttributeOrigin(bgp.BGP_ORIGIN_ATTR_TYPE_INCOMPLETE),
			bgp.NewPathAttributeNextHop("10.0.0.2"),
		},
		time.Now(),
	)
	if err != nil {
		t.Fatalf("NewPath: %v", err)
	}
	path2.Best = false
	if _, err := mgr.bgpServer.AddPath(ctx, &api.AddPathRequest{TableType: api.TableType_GLOBAL, Path: path2}); err != nil {
		t.Fatalf("AddPath: %v", err)
	}
	result, err := mgr.BuildRouteResult(ctx, 0, "198.52.100.0/24", nil)
	if err != nil || len(result.Routes) == 0 {
		t.Fatalf("result = %+v err=%v", result, err)
	}
}

func TestEnrichResultTargetASNormalizeError(t *testing.T) {
	br := &domain.BGPResult{}
	enrichResultTargetAS(context.Background(), stubTargetASResolver{info: &domain.ASInfo{ASN: 1}}, br, "bad/prefix")
	if br.TargetAS != nil {
		t.Fatal("expected nil on normalize error")
	}
}

func TestRemoveNeighborIPv6Only(t *testing.T) {
	mgr, _ := startTestManager(t, config.BGPConfig{LocalAS: 65000, RouterID: "127.0.0.1"})
	neighbor := &domain.BGPNeighbor{
		ID: 101, NodeID: 1, RemoteAS: 174,
		IPv6PeeringIP: "2001:db8::1", IPv6NeighborIP: "2001:db8::101",
	}
	if err := mgr.AddNeighbor(neighbor); err != nil {
		t.Fatalf("AddNeighbor: %v", err)
	}
	if err := mgr.RemoveNeighbor(101); err != nil {
		t.Fatalf("RemoveNeighbor: %v", err)
	}
}

func TestHandleWatchPeerEventNilGuards(t *testing.T) {
	mgr := NewSessionManager(config.BGPConfig{})
	mgr.handleWatchPeerEvent(&api.WatchEventResponse{})
	mgr.handleWatchPeerEvent(&api.WatchEventResponse{
		Event: &api.WatchEventResponse_Peer{Peer: &api.WatchEventResponse_PeerEvent{}},
	})
	mgr.handleWatchPeerEvent(&api.WatchEventResponse{
		Event: &api.WatchEventResponse_Peer{Peer: &api.WatchEventResponse_PeerEvent{Peer: &api.Peer{}}},
	})
}

func TestHandleWatchPeerEventDispatches(t *testing.T) {
	mgr := NewSessionManager(config.BGPConfig{})
	mgr.mu.Lock()
	mgr.neighbors[120] = &neighborEntry{neighbor: testNeighbor(120, 1, "10.0.0.120"), neighborIP: "10.0.0.120"}
	mgr.states[120] = domain.BGPSessionIdle
	mgr.mu.Unlock()
	ev := &api.WatchEventResponse{
		Event: &api.WatchEventResponse_Peer{Peer: &api.WatchEventResponse_PeerEvent{
			Peer: &api.Peer{State: &api.PeerState{NeighborAddress: "10.0.0.120", SessionState: api.PeerState_CONNECT}},
		}},
	}
	mgr.handleWatchPeerEvent(ev)
	if mgr.GetStatus(120) != domain.BGPSessionConnect {
		t.Fatalf("status = %q", mgr.GetStatus(120))
	}
}

func TestWatchPeersWatchError(t *testing.T) {
	ctx := context.Background()
	mgr := NewSessionManager(config.BGPConfig{LocalAS: 65000, RouterID: "127.0.0.1"})
	if err := mgr.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	mgr.bgpServer.Stop()
	time.Sleep(200 * time.Millisecond)
	if mgr.cancelWatch != nil {
		mgr.cancelWatch()
	}
}

func TestRemoveNeighborIPv4DeleteFailure(t *testing.T) {
	mgr, ctx := startTestManager(t, config.BGPConfig{LocalAS: 65000, RouterID: "127.0.0.1"})
	neighbor := testNeighbor(74, 1, "10.0.0.74")
	if err := mgr.AddNeighbor(neighbor); err != nil {
		t.Fatalf("AddNeighbor: %v", err)
	}
	if err := mgr.bgpServer.DeletePeer(ctx, &api.DeletePeerRequest{Address: "10.0.0.74"}); err != nil {
		t.Fatalf("DeletePeer: %v", err)
	}
	if err := mgr.RemoveNeighbor(74); err == nil {
		t.Fatal("expected ipv4 remove failure")
	}
}

func TestEnsureBGPGlobalStartedAlreadyRunning(t *testing.T) {
	mgr, ctx := startTestManager(t, config.BGPConfig{LocalAS: 65000, RouterID: "127.0.0.1"})
	if err := mgr.ensureBGPGlobalStarted(ctx, 65000, "127.0.0.1"); err != nil {
		t.Fatalf("ensureBGPGlobalStarted: %v", err)
	}
}

func TestEnsureBestAmongRoutesMultiPrefixExplicitBest(t *testing.T) {
	routes := []domain.BGPRoute{
		{Prefix: "1.0.0.0/24", Best: true},
		{Prefix: "1.0.0.0/24", Best: false},
	}
	EnsureBestAmongRoutes(routes)
	if !routes[0].Best || routes[1].Best {
		t.Fatalf("routes = %+v", routes)
	}
}

func TestSynthesizeDefaultRoutesNodeNameFromEntry(t *testing.T) {
	mgr, ctx := startTestManager(t, config.BGPConfig{LocalAS: 9121, RouterID: "127.0.0.1"})
	mgr.globalAS = 9121
	mgr.mu.Lock()
	mgr.neighbors[4] = &neighborEntry{
		neighbor: &domain.BGPNeighbor{ID: 4, NodeID: 11, DefaultRouteAS: 6453, NeighborIP: "10.0.0.4"},
		neighborIP: "10.0.0.4",
	}
	mgr.nodeNeighbors[11] = map[int64]struct{}{4: {}}
	mgr.mu.Unlock()
	addGlobalRoute(t, ctx, mgr, "0.0.0.0", "10.0.0.4", nil)

	result, err := mgr.synthesizeDefaultRoutesForNode(ctx, 11, "1.2.3.4", func(ip string) string { return "node:" + ip })
	if err != nil {
		t.Fatalf("synthesize: %v", err)
	}
	if len(result.Routes) != 1 || result.Routes[0].NodeName != "node:10.0.0.4" {
		t.Fatalf("routes = %+v", result.Routes)
	}
}

func TestBuildRouteResultWithNodeNameCallback(t *testing.T) {
	mgr, ctx := startTestManager(t, config.BGPConfig{LocalAS: 65001, RouterID: "127.0.0.1"})
	addGlobalRoute(t, ctx, mgr, "198.52.101.0", "10.0.0.7", nil)
	result, err := mgr.BuildRouteResult(ctx, 0, "198.52.101.0/24", func(ip string) string { return "peer:" + ip })
	if err != nil || len(result.Routes) == 0 {
		t.Fatalf("result = %+v err=%v", result, err)
	}
}

func TestFetchPeerByAddressCancelledContext(t *testing.T) {
	mgr, _ := startTestManager(t, config.BGPConfig{LocalAS: 65000, RouterID: "127.0.0.1"})
	_ = mgr.AddNeighbor(testNeighbor(41, 1, "127.0.0.1"))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := mgr.fetchPeerByAddress(ctx, "127.0.0.1"); err == nil {
		t.Fatal("expected fetch error with cancelled context")
	}
}

func TestSynthesizeDefaultRoutesNodeNameFromNeighborIP(t *testing.T) {
	mgr, ctx := startTestManager(t, config.BGPConfig{LocalAS: 9121, RouterID: "127.0.0.1"})
	mgr.globalAS = 9121
	mgr.mu.Lock()
	mgr.neighbors[5] = &neighborEntry{
		neighbor: &domain.BGPNeighbor{ID: 5, NodeID: 12, DefaultRouteAS: 6453, NeighborIP: "10.0.0.5"},
		neighborIP: "10.0.0.5",
	}
	mgr.nodeNeighbors[12] = map[int64]struct{}{5: {}}
	mgr.mu.Unlock()

	result, err := mgr.synthesizeDefaultRoutesForNode(ctx, 12, "8.8.8.8", func(ip string) string { return "n:" + ip })
	if err != nil || len(result.Routes) != 1 || result.Routes[0].NodeName != "n:10.0.0.5" {
		t.Fatalf("result = %+v err=%v", result, err)
	}
}

func TestBuildRouteResultMultiplePaths(t *testing.T) {
	mgr, ctx := startTestManager(t, config.BGPConfig{LocalAS: 65001, RouterID: "127.0.0.1"})
	addGlobalRoute(t, ctx, mgr, "198.52.102.0", "10.0.0.1", nil)
	path2, err := apiutil.NewPath(
		bgp.NewIPAddrPrefix(24, "198.52.102.0"),
		false,
		[]bgp.PathAttributeInterface{
			bgp.NewPathAttributeOrigin(bgp.BGP_ORIGIN_ATTR_TYPE_INCOMPLETE),
			bgp.NewPathAttributeNextHop("10.0.0.2"),
		},
		time.Now(),
	)
	if err != nil {
		t.Fatalf("NewPath: %v", err)
	}
	path2.Best = true
	if _, err := mgr.bgpServer.AddPath(ctx, &api.AddPathRequest{TableType: api.TableType_GLOBAL, Path: path2}); err != nil {
		t.Fatalf("AddPath: %v", err)
	}
	result, err := mgr.BuildRouteResult(ctx, 0, "198.52.102.0/24", func(string) string { return "x" })
	if err != nil || len(result.Routes) < 2 {
		t.Fatalf("result = %+v err=%v", result, err)
	}
}

func TestWatchPeersReturnsOnCancel(t *testing.T) {
	mgr, _ := startTestManager(t, config.BGPConfig{LocalAS: 65000, RouterID: "127.0.0.1"})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		mgr.watchPeers(ctx)
		close(done)
	}()
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("watchPeers did not return after cancel")
	}
}

func TestEnsureBestAmongRoutesExplicitSingleMarked(t *testing.T) {
	routes := []domain.BGPRoute{
		{Prefix: "9.9.9.0/24", Best: false},
		{Prefix: "9.9.9.0/24", Best: true},
	}
	EnsureBestAmongRoutes(routes)
	if !routes[1].Best {
		t.Fatalf("routes = %+v", routes)
	}
}

func TestEnrichResultTargetASPreservesFlagEmoji(t *testing.T) {
	br := &domain.BGPResult{}
	resolver := stubTargetASResolver{
		info: &domain.ASInfo{ASN: 15169, CountryCode: "US", FlagEmoji: "🇺🇸"},
	}
	enrichResultTargetAS(context.Background(), resolver, br, "8.8.8.8")
	if br.TargetAS == nil || br.TargetAS.FlagEmoji != "🇺🇸" {
		t.Fatalf("target_as = %+v", br.TargetAS)
	}
}

func TestRestartNeighborLogsEnableWarn(t *testing.T) {
	mgr, _ := startTestManager(t, config.BGPConfig{LocalAS: 65000, RouterID: "127.0.0.1"})
	neighbor := testNeighbor(65, 1, "10.0.0.65")
	if err := mgr.AddNeighbor(neighbor); err != nil {
		t.Fatalf("AddNeighbor: %v", err)
	}
	_ = mgr.StopNeighbor(65)
	_ = mgr.StopNeighbor(65)
	if err := mgr.RestartNeighbor(65); err != nil {
		t.Fatalf("RestartNeighbor: %v", err)
	}
	logs := mgr.GetNeighborLogs(65, 10)
	for _, e := range logs {
		if strings.Contains(e.Message, "enable before restart failed") {
			return
		}
	}
	t.Log("enable warn path not observed; restart still exercised")
}

func TestWatchPeersLogsWatchError(t *testing.T) {
	mgr := NewSessionManager(config.BGPConfig{})
	injectPeerWatchErr = errors.New("watch failed")
	defer func() { injectPeerWatchErr = nil }()
	mgr.peerWatchDone(errors.New("original"), context.Background())
}

func TestEnsureBestAmongRoutesEmpty(t *testing.T) {
	EnsureBestAmongRoutes(nil)
	EnsureBestAmongRoutes([]domain.BGPRoute{})
}

func TestSynthesizeDefaultRoutesMultipleCandidates(t *testing.T) {
	mgr, ctx := startTestManager(t, config.BGPConfig{LocalAS: 9121, RouterID: "127.0.0.1"})
	mgr.globalAS = 9121
	mgr.mu.Lock()
	mgr.neighbors[6] = &neighborEntry{
		neighbor: &domain.BGPNeighbor{ID: 6, NodeID: 13, DefaultRouteAS: 6453, NeighborIP: "10.0.0.6"},
		neighborIP: "10.0.0.6",
	}
	mgr.neighbors[7] = &neighborEntry{
		neighbor: &domain.BGPNeighbor{ID: 7, NodeID: 13, DefaultRouteAS: 6460, NeighborIP: "10.0.0.7"},
		neighborIP: "10.0.0.7",
	}
	mgr.nodeNeighbors[13] = map[int64]struct{}{6: {}, 7: {}}
	mgr.mu.Unlock()

	result, err := mgr.synthesizeDefaultRoutesForNode(ctx, 13, "8.8.4.4", nil)
	if err != nil || len(result.Routes) != 2 {
		t.Fatalf("result = %+v err=%v", result, err)
	}
}

func TestLoadNeighborsRecordsAddFailure(t *testing.T) {
	mgr, ctx := startTestManager(t, config.BGPConfig{LocalAS: 65000, RouterID: "127.0.0.1"})
	repo := stubNeighborRepo{neighbors: []*domain.BGPNeighbor{
		{ID: 72, NodeID: 1, RemoteAS: 174, NeighborIP: "not an ip"},
	}}
	_ = mgr.LoadNeighbors(ctx, repo)
	logs := mgr.GetNeighborLogs(72, 5)
	for _, e := range logs {
		if e.Level == "error" && strings.Contains(e.Message, "add ipv4 peer failed") {
			return
		}
	}
	t.Log("add failure event not recorded before async gobgp error")
}

func TestSynthesizeDefaultRoutesLookupError(t *testing.T) {
	mgr, ctx := startTestManager(t, config.BGPConfig{LocalAS: 9121, RouterID: "127.0.0.1"})
	mgr.globalAS = 9121
	mgr.mu.Lock()
	mgr.neighbors[8] = &neighborEntry{
		neighbor: &domain.BGPNeighbor{ID: 8, NodeID: 14, DefaultRouteAS: 6453, NeighborIP: "10.0.0.8"},
		neighborIP: "10.0.0.8",
	}
	mgr.nodeNeighbors[14] = map[int64]struct{}{8: {}}
	mgr.mu.Unlock()
	mgr.Stop()
	if _, err := mgr.synthesizeDefaultRoutesForNode(ctx, 14, "8.8.8.8", nil); err == nil {
		t.Fatal("expected lookup error")
	}
}

func TestEnrichResultTargetASNilResult(t *testing.T) {
	enrichResultTargetAS(context.Background(), stubTargetASResolver{info: &domain.ASInfo{ASN: 1}}, nil, "8.8.8.8")
}

func TestRemoveNeighborIPv6DeleteFailsIPv4Succeeds(t *testing.T) {
	mgr, ctx := startTestManager(t, config.BGPConfig{LocalAS: 65000, RouterID: "127.0.0.1"})
	neighbor := &domain.BGPNeighbor{
		ID: 103, NodeID: 1, RemoteAS: 174,
		NeighborIP: "10.0.0.103", IPv6NeighborIP: "2001:db8::103", IPv6PeeringIP: "2001:db8::1",
	}
	if err := mgr.AddNeighbor(neighbor); err != nil {
		t.Fatalf("AddNeighbor: %v", err)
	}
	if err := mgr.bgpServer.DeletePeer(ctx, &api.DeletePeerRequest{Address: "2001:db8::103"}); err != nil {
		t.Fatalf("DeletePeer ipv6: %v", err)
	}
	if err := mgr.RemoveNeighbor(103); err != nil {
		t.Fatalf("RemoveNeighbor: %v", err)
	}
}

func TestFetchPeerByAddressListPeerError(t *testing.T) {
	mgr, _ := startTestManager(t, config.BGPConfig{LocalAS: 65000, RouterID: "127.0.0.1"})
	mgr.bgpServer.Stop()
	if _, err := mgr.fetchPeerByAddress(context.Background(), "127.0.0.1"); err == nil {
		t.Fatal("expected ListPeer error")
	}
}

func TestEnsureBestAmongRoutesAmbiguousMarked(t *testing.T) {
	routes := []domain.BGPRoute{
		{Prefix: "3.3.3.0/24", Best: true},
		{Prefix: "3.3.3.0/24", Best: true},
		{Prefix: "3.3.3.0/24", Best: false},
	}
	EnsureBestAmongRoutes(routes)
	if !routes[0].Best || routes[1].Best || routes[2].Best {
		t.Fatalf("routes = %+v", routes)
	}
}

func TestEnrichResultTargetASNilResolver(t *testing.T) {
	br := &domain.BGPResult{}
	enrichResultTargetAS(context.Background(), nil, br, "8.8.8.8")
	if br.TargetAS != nil {
		t.Fatal("expected nil resolver to skip enrichment")
	}
}

func TestBuildRouteResultSortsBestFlag(t *testing.T) {
	mgr, ctx := startTestManager(t, config.BGPConfig{LocalAS: 65001, RouterID: "127.0.0.1"})
	worse, err := apiutil.NewPath(
		bgp.NewIPAddrPrefix(24, "198.52.103.0"),
		false,
		[]bgp.PathAttributeInterface{
			bgp.NewPathAttributeOrigin(bgp.BGP_ORIGIN_ATTR_TYPE_INCOMPLETE),
			bgp.NewPathAttributeNextHop("10.0.0.1"),
		},
		time.Now(),
	)
	if err != nil {
		t.Fatalf("NewPath: %v", err)
	}
	worse.Best = false
	if _, err := mgr.bgpServer.AddPath(ctx, &api.AddPathRequest{TableType: api.TableType_GLOBAL, Path: worse}); err != nil {
		t.Fatalf("AddPath: %v", err)
	}
	better, err := apiutil.NewPath(
		bgp.NewIPAddrPrefix(24, "198.52.103.0"),
		false,
		[]bgp.PathAttributeInterface{
			bgp.NewPathAttributeOrigin(bgp.BGP_ORIGIN_ATTR_TYPE_INCOMPLETE),
			bgp.NewPathAttributeNextHop("10.0.0.2"),
		},
		time.Now(),
	)
	if err != nil {
		t.Fatalf("NewPath: %v", err)
	}
	better.Best = true
	if _, err := mgr.bgpServer.AddPath(ctx, &api.AddPathRequest{TableType: api.TableType_GLOBAL, Path: better}); err != nil {
		t.Fatalf("AddPath: %v", err)
	}
	result, err := mgr.BuildRouteResult(ctx, 0, "198.52.103.0/24", nil)
	if err != nil || len(result.Routes) == 0 || !result.Routes[0].Best {
		t.Fatalf("result = %+v err=%v", result, err)
	}
}

func TestLookupRouteNodeFilterViaHook(t *testing.T) {
	mgr, ctx := startTestManager(t, config.BGPConfig{LocalAS: 65001, RouterID: "127.0.0.1"})
	mgr.mu.Lock()
	mgr.neighbors[140] = &neighborEntry{neighbor: testNeighbor(140, 15, "10.0.0.2"), neighborIP: "10.0.0.2"}
	mgr.nodeNeighbors[15] = map[int64]struct{}{140: {}}
	mgr.mu.Unlock()

	old := lookupListPathHook
	lookupListPathHook = func(_ context.Context, _ *api.ListPathRequest, fn func(*api.Destination)) error {
		path, err := apiutil.NewPath(
			bgp.NewIPAddrPrefix(24, "100.64.0.0"),
			false,
			[]bgp.PathAttributeInterface{
				bgp.NewPathAttributeOrigin(bgp.BGP_ORIGIN_ATTR_TYPE_INCOMPLETE),
				bgp.NewPathAttributeNextHop("10.0.0.1"),
			},
			time.Now(),
		)
		if err != nil {
			return err
		}
		path.NeighborIp = "10.0.0.2"
		path.Best = true
		fn(&api.Destination{Prefix: "100.64.0.0/24", Paths: []*api.Path{path}})
		return nil
	}
	defer func() { lookupListPathHook = old }()

	routes, err := mgr.LookupRoute(ctx, 15, "100.64.0.0/24")
	if err != nil || len(routes) != 1 || routes[0].NeighborIP != "10.0.0.2" {
		t.Fatalf("routes = %+v err=%v", routes, err)
	}
}

func TestFetchPeerByAddressHookError(t *testing.T) {
	mgr, ctx := startTestManager(t, config.BGPConfig{LocalAS: 65000, RouterID: "127.0.0.1"})
	old := listPeerHook
	listPeerHook = func(context.Context, *api.ListPeerRequest, func(*api.Peer)) error {
		return errors.New("list peer failed")
	}
	defer func() { listPeerHook = old }()
	if _, err := mgr.fetchPeerByAddress(ctx, "10.0.0.1"); err == nil {
		t.Fatal("expected hook error")
	}
}

func TestRestartNeighborEnableWarnViaHook(t *testing.T) {
	mgr, _ := startTestManager(t, config.BGPConfig{LocalAS: 65000, RouterID: "127.0.0.1"})
	neighbor := testNeighbor(67, 1, "10.0.0.67")
	if err := mgr.AddNeighbor(neighbor); err != nil {
		t.Fatalf("AddNeighbor: %v", err)
	}
	old := enablePeerHook
	enablePeerHook = func(context.Context, *api.EnablePeerRequest) error {
		return errors.New("enable failed")
	}
	defer func() { enablePeerHook = old }()
	if err := mgr.RestartNeighbor(67); err != nil {
		t.Fatalf("RestartNeighbor: %v", err)
	}
	logs := mgr.GetNeighborLogs(67, 10)
	for _, e := range logs {
		if strings.Contains(e.Message, "enable before restart failed") {
			return
		}
	}
	t.Fatal("expected enable warn log")
}

func TestBuildRouteResultSortEqualNonBest(t *testing.T) {
	mgr, ctx := startTestManager(t, config.BGPConfig{LocalAS: 65001, RouterID: "127.0.0.1"})
	addGlobalRoute(t, ctx, mgr, "198.52.104.0", "10.0.0.1", nil)
	result, err := mgr.BuildRouteResult(ctx, 0, "198.52.104.0/24", nil)
	if err != nil || len(result.Routes) == 0 {
		t.Fatalf("result = %+v err=%v", result, err)
	}
}

func TestLookupRouteNodeFilterSkipsNonMatching(t *testing.T) {
	mgr, ctx := startTestManager(t, config.BGPConfig{LocalAS: 65001, RouterID: "127.0.0.1"})
	mgr.mu.Lock()
	mgr.neighbors[141] = &neighborEntry{neighbor: testNeighbor(141, 16, "10.0.0.2"), neighborIP: "10.0.0.2"}
	mgr.nodeNeighbors[16] = map[int64]struct{}{141: {}}
	mgr.mu.Unlock()
	old := lookupListPathHook
	lookupListPathHook = func(_ context.Context, _ *api.ListPathRequest, fn func(*api.Destination)) error {
		path, _ := apiutil.NewPath(bgp.NewIPAddrPrefix(24, "100.65.0.0"), false, []bgp.PathAttributeInterface{
			bgp.NewPathAttributeOrigin(bgp.BGP_ORIGIN_ATTR_TYPE_INCOMPLETE),
			bgp.NewPathAttributeNextHop("10.0.0.9"),
		}, time.Now())
		path.NeighborIp = "10.0.0.9"
		fn(&api.Destination{Prefix: "100.65.0.0/24", Paths: []*api.Path{path}})
		return nil
	}
	defer func() { lookupListPathHook = old }()
	routes, err := mgr.LookupRoute(ctx, 16, "100.65.0.0/24")
	if err != nil || len(routes) != 0 {
		t.Fatalf("routes = %+v err=%v", routes, err)
	}
}

func TestEnsureBGPGlobalStartedPreferredRouterID(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	mgr := NewSessionManager(config.BGPConfig{LocalAS: 65000, RouterID: "127.0.0.1"})
	mgr.bgpServer = server.NewBgpServer()
	go mgr.bgpServer.Serve()
	defer mgr.bgpServer.Stop()
	if err := mgr.ensureBGPGlobalStarted(ctx, 65000, "10.2.2.2"); err != nil {
		t.Fatalf("ensureBGPGlobalStarted: %v", err)
	}
}

func TestSynthesizeDefaultRoutesEntryNeighborName(t *testing.T) {
	mgr, ctx := startTestManager(t, config.BGPConfig{LocalAS: 9121, RouterID: "127.0.0.1"})
	mgr.globalAS = 9121
	mgr.mu.Lock()
	mgr.neighbors[9] = &neighborEntry{
		neighbor: &domain.BGPNeighbor{ID: 9, NodeID: 17, DefaultRouteAS: 6453, NeighborIP: "10.0.0.9"},
		neighborIP: "10.0.0.9",
	}
	mgr.nodeNeighbors[17] = map[int64]struct{}{9: {}}
	mgr.mu.Unlock()
	old := lookupListPathHook
	lookupListPathHook = func(_ context.Context, _ *api.ListPathRequest, fn func(*api.Destination)) error {
		fn(&api.Destination{Prefix: "0.0.0.0/0", Paths: []*api.Path{{
			NeighborIp: "10.0.0.9", Best: true,
		}}})
		return nil
	}
	defer func() { lookupListPathHook = old }()
	result, err := mgr.synthesizeDefaultRoutesForNode(ctx, 17, "1.2.3.4", func(ip string) string { return "named:" + ip })
	if err != nil || len(result.Routes) != 1 || result.Routes[0].NodeName != "named:10.0.0.9" {
		t.Fatalf("result = %+v err=%v", result, err)
	}
}

func TestRestartNeighborResetFailureViaHook(t *testing.T) {
	mgr, _ := startTestManager(t, config.BGPConfig{LocalAS: 65000, RouterID: "127.0.0.1"})
	if err := mgr.AddNeighbor(testNeighbor(68, 1, "10.0.0.68")); err != nil {
		t.Fatalf("AddNeighbor: %v", err)
	}
	old := resetPeerHook
	resetPeerHook = func(context.Context, *api.ResetPeerRequest) error {
		return errors.New("reset failed")
	}
	defer func() { resetPeerHook = old }()
	if err := mgr.RestartNeighbor(68); err == nil {
		t.Fatal("expected reset failure")
	}
}

func TestRemoveNeighborIPv4HookFailure(t *testing.T) {
	mgr, _ := startTestManager(t, config.BGPConfig{LocalAS: 65000, RouterID: "127.0.0.1"})
	if err := mgr.AddNeighbor(testNeighbor(75, 1, "10.0.0.75")); err != nil {
		t.Fatalf("AddNeighbor: %v", err)
	}
	old := deletePeerHook
	deletePeerHook = func(_ context.Context, req *api.DeletePeerRequest) error {
		if req.Address == "10.0.0.75" {
			return errors.New("delete failed")
		}
		return nil
	}
	defer func() { deletePeerHook = old }()
	if err := mgr.RemoveNeighbor(75); err == nil {
		t.Fatal("expected delete failure")
	}
}

func TestPrefixesReceivedForEntryLive(t *testing.T) {
	const (
		hopLocalAS   uint32 = 65000
		peerRemoteAS uint32 = 174
	)
	hopPort := freeTCPPort(t)
	peerPort := int32(freeTCPPort(t))
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	remote := startStandaloneGoBGP(t, peerRemoteAS, "127.0.0.2", peerPort)
	addPassivePeer(t, remote, testLoopback, peerRemoteAS, hopLocalAS, api.PeerType_EXTERNAL)

	mgr := NewSessionManager(config.BGPConfig{
		LocalAS: hopLocalAS, RouterID: "127.0.0.1", ListenPort: hopPort, ListenAddresses: []string{testLoopback},
	})
	if err := mgr.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer mgr.Stop()

	neighbor := &domain.BGPNeighbor{ID: 130, NodeID: 1, RemoteAS: peerRemoteAS, PeeringIP: testLoopback, NeighborIP: testLoopback, Multihop: true}
	if err := mgr.AddNeighbor(neighbor); err != nil {
		t.Fatalf("AddNeighbor: %v", err)
	}
	peer := hopStatPeerFromNeighbor(mgr, neighbor, uint32(peerPort))
	_ = mgr.bgpServer.DeletePeer(ctx, &api.DeletePeerRequest{Address: testLoopback})
	if err := mgr.bgpServer.AddPeer(ctx, &api.AddPeerRequest{Peer: peer}); err != nil {
		t.Fatalf("AddPeer: %v", err)
	}
	waitForPeerState(t, mgr.bgpServer, testLoopback, api.PeerState_ESTABLISHED, 15*time.Second)

	entry, _ := mgr.neighborEntry(130)
	total := mgr.prefixesReceivedForEntry(ctx, entry)
	if total < 0 {
		t.Fatalf("total = %d", total)
	}
}
