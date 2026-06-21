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
)

func TestEnsureBGPGlobalStartedZeroLocalASWithServer(t *testing.T) {
	mgr := NewSessionManager(config.BGPConfig{LocalAS: 65000})
	mgr.bgpServer = server.NewBgpServer()
	go mgr.bgpServer.Serve()
	defer mgr.bgpServer.Stop()

	if err := mgr.ensureBGPGlobalStarted(context.Background(), 0, "127.0.0.1"); err == nil {
		t.Fatal("expected error for localAS 0")
	}
}

func TestAddRemoveNeighborWithoutServer(t *testing.T) {
	mgr := NewSessionManager(config.BGPConfig{LocalAS: 65000})
	if err := mgr.AddNeighbor(testNeighbor(1, 1, "10.0.0.1")); err == nil {
		t.Fatal("expected AddNeighbor error without server")
	}
	if err := mgr.RemoveNeighbor(1); err == nil {
		t.Fatal("expected RemoveNeighbor error without server")
	}
}

func TestRestartNeighborUnknownID(t *testing.T) {
	mgr, _ := startTestManager(t, config.BGPConfig{LocalAS: 65000, RouterID: "127.0.0.1"})
	if err := mgr.RestartNeighbor(999); err == nil {
		t.Fatal("expected error for unknown neighbor")
	}
}

func TestRemoveNeighborIPv6UsesDeletePeerHook(t *testing.T) {
	mgr, _ := startTestManager(t, config.BGPConfig{LocalAS: 65000, RouterID: "127.0.0.1"})
	neighbor := &domain.BGPNeighbor{
		ID: 76, NodeID: 1, RemoteAS: 174, PeeringIP: "127.0.0.1", NeighborIP: "10.0.0.76",
		IPv6PeeringIP: "::1", IPv6NeighborIP: "2001:db8::76",
	}
	if err := mgr.AddNeighbor(neighbor); err != nil {
		t.Fatalf("AddNeighbor: %v", err)
	}
	var ipv6HookUsed bool
	old := deletePeerHook
	deletePeerHook = func(ctx context.Context, req *api.DeletePeerRequest) error {
		if req.Address == "2001:db8::76" {
			ipv6HookUsed = true
		}
		return mgr.bgpServer.DeletePeer(ctx, req)
	}
	defer func() { deletePeerHook = old }()
	if err := mgr.RemoveNeighbor(76); err != nil {
		t.Fatalf("RemoveNeighbor: %v", err)
	}
	if !ipv6HookUsed {
		t.Fatal("expected ipv6 deletePeerHook branch")
	}
}

func TestLookupRouteInvalidNormalizedPrefix(t *testing.T) {
	mgr, ctx := startTestManager(t, config.BGPConfig{LocalAS: 65001, RouterID: "127.0.0.1"})
	old := lookupNormalizeHook
	lookupNormalizeHook = func(context.Context, string) (string, error) {
		return "not-a-valid-ip", nil
	}
	defer func() { lookupNormalizeHook = old }()
	if _, err := mgr.LookupRoute(ctx, 0, "8.8.8.8"); err == nil || !strings.Contains(err.Error(), "invalid prefix") {
		t.Fatalf("LookupRoute err = %v", err)
	}
}

func TestBuildRouteResultSortEqualBestPrefixes(t *testing.T) {
	mgr, ctx := startTestManager(t, config.BGPConfig{LocalAS: 65001, RouterID: "127.0.0.1"})
	old := lookupListPathHook
	lookupListPathHook = func(_ context.Context, _ *api.ListPathRequest, fn func(*api.Destination)) error {
		var paths []*api.Path
		for _, nip := range []string{"10.0.0.1", "10.0.0.2", "10.0.0.3"} {
			path, err := apiutil.NewPath(
				bgp.NewIPAddrPrefix(24, "198.52.200.0"),
				false,
				[]bgp.PathAttributeInterface{
					bgp.NewPathAttributeOrigin(bgp.BGP_ORIGIN_ATTR_TYPE_IGP),
					bgp.NewPathAttributeNextHop(nip),
				},
				time.Now(),
			)
			if err != nil {
				t.Fatalf("NewPath: %v", err)
			}
			path.NeighborIp = nip
			paths = append(paths, path)
		}
		fn(&api.Destination{Prefix: "198.52.200.0/24", Paths: paths})
		return nil
	}
	defer func() { lookupListPathHook = old }()

	result, err := mgr.BuildRouteResult(ctx, 0, "198.52.200.0/24", nil)
	if err != nil {
		t.Fatalf("BuildRouteResult: %v", err)
	}
	if len(result.Routes) < 3 {
		t.Fatalf("expected 3 routes, got %d", len(result.Routes))
	}
}

func TestEnsureBestAmongRoutesSingleRoute(t *testing.T) {
	routes := []domain.BGPRoute{{Prefix: "203.0.113.0/24", Best: false}}
	EnsureBestAmongRoutes(routes)
	if !routes[0].Best {
		t.Fatal("expected single route marked best")
	}
}

func TestEnrichResultTargetASEmptyQueryIP(t *testing.T) {
	br := &domain.BGPResult{}
	old := queryTargetIPHook
	queryTargetIPHook = func(string) string { return "" }
	defer func() { queryTargetIPHook = old }()
	enrichResultTargetAS(context.Background(), stubTargetASResolver{
		info: &domain.ASInfo{ASN: 15169},
	}, br, "8.8.8.8")
	if br.TargetAS != nil {
		t.Fatalf("expected nil target AS, got %+v", br.TargetAS)
	}
}

func TestLookupNormalizeHookError(t *testing.T) {
	mgr, ctx := startTestManager(t, config.BGPConfig{LocalAS: 65001, RouterID: "127.0.0.1"})
	old := lookupNormalizeHook
	lookupNormalizeHook = func(context.Context, string) (string, error) {
		return "", errors.New("normalize failed")
	}
	defer func() { lookupNormalizeHook = old }()
	if _, err := mgr.LookupRoute(ctx, 0, "8.8.8.8"); err == nil {
		t.Fatal("expected normalize error")
	}
}
