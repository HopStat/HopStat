package handler

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/HopStat/HopStat/internal/bgp"
	"github.com/HopStat/HopStat/internal/config"
	"github.com/HopStat/HopStat/internal/domain"
	"github.com/HopStat/HopStat/internal/store/repo"
)

func startedBGPManager(t *testing.T) *bgp.SessionManager {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	mgr := bgp.NewSessionManager(config.BGPConfig{
		LocalAS: 65000, RouterID: "127.0.0.1", ListenPort: freeListenPort(t),
	})
	if err := mgr.Start(ctx); err != nil {
		t.Fatalf("start bgp manager: %v", err)
	}
	t.Cleanup(mgr.Stop)
	return mgr
}

func TestListBGPNeighbors_NilManager(t *testing.T) {
	db := setupDB(t)
	c, w := setupAdminContext(db, http.MethodGet, "/admin/bgp-neighbors", "", 1)

	ListBGPNeighbors(db, nil, config.BGPConfig{})(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
}

func TestGetBGPConfig(t *testing.T) {
	c, w := setupAdminContext(nil, http.MethodGet, "/admin/bgp/config", "", 1)

	GetBGPConfig(config.BGPConfig{LocalAS: 64512}, nil)(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
}

func TestGetBGPNeighborStatuses_NilManager(t *testing.T) {
	c, w := setupAdminContext(nil, http.MethodGet, "/admin/bgp-neighbors/statuses", "", 1)

	GetBGPNeighborStatuses(nil)(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
}

func TestLookupBGPPaths_RequiresBGP(t *testing.T) {
	c, w := setupAdminContext(nil, http.MethodGet, "/admin/bgp/paths?prefix=8.8.8.0/24", "", 1)

	LookupBGPPaths(nil)(c)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
}

func TestLookupBGPPaths_ValidatesInput(t *testing.T) {
	mgr := startedBGPManager(t)

	tests := []struct {
		name  string
		query string
		want  int
	}{
		{name: "missing prefix", query: "", want: http.StatusBadRequest},
		{name: "blank prefix", query: "?prefix=%20%20", want: http.StatusBadRequest},
		{name: "bad node id", query: "?prefix=8.8.8.0/24&node_id=abc", want: http.StatusBadRequest},
		{name: "negative node id", query: "?prefix=8.8.8.0/24&node_id=-1", want: http.StatusBadRequest},
		{name: "unusable prefix", query: "?prefix=nonsense-target", want: http.StatusBadRequest},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, w := setupAdminContext(nil, http.MethodGet, "/admin/bgp/paths"+tt.query, "", 1)
			LookupBGPPaths(mgr)(c)
			if w.Code != tt.want {
				t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
			}
		})
	}
}

func TestLookupBGPPaths_ReturnsPaths(t *testing.T) {
	mgr := startedBGPManager(t)
	c, w := setupAdminContext(nil, http.MethodGet, "/admin/bgp/paths?prefix=8.8.8.0/24&node_id=0", "", 1)

	LookupBGPPaths(mgr)(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	// An empty table is a valid answer; the point is that it reports rather than fails.
	if !strings.Contains(w.Body.String(), `"data"`) {
		t.Fatalf("body = %s", w.Body.String())
	}
}

func TestBGPPathNodeNamer(t *testing.T) {
	db := setupDB(t)
	nodeRepo := repo.NewNodeRepo(db, "")
	node, err := nodeRepo.Create(context.Background(), &domain.Node{
		Name:        "ESENYURT",
		Type:        domain.NodeTypeStandalone,
		EnabledCmds: []domain.CommandType{domain.CmdBGPRoute},
		Active:      true,
	})
	if err != nil {
		t.Fatalf("create node: %v", err)
	}
	refreshTestSiteCache(t, db)

	mgr := startedBGPManager(t)
	if err := mgr.AddNeighbor(&domain.BGPNeighbor{
		ID: 1, NodeID: node.ID, LocalAS: 65000, RemoteAS: 174,
		PeeringIP: "127.0.0.1", NeighborIP: "10.9.9.9",
	}); err != nil {
		t.Fatalf("add neighbor: %v", err)
	}

	namer := bgpPathNodeNamer(mgr)

	if name := namer("10.9.9.9"); name != "ESENYURT" {
		t.Fatalf("name = %q", name)
	}
	// Second call comes from the cache.
	if name := namer("10.9.9.9"); name != "ESENYURT" {
		t.Fatalf("cached name = %q", name)
	}

	// A neighbour belonging to no node, and one whose node is not in the snapshot.
	if name := namer("203.0.113.9"); name != "" {
		t.Fatalf("unknown neighbour should have no node name, got %q", name)
	}
	if err := mgr.AddNeighbor(&domain.BGPNeighbor{
		ID: 2, NodeID: 9999, LocalAS: 65000, RemoteAS: 174,
		PeeringIP: "127.0.0.1", NeighborIP: "10.9.9.10",
	}); err != nil {
		t.Fatalf("add neighbor: %v", err)
	}
	if name := namer("10.9.9.10"); name != "" {
		t.Fatalf("missing node should have no name, got %q", name)
	}
}
