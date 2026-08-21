package engine

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/HopStat/HopStat/internal/bgp"
	"github.com/HopStat/HopStat/internal/config"
	"github.com/HopStat/HopStat/internal/domain"
)

// activeNodeRepo answers GetActive, which idNodeRepo stubs out as nil.
type activeNodeRepo struct {
	*idNodeRepo
	active    []*domain.Node
	activeErr error
}

func (r *activeNodeRepo) GetActive(context.Context) ([]*domain.Node, error) {
	return r.active, r.activeErr
}

func TestSplitMapNodesSortsBySource(t *testing.T) {
	e := New(&QueryConfig{MaxConcurrent: 4}, nil, nil, nil, nil, nil, 0)
	nodes := []*domain.Node{
		{ID: 1, Name: "BURSA", Type: domain.NodeTypeStandalone, EnabledCmds: []domain.CommandType{domain.CmdBGPRoute}},
		nil,
		{ID: 2, Name: "AGENT", Type: domain.NodeTypeLGNode, EnabledCmds: []domain.CommandType{domain.CmdBGPRoute}},
		{ID: 3, Name: "PING-ONLY", Type: domain.NodeTypeLGNode, EnabledCmds: []domain.CommandType{domain.CmdPing}},
	}

	// Without a BGP manager nothing can come from the RIB, so only BGP-capable agents remain.
	ribNodes, agentNodes := e.splitMapNodes(nodes)
	if len(ribNodes) != 0 {
		t.Fatalf("rib nodes = %+v", ribNodes)
	}
	if len(agentNodes) != 1 || agentNodes[0].Name != "AGENT" {
		t.Fatalf("agent nodes = %+v", agentNodes)
	}
}

func TestBuildNodeASPathMapRequiresReadyManagerAndRepo(t *testing.T) {
	repo := &activeNodeRepo{idNodeRepo: &idNodeRepo{}}
	e := New(&QueryConfig{MaxConcurrent: 4}, repo, nil, nil, nil, nil, 0)
	if got := e.buildNodeASPathMap(context.Background(), "8.8.8.8"); got != nil {
		t.Fatalf("nil manager returned %+v", got)
	}

	// Created but never started: IsReady is false, so no lookup should be attempted.
	e.bgpMgr = bgp.NewSessionManager(config.BGPConfig{})
	if got := e.buildNodeASPathMap(context.Background(), "8.8.8.8"); got != nil {
		t.Fatalf("unready manager returned %+v", got)
	}
}

func TestBuildNodeASPathMapWithoutNodeRepo(t *testing.T) {
	e := New(&QueryConfig{MaxConcurrent: 4}, nil, nil, nil, startedManager(t), nil, 0)
	if got := e.buildNodeASPathMap(context.Background(), "8.8.8.8"); got != nil {
		t.Fatalf("returned %+v", got)
	}
}

func TestBuildNodeASPathMapNodeListError(t *testing.T) {
	repo := &activeNodeRepo{idNodeRepo: &idNodeRepo{}, activeErr: errors.New("db down")}
	e := New(&QueryConfig{MaxConcurrent: 4}, repo, nil, nil, startedManager(t), nil, 0)
	if got := e.buildNodeASPathMap(context.Background(), "8.8.8.8"); got != nil {
		t.Fatalf("returned %+v", got)
	}
}

func TestBuildNodeASPathMapWithoutEstablishedSessions(t *testing.T) {
	repo := &activeNodeRepo{
		idNodeRepo: &idNodeRepo{},
		active: []*domain.Node{
			{ID: 1, Name: "BURSA", Type: domain.NodeTypeStandalone},
			{ID: 2, Name: "SOFIA", Type: domain.NodeTypeStandalone},
		},
	}
	e := New(&QueryConfig{MaxConcurrent: 4}, repo, nil, nil, startedManager(t), nil, 0)
	// No neighbor has reached Established, so there is nothing to compare.
	if got := e.buildNodeASPathMap(context.Background(), "8.8.8.8"); got != nil {
		t.Fatalf("returned %+v", got)
	}
}

func startedManager(t *testing.T) *bgp.SessionManager {
	t.Helper()
	mgr := bgp.NewSessionManager(config.BGPConfig{
		LocalAS: 65000, RouterID: "127.0.0.1", ListenPort: freeListenPort(t),
	})
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	if err := mgr.Start(ctx); err != nil {
		t.Fatalf("start bgp manager: %v", err)
	}
	t.Cleanup(mgr.Stop)
	return mgr
}

func TestCollectBGPASPathASNsIncludesNodeMap(t *testing.T) {
	result := &domain.QueryResult{
		ASPath: []uint32{9121, 3356},
		ASPathNodes: []domain.NodeASPath{
			{NodeID: 1, ASPath: []uint32{9121, 3356}},   // already seen, must not duplicate
			{NodeID: 2, ASPath: []uint32{9121, 6939, 0}}, // 0 is dropped
		},
	}
	br := &domain.BGPResult{Routes: []domain.BGPRoute{{ASPath: []uint32{15169}}}}

	got := collectBGPASPathASNs(result, br)

	want := []uint32{9121, 3356, 15169, 6939}
	if len(got) != len(want) {
		t.Fatalf("asns = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("asns = %v, want %v", got, want)
		}
	}
}

func TestEnrichASPathEmitsNodeMapWithoutRoutes(t *testing.T) {
	e := New(&QueryConfig{MaxConcurrent: 4}, nil, nil, nil, nil, nil, 0)
	result := &domain.QueryResult{
		ASPathNodes: []domain.NodeASPath{{NodeID: 1, NodeName: "BURSA", ASPath: []uint32{9121}}},
	}

	var partial *domain.QueryResult
	e.enrichASPath(context.Background(), &domain.BGPResult{}, result, ExecuteOption{
		OnPartial: func(p *domain.QueryResult) { partial = p },
	})

	if partial == nil || len(partial.ASPathNodes) != 1 {
		t.Fatalf("partial = %+v", partial)
	}

	// Without a node map there is nothing to send, so no partial is emitted.
	partial = nil
	e.enrichASPath(context.Background(), &domain.BGPResult{}, &domain.QueryResult{}, ExecuteOption{
		OnPartial: func(p *domain.QueryResult) { partial = p },
	})
	if partial != nil {
		t.Fatalf("unexpected partial: %+v", partial)
	}
}

func TestAgentNodeASPathsAsksRemoteAgents(t *testing.T) {
	agent := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(domain.BGPResult{Routes: []domain.BGPRoute{
			{Prefix: "8.8.8.0/24", ASPath: []uint32{64500, 15169}, Best: true},
			{Prefix: "8.8.8.0/24", ASPath: []uint32{64501, 6939, 15169}},
		}})
	}))
	defer agent.Close()

	e := New(&QueryConfig{MaxConcurrent: 4}, nil, nil, nil, nil, nil, 0)
	paths := e.agentNodeASPaths(context.Background(), []*domain.Node{lgNode(7, agent.URL)}, "8.8.8.8")

	if len(paths) != 2 {
		t.Fatalf("paths = %+v", paths)
	}
	if !paths[0].Best || paths[1].Best {
		t.Fatalf("expected one selected route and one backup: %+v", paths)
	}
	if paths[0].NodeID != 7 || paths[0].NodeName != "agent-node" {
		t.Fatalf("node identity lost: %+v", paths[0])
	}
	// The agent's path is used as-is; the local AS belongs to the embedded speaker only.
	if paths[0].ASPath[0] != 64500 {
		t.Fatalf("agent path was rewritten: %v", paths[0].ASPath)
	}
}

func TestAgentNodeASPathsDegradeToNoRoute(t *testing.T) {
	e := New(&QueryConfig{MaxConcurrent: 4}, nil, nil, nil, nil, nil, 0)

	if got := e.agentNodeASPaths(context.Background(), nil, "8.8.8.8"); got != nil {
		t.Fatalf("no nodes should mean no work: %+v", got)
	}

	// Unreachable agent: the map keeps the node instead of failing.
	dead := lgNode(8, "http://127.0.0.1:1")
	paths := e.agentNodeASPaths(context.Background(), []*domain.Node{dead}, "8.8.8.8")
	if len(paths) != 1 || !paths[0].NoRoute || paths[0].NodeID != 8 {
		t.Fatalf("paths = %+v", paths)
	}

	// A node the driver factory rejects behaves the same way.
	broken := lgNode(9, "")
	broken.Type = domain.NodeType("nonsense")
	paths = e.agentNodeASPaths(context.Background(), []*domain.Node{broken}, "8.8.8.8")
	if len(paths) != 1 || !paths[0].NoRoute {
		t.Fatalf("paths = %+v", paths)
	}
}

func TestBuildNodeASPathMapCombinesRIBAndAgents(t *testing.T) {
	agent := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(domain.BGPResult{Routes: []domain.BGPRoute{
			{Prefix: "8.8.8.0/24", ASPath: []uint32{64500, 15169}, Best: true},
		}})
	}))
	defer agent.Close()

	repo := &activeNodeRepo{
		idNodeRepo: &idNodeRepo{},
		active:     []*domain.Node{lgNode(1, agent.URL), lgNode(2, agent.URL)},
	}
	e := New(&QueryConfig{MaxConcurrent: 4}, repo, nil, nil, nil, nil, 0)

	// No BGP manager at all: the map still works from agents alone.
	paths := e.buildNodeASPathMap(context.Background(), "8.8.8.8")
	if len(paths) != 2 {
		t.Fatalf("paths = %+v", paths)
	}
	if paths[0].NodeID != 1 || paths[1].NodeID != 2 {
		t.Fatalf("node order = %d, %d", paths[0].NodeID, paths[1].NodeID)
	}
}

func TestBuildNodeASPathMapReadsSessionNodesFromRIB(t *testing.T) {
	repo := &activeNodeRepo{
		idNodeRepo: &idNodeRepo{},
		active: []*domain.Node{
			{ID: 1, Name: "BURSA", Type: domain.NodeTypeStandalone},
			{ID: 2, Name: "SOFIA", Type: domain.NodeTypeStandalone},
		},
	}
	e := New(&QueryConfig{MaxConcurrent: 4}, repo, nil, nil, startedManager(t), nil, 0)

	old := hasActiveBGPSession
	hasActiveBGPSession = func(*bgp.SessionManager, int64) bool { return true }
	defer func() { hasActiveBGPSession = old }()

	ribNodes, agentNodes := e.splitMapNodes(repo.active)
	if len(ribNodes) != 2 || len(agentNodes) != 0 {
		t.Fatalf("rib = %+v, agents = %+v", ribNodes, agentNodes)
	}

	// The RIB is empty, so both nodes report no route — but they stay on the map.
	paths := e.buildNodeASPathMap(context.Background(), "8.8.8.8")
	if len(paths) != 2 {
		t.Fatalf("paths = %+v", paths)
	}
	for _, p := range paths {
		if !p.NoRoute {
			t.Fatalf("unexpected route from an empty RIB: %+v", p)
		}
	}
}
