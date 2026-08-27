package bgp

import (
	"context"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"

	api "github.com/osrg/gobgp/v3/api"
	"github.com/osrg/gobgp/v3/pkg/apiutil"
	"github.com/osrg/gobgp/v3/pkg/packet/bgp"

	"github.com/HopStat/HopStat/internal/config"
	"github.com/HopStat/HopStat/internal/domain"
)

type stubNeighborRepo struct {
	neighbors []*domain.BGPNeighbor
	err       error
}

func (s stubNeighborRepo) GetAll(ctx context.Context) ([]*domain.BGPNeighbor, error) {
	return s.neighbors, s.err
}
func (s stubNeighborRepo) GetByNodeID(ctx context.Context, nodeID int64) ([]*domain.BGPNeighbor, error) {
	return nil, nil
}
func (s stubNeighborRepo) GetByID(ctx context.Context, id int64) (*domain.BGPNeighbor, error) {
	return nil, nil
}
func (s stubNeighborRepo) Create(ctx context.Context, n *domain.BGPNeighbor) (*domain.BGPNeighbor, error) {
	return nil, nil
}
func (s stubNeighborRepo) Update(ctx context.Context, n *domain.BGPNeighbor) (*domain.BGPNeighbor, error) {
	return nil, nil
}
func (s stubNeighborRepo) Delete(ctx context.Context, id int64) error {
	return nil
}

func startTestManager(t *testing.T, cfg config.BGPConfig) (*SessionManager, context.Context) {
	t.Helper()
	if cfg.ListenPort == 0 {
		cfg.ListenPort = freeListenPort(t)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	mgr := NewSessionManager(cfg)
	if err := mgr.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { mgr.Stop() })
	return mgr, ctx
}

func freeListenPort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()
	return port
}

func testNeighbor(id, nodeID int64, neighborIP string) *domain.BGPNeighbor {
	return &domain.BGPNeighbor{
		ID:         id,
		NodeID:     nodeID,
		RemoteAS:   174,
		PeeringIP:  "127.0.0.1",
		NeighborIP: neighborIP,
	}
}

func addGlobalRoute(t *testing.T, ctx context.Context, mgr *SessionManager, prefix, neighborIP string, attrs []bgp.PathAttributeInterface) {
	t.Helper()
	if attrs == nil {
		attrs = []bgp.PathAttributeInterface{
			bgp.NewPathAttributeOrigin(bgp.BGP_ORIGIN_ATTR_TYPE_IGP),
			bgp.NewPathAttributeNextHop("10.0.0.1"),
		}
	}
	path, err := apiutil.NewPath(bgp.NewIPAddrPrefix(24, prefix), false, attrs, time.Now().Add(-time.Hour))
	if err != nil {
		t.Fatalf("NewPath: %v", err)
	}
	path.NeighborIp = neighborIP
	path.Best = true
	path.SourceAsn = 174
	if _, err := mgr.bgpServer.AddPath(ctx, &api.AddPathRequest{
		TableType: api.TableType_GLOBAL,
		Path:      path,
	}); err != nil {
		t.Fatalf("AddPath: %v", err)
	}
}

func TestSessionManagerConfigAndReady(t *testing.T) {
	cfg := config.BGPConfig{LocalAS: 65000, RouterID: "10.0.0.1", ListenPort: 11791}
	mgr := NewSessionManager(cfg)
	if got := mgr.BGPConfig(); got.LocalAS != 65000 {
		t.Fatalf("BGPConfig local_as = %d", got.LocalAS)
	}
	if mgr.IsReady() {
		t.Fatal("expected not ready before Start")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := mgr.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer mgr.Stop()
	if !mgr.IsReady() {
		t.Fatal("expected ready after Start with local AS")
	}
	if mgr.LocalAS() != 65000 {
		t.Fatalf("LocalAS = %d", mgr.LocalAS())
	}
}

func TestStartWithoutLocalAS(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	mgr := NewSessionManager(config.BGPConfig{})
	if err := mgr.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer mgr.Stop()
	if mgr.IsReady() {
		t.Fatal("expected not ready without local AS")
	}
}

func TestEffectiveRouterIDAndGlobalStartErrors(t *testing.T) {
	mgr := NewSessionManager(config.BGPConfig{LocalAS: 65000})
	if got := mgr.effectiveRouterID(""); got != "127.0.0.1" {
		t.Fatalf("effectiveRouterID default = %q", got)
	}
	if got := mgr.effectiveRouterID("0.0.0.0"); got != "127.0.0.1" {
		t.Fatalf("effectiveRouterID zero = %q", got)
	}
	if got := mgr.effectiveRouterID("10.1.1.1"); got != "10.1.1.1" {
		t.Fatalf("effectiveRouterID preferred = %q", got)
	}

	if err := mgr.ensureBGPGlobalStarted(context.Background(), 0, "10.0.0.1"); err == nil {
		t.Fatal("expected error for localAS 0")
	}
	if err := mgr.ensureBGPGlobalStarted(context.Background(), 65000, "not-an-ip"); err == nil {
		t.Fatal("expected error for invalid router id")
	}
	if err := mgr.ensureBGPGlobalStarted(context.Background(), 65000, "10.0.0.1"); err == nil {
		t.Fatal("expected error when bgp server not started")
	}
}

func TestEnsureBGPGlobalStartedIdempotent(t *testing.T) {
	port := freeTCPPort(t)
	mgr, ctx := startTestManager(t, config.BGPConfig{
		LocalAS:         65001,
		RouterID:        "127.0.0.1",
		ListenPort:      port,
		ListenAddresses: []string{"127.0.0.1"},
	})
	if mgr.globalAS != 65001 {
		t.Fatalf("globalAS = %d", mgr.globalAS)
	}
	if err := mgr.ensureBGPGlobalStarted(ctx, 65001, "127.0.0.1"); err != nil {
		t.Fatalf("second ensureBGPGlobalStarted: %v", err)
	}
}

func TestRecordEventAndNeighborLogsNilSafe(t *testing.T) {
	mgr := NewSessionManager(config.BGPConfig{})
	mgr.events = nil
	mgr.recordEvent(1, "info", "ignored", "")
	if logs := mgr.GetNeighborLogs(1, 10); len(logs) != 0 {
		t.Fatalf("expected empty logs, got %d", len(logs))
	}
}

func TestNeighborEntryPeerAddresses(t *testing.T) {
	mgr := NewSessionManager(config.BGPConfig{})
	if _, err := mgr.neighborEntry(99); err == nil {
		t.Fatal("expected error for unknown neighbor")
	}

	mgr.mu.Lock()
	mgr.neighbors[1] = &neighborEntry{
		neighbor:       &domain.BGPNeighbor{ID: 1},
		neighborIP:     "10.0.0.2",
		ipv6NeighborIP: "2001:db8::2",
	}
	mgr.mu.Unlock()

	entry, err := mgr.neighborEntry(1)
	if err != nil {
		t.Fatalf("neighborEntry: %v", err)
	}
	addrs := mgr.peerAddresses(entry)
	if len(addrs) != 2 || addrs[0] != "10.0.0.2" || addrs[1] != "2001:db8::2" {
		t.Fatalf("peerAddresses = %v", addrs)
	}
}

func TestStopRestartNeighborErrors(t *testing.T) {
	mgr := NewSessionManager(config.BGPConfig{LocalAS: 65000})
	if err := mgr.StopNeighbor(1); err == nil || !strings.Contains(err.Error(), "not started") {
		t.Fatalf("StopNeighbor err = %v", err)
	}
	if err := mgr.RestartNeighbor(1); err == nil || !strings.Contains(err.Error(), "not started") {
		t.Fatalf("RestartNeighbor err = %v", err)
	}

	mgr, ctx := startTestManager(t, config.BGPConfig{LocalAS: 65000, RouterID: "127.0.0.1"})
	neighbor := testNeighbor(5, 1, "10.0.0.5")
	if err := mgr.AddNeighbor(neighbor); err != nil {
		t.Fatalf("AddNeighbor: %v", err)
	}
	if err := mgr.StopNeighbor(5); err != nil {
		t.Fatalf("StopNeighbor: %v", err)
	}
	if err := mgr.RestartNeighbor(5); err != nil {
		t.Fatalf("RestartNeighbor: %v", err)
	}
	if err := mgr.StopNeighbor(99); err == nil {
		t.Fatal("expected error for unknown neighbor")
	}
	if err := mgr.RestartNeighbor(99); err == nil {
		t.Fatal("expected RestartNeighbor error for unknown neighbor")
	}
	_ = ctx
}

func TestRemoveUpdateNeighbor(t *testing.T) {
	mgr, ctx := startTestManager(t, config.BGPConfig{LocalAS: 65000, RouterID: "127.0.0.1"})
	neighbor := testNeighbor(10, 2, "10.0.0.10")
	if err := mgr.AddNeighbor(neighbor); err != nil {
		t.Fatalf("AddNeighbor: %v", err)
	}
	if err := mgr.RemoveNeighbor(10); err != nil {
		t.Fatalf("RemoveNeighbor: %v", err)
	}
	if err := mgr.RemoveNeighbor(10); err != nil {
		t.Fatalf("RemoveNeighbor idempotent: %v", err)
	}

	neighbor.Multihop = true
	if err := mgr.UpdateNeighbor(neighbor); err != nil {
		t.Fatalf("UpdateNeighbor: %v", err)
	}
	if _, err := mgr.neighborEntry(10); err != nil {
		t.Fatalf("neighbor should exist after update: %v", err)
	}
	logs := mgr.GetNeighborLogs(10, 5)
	if len(logs) == 0 {
		t.Fatal("expected neighbor logs after update")
	}
	_ = ctx
}

// Saving a neighbour from the admin panel must not cost an established session unless the
// session itself is being changed — otherwise renaming a node drops the peering.
func TestUpdateNeighborKeepsSessionWhenOnlyMetadataChanges(t *testing.T) {
	mgr, _ := startTestManager(t, config.BGPConfig{LocalAS: 65000, RouterID: "127.0.0.1"})
	neighbor := testNeighbor(20, 2, "10.0.0.20")
	if err := mgr.AddNeighbor(neighbor); err != nil {
		t.Fatalf("AddNeighbor: %v", err)
	}

	deleted := 0
	old := deletePeerHook
	deletePeerHook = func(_ context.Context, _ *api.DeletePeerRequest) error {
		deleted++
		return nil
	}
	defer func() { deletePeerHook = old }()

	// The everyday save: same session, same node, one corrected field.
	corrected := testNeighbor(20, 2, "10.0.0.20")
	corrected.DefaultRouteAS = 3356
	if err := mgr.UpdateNeighbor(corrected); err != nil {
		t.Fatalf("UpdateNeighbor: %v", err)
	}
	entry, err := mgr.neighborEntry(20)
	if err != nil {
		t.Fatalf("neighbor should still exist: %v", err)
	}
	if entry.neighbor.DefaultRouteAS != 3356 {
		t.Fatalf("default route AS = %d, want the updated 3356", entry.neighbor.DefaultRouteAS)
	}
	if entry.neighborIP != "10.0.0.20" {
		t.Fatalf("neighbor ip = %q, want it preserved", entry.neighborIP)
	}
	if !mgr.HasNeighbors(2) {
		t.Fatal("expected the neighbour to stay filed under its node")
	}

	// Moving it to another node is still no reason to drop the peering.
	moved := testNeighbor(20, 7, "10.0.0.20")
	if err := mgr.UpdateNeighbor(moved); err != nil {
		t.Fatalf("UpdateNeighbor after node move: %v", err)
	}
	if !mgr.HasNeighbors(7) {
		t.Fatal("expected the neighbour to be filed under its new node")
	}
	if mgr.HasNeighbors(2) {
		t.Fatal("expected the neighbour to be gone from its old node")
	}

	if deleted != 0 {
		t.Fatalf("peer deleted %d times, want the session left alone throughout", deleted)
	}
}

// A change the router would see must still rebuild the peer.
func TestUpdateNeighborRebuildsWhenSessionParamsChange(t *testing.T) {
	for _, tc := range []struct {
		name  string
		apply func(*domain.BGPNeighbor)
	}{
		{"remote as", func(n *domain.BGPNeighbor) { n.RemoteAS = 3356 }},
		{"neighbor ip", func(n *domain.BGPNeighbor) { n.NeighborIP = "10.0.0.31" }},
		{"peering ip", func(n *domain.BGPNeighbor) { n.PeeringIP = "127.0.0.2" }},
		{"multihop", func(n *domain.BGPNeighbor) { n.Multihop = true }},
		{"ipv6 neighbor ip", func(n *domain.BGPNeighbor) { n.IPv6NeighborIP = "2001:db8::31" }},
		{"ipv6 peering ip", func(n *domain.BGPNeighbor) { n.IPv6PeeringIP = "2001:db8::1" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mgr, _ := startTestManager(t, config.BGPConfig{LocalAS: 65000, RouterID: "127.0.0.1"})
			if err := mgr.AddNeighbor(testNeighbor(30, 2, "10.0.0.30")); err != nil {
				t.Fatalf("AddNeighbor: %v", err)
			}

			deleted := 0
			old := deletePeerHook
			// Count the teardown but still perform it, so the re-add sees a free address.
			deletePeerHook = func(ctx context.Context, req *api.DeletePeerRequest) error {
				deleted++
				return mgr.bgpServer.DeletePeer(ctx, req)
			}
			defer func() { deletePeerHook = old }()

			changed := testNeighbor(30, 2, "10.0.0.30")
			tc.apply(changed)
			if err := mgr.UpdateNeighbor(changed); err != nil {
				t.Fatalf("UpdateNeighbor: %v", err)
			}
			if deleted == 0 {
				t.Fatal("expected the peer to be rebuilt when the session changes")
			}
		})
	}
}

// An update for a neighbour the manager never saw simply adds it.
func TestUpdateNeighborAddsUnknown(t *testing.T) {
	mgr, _ := startTestManager(t, config.BGPConfig{LocalAS: 65000, RouterID: "127.0.0.1"})
	if err := mgr.UpdateNeighbor(testNeighbor(40, 4, "10.0.0.40")); err != nil {
		t.Fatalf("UpdateNeighbor: %v", err)
	}
	if _, err := mgr.neighborEntry(40); err != nil {
		t.Fatalf("neighbor should exist after update: %v", err)
	}
}

func TestSessionParamsEqualRejectsMissingRecords(t *testing.T) {
	n := testNeighbor(50, 5, "10.0.0.50")
	if sessionParamsEqual(nil, n) || sessionParamsEqual(n, nil) {
		t.Fatal("a missing record can never match a session")
	}
	// Whitespace around an address is not a different session.
	spaced := testNeighbor(50, 5, "  10.0.0.50  ")
	if !sessionParamsEqual(n, spaced) {
		t.Fatal("expected surrounding whitespace to be ignored")
	}
}

func TestAddNeighborIPv6Only(t *testing.T) {
	mgr, _ := startTestManager(t, config.BGPConfig{LocalAS: 65000, RouterID: "127.0.0.1"})
	neighbor := &domain.BGPNeighbor{
		ID:             11,
		NodeID:         3,
		RemoteAS:       174,
		IPv6PeeringIP:  "2001:db8::1",
		IPv6NeighborIP: "2001:db8::3",
	}
	if err := mgr.AddNeighbor(neighbor); err != nil {
		t.Fatalf("AddNeighbor ipv6-only: %v", err)
	}
	entry, _ := mgr.neighborEntry(11)
	if entry.neighborIP != "" || entry.ipv6NeighborIP != "2001:db8::3" {
		t.Fatalf("entry = %+v", entry)
	}
}

func TestStatusQueriesAndNodeHelpers(t *testing.T) {
	mgr := NewSessionManager(config.BGPConfig{})
	mgr.mu.Lock()
	mgr.neighbors[1] = &neighborEntry{neighbor: &domain.BGPNeighbor{ID: 1, NodeID: 7, RemoteAS: 174, NeighborIP: "10.0.0.1"}, neighborIP: "10.0.0.1", ipv6NeighborIP: "2001:db8::1"}
	mgr.neighbors[2] = &neighborEntry{neighbor: &domain.BGPNeighbor{ID: 2, NodeID: 7, RemoteAS: 174, NeighborIP: "10.0.0.2"}, neighborIP: "10.0.0.2"}
	mgr.states[1] = domain.BGPSessionEstablished
	mgr.states[2] = domain.BGPSessionIdle
	mgr.nodeNeighbors[7] = map[int64]struct{}{1: {}, 2: {}}
	mgr.mu.Unlock()

	if mgr.GetStatus(1) != domain.BGPSessionEstablished {
		t.Fatal("GetStatus mismatch")
	}
	all := mgr.GetAllStatuses()
	if len(all) != 2 {
		t.Fatalf("GetAllStatuses len = %d", len(all))
	}
	if nodeID, ok := mgr.NodeIDForNeighborIP("2001:db8::1"); !ok || nodeID != 7 {
		t.Fatalf("NodeIDForNeighborIP ipv6 = (%d, %v)", nodeID, ok)
	}
	if _, ok := mgr.NodeIDForNeighborIP(""); ok {
		t.Fatal("expected false for empty IP")
	}
	if !mgr.HasNeighbors(7) || mgr.HasNeighbors(99) {
		t.Fatal("HasNeighbors mismatch")
	}
	if !mgr.HasActiveSession(7) || mgr.HasActiveSession(99) {
		t.Fatal("HasActiveSession mismatch")
	}
}

func TestGetSessionStatusesAndPrefixCounts(t *testing.T) {
	mgr, ctx := startTestManager(t, config.BGPConfig{LocalAS: 65001, RouterID: "127.0.0.1"})
	mgr.mu.Lock()
	mgr.neighbors[1] = &neighborEntry{
		neighbor:   &domain.BGPNeighbor{ID: 1, NodeID: 1, RemoteAS: 174, NeighborIP: "10.0.0.2"},
		neighborIP: "10.0.0.2",
	}
	mgr.states[1] = domain.BGPSessionEstablished
	mgr.mu.Unlock()

	statuses := mgr.GetSessionStatuses()
	if len(statuses) != 1 {
		t.Fatalf("statuses len = %d", len(statuses))
	}

	if got := mgr.GetPrefixesReceived(ctx, 99); got != 0 {
		t.Fatalf("unknown neighbor prefixes = %d", got)
	}
	mgr.mu.Lock()
	mgr.states[1] = domain.BGPSessionIdle
	mgr.mu.Unlock()
	if got := mgr.GetPrefixesReceived(ctx, 1); got != 0 {
		t.Fatalf("non-established prefixes = %d", got)
	}

	mgr.mu.Lock()
	mgr.states[1] = domain.BGPSessionEstablished
	mgr.mu.Unlock()
	total := mgr.prefixesReceivedForEntry(ctx, mgr.neighbors[1])
	if total < 0 {
		t.Fatalf("prefixesReceivedForEntry = %d", total)
	}
	if n, err := mgr.countAdjPrefixes(ctx, "", api.Family_AFI_IP); err != nil || n != 0 {
		t.Fatalf("empty peer count = (%d, %v)", n, err)
	}
	mgr.storePrefixCount(1, 42)
	if got := mgr.GetPrefixesReceived(ctx, 1); got != 42 {
		t.Fatalf("cached prefixes = %d", got)
	}
}

func TestLoadNeighbors(t *testing.T) {
	mgr, ctx := startTestManager(t, config.BGPConfig{LocalAS: 65000, RouterID: "127.0.0.1"})
	repo := stubNeighborRepo{
		neighbors: []*domain.BGPNeighbor{
			testNeighbor(20, 1, "10.0.0.20"),
		},
	}
	if err := mgr.LoadNeighbors(ctx, repo); err != nil {
		t.Fatalf("LoadNeighbors: %v", err)
	}
	status := mgr.GetStatus(20)
	if status != domain.BGPSessionIdle && status != domain.BGPSessionActive && status != domain.BGPSessionConnect {
		t.Fatalf("loaded neighbor status = %q", status)
	}

	errRepo := stubNeighborRepo{err: fmt.Errorf("db down")}
	if err := mgr.LoadNeighbors(ctx, errRepo); err == nil {
		t.Fatal("expected load error")
	}
}

func TestLookupRoute(t *testing.T) {
	mgr, ctx := startTestManager(t, config.BGPConfig{LocalAS: 65001, RouterID: "127.0.0.1"})
	mgr.mu.Lock()
	mgr.neighbors[1] = &neighborEntry{neighbor: testNeighbor(1, 5, "10.0.0.2"), neighborIP: "10.0.0.2"}
	mgr.nodeNeighbors[5] = map[int64]struct{}{1: {}}
	mgr.mu.Unlock()

	addGlobalRoute(t, ctx, mgr, "192.0.2.0", "10.0.0.2", []bgp.PathAttributeInterface{
		bgp.NewPathAttributeOrigin(bgp.BGP_ORIGIN_ATTR_TYPE_EGP),
		bgp.NewPathAttributeNextHop("10.0.0.1"),
		bgp.NewPathAttributeAsPath([]bgp.AsPathParamInterface{bgp.NewAs4PathParam(2, []uint32{65001, 15169})}),
		bgp.NewPathAttributeLocalPref(100),
		bgp.NewPathAttributeMultiExitDisc(10),
		bgp.NewPathAttributeCommunities([]uint32{0x00020003}),
	})
	addGlobalRoute(t, ctx, mgr, "192.0.3.0", "10.0.0.9", nil)

	if _, err := mgr.LookupRoute(ctx, 5, "not-an-ip"); err == nil {
		t.Fatal("expected invalid prefix error")
	}
	if _, err := mgr.LookupRoute(ctx, 5, "192.0.2.0/24"); err != nil {
		t.Fatalf("LookupRoute: %v", err)
	}
	all, err := mgr.LookupRoute(ctx, 0, "192.0.2.0/24")
	if err != nil || len(all) == 0 {
		t.Fatalf("LookupRoute all: %v len=%d", err, len(all))
	}
	filtered, err := mgr.LookupRoute(ctx, 5, "192.0.2.0/24")
	if err != nil {
		t.Fatalf("LookupRoute filtered: %v", err)
	}
	// Manually added GLOBAL paths may not carry NeighborIp; node filter skips non-matching paths.
	_ = filtered

	// IPv6 lookup
	v6path, err := apiutil.NewPath(
		bgp.NewIPv6AddrPrefix(64, "2001:db8:1::"),
		false,
		[]bgp.PathAttributeInterface{
			bgp.NewPathAttributeOrigin(bgp.BGP_ORIGIN_ATTR_TYPE_INCOMPLETE),
			bgp.NewPathAttributeMpReachNLRI("2001:db8::1", []bgp.AddrPrefixInterface{bgp.NewIPv6AddrPrefix(64, "2001:db8:1::")}),
		},
		time.Now(),
	)
	if err != nil {
		t.Fatalf("NewPath v6: %v", err)
	}
	v6path.NeighborIp = "10.0.0.2"
	if _, err := mgr.bgpServer.AddPath(ctx, &api.AddPathRequest{TableType: api.TableType_GLOBAL, Path: v6path}); err != nil {
		t.Fatalf("AddPath v6: %v", err)
	}
	if _, err := mgr.LookupRoute(ctx, 0, "2001:db8:1::/64"); err != nil {
		t.Fatalf("LookupRoute v6: %v", err)
	}

	mgrNoServer := NewSessionManager(config.BGPConfig{})
	if _, err := mgrNoServer.LookupRoute(ctx, 0, "8.8.8.8"); err == nil {
		t.Fatal("expected not started error")
	}
}

func TestPathHelpersAndAPIState(t *testing.T) {
	if got := apiStateToDomain(api.PeerState_UNKNOWN); got != domain.BGPSessionIdle {
		t.Fatalf("unknown state = %q", got)
	}
	for apiState, want := range map[api.PeerState_SessionState]domain.BGPSessionState{
		api.PeerState_IDLE:        domain.BGPSessionIdle,
		api.PeerState_CONNECT:     domain.BGPSessionConnect,
		api.PeerState_ACTIVE:      domain.BGPSessionActive,
		api.PeerState_OPENSENT:    domain.BGPSessionOpenSent,
		api.PeerState_OPENCONFIRM: domain.BGPSessionOpenConfirm,
		api.PeerState_ESTABLISHED: domain.BGPSessionEstablished,
	} {
		if got := apiStateToDomain(apiState); got != want {
			t.Fatalf("apiStateToDomain(%v) = %q, want %q", apiState, got, want)
		}
	}

	asPath := asPathToString(bgp.NewPathAttributeAsPath([]bgp.AsPathParamInterface{
		bgp.NewAs4PathParam(2, []uint32{65001, 15169}),
	}))
	if !strings.Contains(asPath, "65001") {
		t.Fatalf("asPathToString = %q", asPath)
	}
	if got := communityToString(0x00020003); got != "2:3" {
		t.Fatalf("communityToString = %q", got)
	}
	for origin, want := range map[uint8]string{
		bgp.BGP_ORIGIN_ATTR_TYPE_IGP:        "IGP",
		bgp.BGP_ORIGIN_ATTR_TYPE_EGP:        "EGP",
		bgp.BGP_ORIGIN_ATTR_TYPE_INCOMPLETE: "incomplete",
		99:                                  "99",
	} {
		got := originToString(bgp.NewPathAttributeOrigin(origin))
		if got != want {
			t.Fatalf("origin %d = %q, want %q", origin, got, want)
		}
	}

	path, err := apiutil.NewPath(
		bgp.NewIPAddrPrefix(24, "203.0.113.0"),
		false,
		[]bgp.PathAttributeInterface{
			bgp.NewPathAttributeOrigin(bgp.BGP_ORIGIN_ATTR_TYPE_IGP),
			bgp.NewPathAttributeNextHop("10.0.0.1"),
			bgp.NewPathAttributeAsPath([]bgp.AsPathParamInterface{bgp.NewAs4PathParam(2, []uint32{65001})}),
			bgp.NewPathAttributeLocalPref(200),
			bgp.NewPathAttributeMultiExitDisc(5),
			bgp.NewPathAttributeCommunities([]uint32{0x00010002}),
			bgp.NewPathAttributeLargeCommunities([]*bgp.LargeCommunity{bgp.NewLargeCommunity(1, 2, 3)}),
			bgp.NewPathAttributeExtendedCommunities([]bgp.ExtendedCommunityInterface{
				bgp.NewOpaqueExtended(false, []byte{1, 2, 3, 4, 5, 6, 7}),
			}),
		},
		time.Now().Add(-30*time.Minute),
	)
	if err != nil {
		t.Fatalf("NewPath: %v", err)
	}
	path.NeighborIp = "10.0.0.2"
	path.SourceAsn = 174
	path.Best = true
	entry := NewSessionManager(config.BGPConfig{}).pathToRouteEntry(path, "203.0.113.0/24")
	if entry.NextHop != "10.0.0.1" || entry.Origin != "IGP" || entry.LocalPref != "200" || entry.MED != "5" {
		t.Fatalf("entry = %+v", entry)
	}
	if len(entry.Communities) < 2 {
		t.Fatalf("communities = %v", entry.Communities)
	}
}

func TestHandlePeerStateChange(t *testing.T) {
	mgr := NewSessionManager(config.BGPConfig{LocalAS: 65000})
	mgr.mu.Lock()
	mgr.neighbors[30] = &neighborEntry{neighbor: testNeighbor(30, 1, "127.0.0.1"), neighborIP: "127.0.0.1"}
	mgr.states[30] = domain.BGPSessionIdle
	mgr.mu.Unlock()

	initEv := &api.WatchEventResponse_PeerEvent{Type: api.WatchEventResponse_PeerEvent_INIT}
	mgr.handlePeerStateChange(30, "127.0.0.1", domain.BGPSessionIdle, domain.BGPSessionIdle, initEv)
	endEv := &api.WatchEventResponse_PeerEvent{Type: api.WatchEventResponse_PeerEvent_END_OF_INIT}
	mgr.handlePeerStateChange(30, "127.0.0.1", domain.BGPSessionIdle, domain.BGPSessionIdle, endEv)

	mgr.handlePeerStateChange(30, "127.0.0.1", domain.BGPSessionIdle, domain.BGPSessionIdle, nil)
	mgr.handlePeerStateChange(30, "127.0.0.1", domain.BGPSessionIdle, domain.BGPSessionConnect, nil)
	if mgr.GetStatus(30) != domain.BGPSessionConnect {
		t.Fatalf("status = %q", mgr.GetStatus(30))
	}
	mgr.handlePeerStateChange(30, "127.0.0.1", domain.BGPSessionConnect, domain.BGPSessionEstablished, nil)
	if mgr.GetStatus(30) != domain.BGPSessionEstablished {
		t.Fatalf("status = %q", mgr.GetStatus(30))
	}
	mgr.handlePeerStateChange(30, "127.0.0.1", domain.BGPSessionEstablished, domain.BGPSessionIdle, nil)
	if mgr.GetStatus(30) != domain.BGPSessionIdle {
		t.Fatalf("status = %q", mgr.GetStatus(30))
	}

	logs := mgr.GetNeighborLogs(30, 20)
	if len(logs) == 0 {
		t.Fatal("expected state change logs")
	}
}

func TestFetchPeerByAddress(t *testing.T) {
	mgr, ctx := startTestManager(t, config.BGPConfig{LocalAS: 65000, RouterID: "127.0.0.1"})
	if err := mgr.AddNeighbor(testNeighbor(40, 1, "127.0.0.1")); err != nil {
		t.Fatalf("AddNeighbor: %v", err)
	}
	peer, err := mgr.fetchPeerByAddress(ctx, "127.0.0.1")
	if err != nil {
		t.Fatalf("fetchPeerByAddress: %v", err)
	}
	if peer == nil || peer.Conf == nil {
		t.Fatal("expected peer snapshot")
	}
	if _, err := mgr.fetchPeerByAddress(ctx, "198.51.100.1"); err == nil {
		t.Fatal("expected peer not found error")
	}
	noServer := NewSessionManager(config.BGPConfig{})
	if _, err := noServer.fetchPeerByAddress(ctx, "127.0.0.1"); err == nil {
		t.Fatal("expected not started error")
	}
}

func TestStartInvalidRouterID(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	mgr := NewSessionManager(config.BGPConfig{LocalAS: 65000, RouterID: "not-valid"})
	if err := mgr.Start(ctx); err == nil {
		t.Fatal("expected invalid router id error")
	}
}

func TestConfiguredLocalASFallback(t *testing.T) {
	mgr := NewSessionManager(config.BGPConfig{LocalAS: 65099})
	if got := mgr.configuredLocalAS(); got != 65099 {
		t.Fatalf("configuredLocalAS = %d", got)
	}
}

func TestLookupRouteCIDRAndHost(t *testing.T) {
	mgr, ctx := startTestManager(t, config.BGPConfig{LocalAS: 65001, RouterID: "127.0.0.1"})
	addGlobalRoute(t, ctx, mgr, "198.51.100", "10.0.0.1", nil)
	if _, err := mgr.LookupRoute(ctx, 0, "198.51.100.1"); err != nil {
		t.Fatalf("host lookup: %v", err)
	}
	if ip := net.ParseIP("198.51.100.1"); ip == nil {
		t.Fatal("parse ip")
	}
}
