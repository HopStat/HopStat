package engine

import (
	"context"
	"errors"
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

func TestBGPMapNodesKeepsOnlySessionNodes(t *testing.T) {
	nodes := []*domain.Node{
		{ID: 1, Name: "BURSA", Type: domain.NodeTypeStandalone},
		nil,
		{ID: 2, Name: "SOFIA", Type: domain.NodeTypeLGNode},
		{ID: 3, Name: "IDLE", Type: domain.NodeTypeStandalone},
	}
	established := map[int64]bool{1: true, 2: true}

	got := bgpMapNodes(nodes, func(id int64) bool { return established[id] })

	if len(got) != 2 {
		t.Fatalf("nodes = %+v", got)
	}
	if got[0] != (bgp.MapNode{ID: 1, Name: "BURSA", Type: domain.NodeTypeStandalone}) {
		t.Fatalf("first = %+v", got[0])
	}
	if got[1].Type != domain.NodeTypeLGNode {
		t.Fatalf("node type lost: %+v", got[1])
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
