package engine

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/HopStat/HopStat/internal/bgp"
	"github.com/HopStat/HopStat/internal/circuitbreaker"
	"github.com/HopStat/HopStat/internal/config"
	"github.com/HopStat/HopStat/internal/domain"
	"github.com/HopStat/HopStat/internal/geo"
)

type idNodeRepo struct {
	nodes map[int64]*domain.Node
	err   error
}

func (m *idNodeRepo) GetByID(_ context.Context, id int64) (*domain.Node, error) {
	if m.err != nil {
		return nil, m.err
	}
	if n, ok := m.nodes[id]; ok {
		return n, nil
	}
	return nil, domain.ErrNodeNotFound
}
func (m *idNodeRepo) GetActive(context.Context) ([]*domain.Node, error)   { return nil, nil }
func (m *idNodeRepo) GetAll(context.Context) ([]*domain.Node, error)      { return nil, nil }
func (m *idNodeRepo) Create(context.Context, *domain.Node) (*domain.Node, error) { return nil, nil }
func (m *idNodeRepo) Update(context.Context, *domain.Node) (*domain.Node, error) { return nil, nil }
func (m *idNodeRepo) SetDefault(context.Context, int64) error             { return nil }
func (m *idNodeRepo) Delete(context.Context, int64) error                   { return nil }
func (m *idNodeRepo) UpdateEnabledCmds(context.Context, int64, []domain.CommandType) error {
	return nil
}

type mockCommunityRepo struct {
	rules []*domain.CommunityRule
	err   error
}

func (m *mockCommunityRepo) GetAll(context.Context) ([]*domain.CommunityRule, error) { return nil, nil }
func (m *mockCommunityRepo) GetActive(context.Context) ([]*domain.CommunityRule, error) {
	return m.rules, m.err
}
func (m *mockCommunityRepo) GetActiveRulesForNode(_ context.Context, _ int64) ([]*domain.CommunityRule, error) {
	return m.rules, m.err
}
func (m *mockCommunityRepo) Create(context.Context, *domain.CommunityRule) (*domain.CommunityRule, error) {
	return nil, nil
}
func (m *mockCommunityRepo) Update(context.Context, *domain.CommunityRule) (*domain.CommunityRule, error) {
	return nil, nil
}
func (m *mockCommunityRepo) Delete(context.Context, int64) error { return nil }
func (m *mockCommunityRepo) Toggle(context.Context, int64) error { return nil }

func testGeoDB(t *testing.T) *geo.GeoIPDB {
	t.Helper()
	g := geo.New(
		filepath.Join("..", "geo", "testdata", "GeoLite2-ASN-Test.mmdb"),
		filepath.Join("..", "geo", "testdata", "GeoLite2-City-Test.mmdb"),
	)
	if !g.Enabled() {
		t.Fatal("expected test geo databases to load")
	}
	return g
}

func newAgentServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-token" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		switch r.URL.Path {
		case "/agent/v1/ping", "/agent/v1/ping/stream":
			if r.URL.Path == "/agent/v1/ping/stream" {
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = w.Write([]byte("event: result\ndata: {\"raw\":\"line-a\\nline-b\",\"packets_sent\":1,\"packets_recv\":1}\n\n"))
				return
			}
			_ = json.NewEncoder(w).Encode(domain.PingResult{
				Raw:          "PING 8.8.8.8\n1 packets transmitted, 1 received, 0% packet loss",
				PacketsSent:  1,
				PacketsRecv:  1,
				PacketLoss:   0,
				MinRTT:       1.0,
				AvgRTT:       1.0,
				MaxRTT:       1.0,
			})
		case "/agent/v1/traceroute":
			_ = json.NewEncoder(w).Encode(domain.TracerouteResult{
				Raw: "traceroute to 8.8.8.8\n 1  10.0.0.1 (10.0.0.1)  1.000 ms",
				Hops: []domain.Hop{{Number: 1, IP: "1.128.0.1", RTT: []float64{1.0}}},
			})
		case "/agent/v1/bgp/route":
			_ = json.NewEncoder(w).Encode(domain.BGPResult{
				Raw: "*> 8.8.8.8/32 10.0.0.1",
				Routes: []domain.BGPRoute{{
					Prefix:  "8.8.8.8/32",
					NextHop: "10.0.0.1",
					ASPath:  []uint32{15169},
					Best:    true,
				}},
				TargetAS: &domain.ASInfo{ASN: 15169, OrgName: "GOOGLE"},
			})
		default:
			http.NotFound(w, r)
		}
	}))
}

func lgNode(id int64, agentURL string) *domain.Node {
	return &domain.Node{
		ID:          id,
		Name:        "agent-node",
		Type:        domain.NodeTypeLGNode,
		AgentURL:    agentURL,
		AgentToken:  "test-token",
		EnabledCmds: domain.DefaultEnabledCmds(),
		Active:      true,
	}
}

func TestEmitRawLines(t *testing.T) {
	var lines []string
	emitRawLines("line1\r\n\r\nline2\n", func(s string) { lines = append(lines, s) })
	if len(lines) != 2 || lines[0] != "line1" || lines[1] != "line2" {
		t.Fatalf("lines = %v", lines)
	}
}

func TestDriverConfig(t *testing.T) {
	e := New(&QueryConfig{ServerPort: 8080}, &mockNodeRepo{}, nil, nil, nil, nil, 0)
	if cfg := e.driverConfig(); cfg == nil || cfg.Server.Port != 8080 {
		t.Fatalf("driverConfig = %+v", cfg)
	}
	e2 := New(&QueryConfig{ServerPort: 0}, &mockNodeRepo{}, nil, nil, nil, nil, 0)
	if e2.driverConfig() != nil {
		t.Fatal("expected nil driver config")
	}
}

func TestExecutePingTracerouteBGP(t *testing.T) {
	agent := newAgentServer(t)
	defer agent.Close()

	repo := &idNodeRepo{nodes: map[int64]*domain.Node{1: lgNode(1, agent.URL)}}
	g := testGeoDB(t)
	e := New(&QueryConfig{
		MaxConcurrent:        4,
		DefaultTimeoutSec:    5,
		TracerouteTimeoutSec: 5,
	}, repo, nil, g, nil, nil, 65000)

	tests := []struct {
		cmd domain.CommandType
	}{
		{domain.CmdPing},
		{domain.CmdTraceroute},
		{domain.CmdBGPRoute},
	}
	for _, tc := range tests {
		t.Run(string(tc.cmd), func(t *testing.T) {
			var partials int
			var streamed []string
			result, err := e.Execute(context.Background(), &domain.Query{
				ID:      "q",
				NodeID:  1,
				Command: tc.cmd,
				Target:  "8.8.8.8",
				Options: domain.QueryOptions{PingCount: 1, MaxHops: 5},
			}, ExecuteOption{
				OnLine: func(line string) { streamed = append(streamed, line) },
				OnPartial: func(*domain.QueryResult) { partials++ },
			})
			if err != nil {
				t.Fatalf("Execute error: %v", err)
			}
			if result.Status != domain.StatusDone {
				t.Fatalf("status = %s err=%s", result.Status, result.ErrorMsg)
			}
			if tc.cmd == domain.CmdBGPRoute && partials == 0 {
				t.Fatal("expected BGP partial callbacks")
			}
			if tc.cmd == domain.CmdBGPRoute && len(streamed) == 0 {
				t.Fatal("expected BGP raw lines streamed")
			}
		})
	}
}

func TestExecuteEarlyStop(t *testing.T) {
	agent := newAgentServer(t)
	defer agent.Close()
	repo := &idNodeRepo{nodes: map[int64]*domain.Node{1: lgNode(1, agent.URL)}}
	e := New(&QueryConfig{MaxConcurrent: 4, DefaultTimeoutSec: 5}, repo, nil, nil, nil, nil, 0)

	var stop atomic.Bool
	stop.Store(true)
	result, err := e.Execute(context.Background(), &domain.Query{
		ID: "q", NodeID: 1, Command: domain.CmdPing, Target: "8.8.8.8",
	}, ExecuteOption{ShouldStop: func() bool { return stop.Load() }})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if result.Status != domain.StatusDone {
		t.Fatalf("status = %s", result.Status)
	}
}

func TestExecuteDriverError(t *testing.T) {
	repo := &idNodeRepo{nodes: map[int64]*domain.Node{1: lgNode(1, "http://127.0.0.1:1")}}
	e := New(&QueryConfig{MaxConcurrent: 4, DefaultTimeoutSec: 1}, repo, nil, nil, nil, nil, 0)
	result, _ := e.Execute(context.Background(), &domain.Query{
		ID: "q", NodeID: 1, Command: domain.CmdPing, Target: "8.8.8.8",
	})
	if result.Status != domain.StatusError {
		t.Fatalf("status = %s", result.Status)
	}
}

func TestExecuteUnknownNodeType(t *testing.T) {
	repo := &idNodeRepo{nodes: map[int64]*domain.Node{1: {
		ID: 1, Type: "unknown", EnabledCmds: domain.DefaultEnabledCmds(),
	}}}
	e := New(&QueryConfig{MaxConcurrent: 4}, repo, nil, nil, nil, nil, 0)
	result, _ := e.Execute(context.Background(), &domain.Query{
		ID: "q", NodeID: 1, Command: domain.CmdPing, Target: "8.8.8.8",
	})
	if result.ErrorCode != "DRIVER_ERROR" {
		t.Fatalf("code = %s", result.ErrorCode)
	}
}

func TestEnrichHopsWithGeo(t *testing.T) {
	g := testGeoDB(t)
	e := New(&QueryConfig{MaxConcurrent: 4}, &mockNodeRepo{}, nil, g, nil, nil, 0)
	hops := []domain.Hop{{IP: "1.128.0.1"}, {IP: "not-an-ip"}}
	e.enrichHops(context.Background(), hops)
	if hops[0].ASInfo == nil || hops[0].ASInfo.ASN != 1221 {
		t.Fatalf("ASInfo = %+v", hops[0].ASInfo)
	}
}

func TestMatchCommunities(t *testing.T) {
	e := New(&QueryConfig{MaxConcurrent: 4}, &mockNodeRepo{}, &mockCommunityRepo{
		rules: []*domain.CommunityRule{
			{ID: 1, Community: "65000:100", Active: true},
			{ID: 2, Community: "65000:200", Active: true},
		},
	}, nil, nil, nil, 0)
	br := &domain.BGPResult{Routes: []domain.BGPRoute{{Communities: []string{"65000:100", "65000:200"}}}}
	result := &domain.QueryResult{}
	e.matchCommunities(context.Background(), 1, br, result)
	if len(result.MatchedRules) != 2 {
		t.Fatalf("matched = %d", len(result.MatchedRules))
	}
	if len(br.Routes[0].MatchedRules) != 2 {
		t.Fatalf("route matched = %d", len(br.Routes[0].MatchedRules))
	}
}

func TestMatchCommunitiesNilRepo(t *testing.T) {
	e := New(&QueryConfig{MaxConcurrent: 4}, &mockNodeRepo{}, nil, nil, nil, nil, 0)
	result := &domain.QueryResult{}
	e.matchCommunities(context.Background(), 1, &domain.BGPResult{}, result)
}

func TestNodeNameHelpers(t *testing.T) {
	repo := &idNodeRepo{nodes: map[int64]*domain.Node{
		2: {ID: 2, Name: "peer-node"},
	}}
	e := New(&QueryConfig{MaxConcurrent: 4}, repo, nil, nil, nil, nil, 0)
	if e.nodeName(context.Background(), 0) != "" {
		t.Fatal("expected empty for zero id")
	}
	if e.nodeName(context.Background(), 2) != "peer-node" {
		t.Fatal("unexpected node name")
	}

	mgr := bgp.NewSessionManager(config.BGPConfig{LocalAS: 65000, RouterID: "127.0.0.1", ListenPort: freeListenPort(t)})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := mgr.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer mgr.Stop()
	_ = mgr.AddNeighbor(&domain.BGPNeighbor{
		ID: 1, NodeID: 2, LocalAS: 65000, RemoteAS: 174,
		PeeringIP: "127.0.0.1", NeighborIP: "10.4.4.3",
	})
	e.bgpMgr = mgr
	resolver := e.nodeNameForNeighborIP(context.Background())
	if resolver("10.4.4.3") != "peer-node" {
		t.Fatalf("neighbor name = %q", resolver("10.4.4.3"))
	}
	if resolver("unknown") != "" {
		t.Fatal("expected empty for unknown neighbor")
	}
}

func TestAttachRouteNodeNames(t *testing.T) {
	repo := &idNodeRepo{nodes: map[int64]*domain.Node{1: {ID: 1, Name: "query-node"}}}
	e := New(&QueryConfig{MaxConcurrent: 4}, repo, nil, nil, nil, nil, 0)
	br := &domain.BGPResult{Routes: []domain.BGPRoute{{Prefix: "1.1.1.0/24"}}}
	e.attachRouteNodeNames(context.Background(), br, 1)
	if br.Routes[0].NodeName != "query-node" {
		t.Fatalf("node name = %q", br.Routes[0].NodeName)
	}
	e.attachRouteNodeNames(context.Background(), nil, 1)
	e.attachRouteNodeNames(context.Background(), &domain.BGPResult{}, 1)
}

func TestLocalAS(t *testing.T) {
	mgr := bgp.NewSessionManager(config.BGPConfig{LocalAS: 65001, RouterID: "127.0.0.1", ListenPort: freeListenPort(t)})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := mgr.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer mgr.Stop()
	e := New(&QueryConfig{MaxConcurrent: 4}, &mockNodeRepo{}, nil, nil, mgr, nil, 9121)
	if e.localAS() != 65001 {
		t.Fatalf("localAS = %d", e.localAS())
	}
	e.bgpMgr = nil
	if e.localAS() != 9121 {
		t.Fatalf("fallback localAS = %d", e.localAS())
	}
}

func TestApplyBGPASPathEnriched(t *testing.T) {
	g := testGeoDB(t)
	e := New(&QueryConfig{MaxConcurrent: 4}, &mockNodeRepo{}, nil, g, nil, nil, 0)
	br := &domain.BGPResult{
		Routes: []domain.BGPRoute{{
			Prefix: "1.128.0.0/24",
			ASPath: []uint32{1221, 15169},
			Best:   true,
		}},
		TargetAS: &domain.ASInfo{ASN: 15169, OrgName: "GOOGLE", CountryCode: "US"},
	}
	result := &domain.QueryResult{}
	var partials int
	e.applyBGPASPath(context.Background(), br, "1.128.0.1", result, ExecuteOption{
		OnPartial: func(*domain.QueryResult) { partials++ },
	})
	if len(result.ASPath) != 2 || len(result.ASPathEnriched) == 0 {
		t.Fatalf("ASPath=%v enriched=%v", result.ASPath, result.ASPathEnriched)
	}
	if partials == 0 {
		t.Fatal("expected partial emissions")
	}
}

func TestEnsureTargetASInResult(t *testing.T) {
	e := New(&QueryConfig{MaxConcurrent: 4}, &mockNodeRepo{}, nil, nil, nil, nil, 0)
	br := &domain.BGPResult{TargetAS: &domain.ASInfo{ASN: 15169, OrgName: "GOOGLE"}}
	result := &domain.QueryResult{ASPath: []uint32{13335}}
	var partials int
	e.ensureTargetASInResult(br, result, ExecuteOption{OnPartial: func(*domain.QueryResult) { partials++ }})
	if len(result.ASPathEnriched) != 1 || result.ASPathEnriched[0].ASN != 15169 {
		t.Fatalf("enriched = %v", result.ASPathEnriched)
	}
	if partials == 0 {
		t.Fatal("expected partial")
	}
	e.ensureTargetASInResult(br, result, ExecuteOption{})
}

func TestEnrichASPathEmptyPath(t *testing.T) {
	e := New(&QueryConfig{MaxConcurrent: 4}, &mockNodeRepo{}, nil, testGeoDB(t), nil, nil, 0)
	br := &domain.BGPResult{Routes: []domain.BGPRoute{{Prefix: "1.1.1.0/24", ASPath: []uint32{15169}, Best: true}}}
	result := &domain.QueryResult{}
	e.enrichASPath(context.Background(), br, result, ExecuteOption{})
	if len(result.ASPath) != 1 {
		t.Fatalf("ASPath = %v", result.ASPath)
	}
}

func TestLookupBGPRoutesViaManager(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	mgr := bgp.NewSessionManager(config.BGPConfig{LocalAS: 65000, RouterID: "127.0.0.1", ListenPort: freeListenPort(t)})
	if err := mgr.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer mgr.Stop()
	if err := mgr.AddNeighbor(&domain.BGPNeighbor{
		ID: 1, NodeID: 1, LocalAS: 65000, RemoteAS: 174,
		PeeringIP: "127.0.0.1", NeighborIP: "10.4.4.3", DefaultRouteAS: 174,
	}); err != nil {
		t.Fatal(err)
	}

	repo := &idNodeRepo{nodes: map[int64]*domain.Node{1: {ID: 1, Name: "node-1"}}}
	e := New(&QueryConfig{MaxConcurrent: 4}, repo, nil, nil, mgr, nil, 65000)
	got, err := e.lookupBGPRoutes(ctx, stubBGPDriver{}, "8.8.8.8", 1, domain.NodeTypeStandalone)
	if err != nil {
		t.Fatalf("lookupBGPRoutes: %v", err)
	}
	if len(got.Routes) == 0 {
		t.Fatal("expected synthesized default routes")
	}
}

func TestLookupBGPRoutesDriverError(t *testing.T) {
	e := New(&QueryConfig{MaxConcurrent: 4}, &mockNodeRepo{}, nil, nil, nil, nil, 0)
	drv := stubBGPDriver{result: &domain.BGPResult{}}
	drvErr := errors.New("bgp failed")
	badDrv := errorBGPDriver{err: drvErr}
	_, err := e.lookupBGPRoutes(context.Background(), badDrv, "8.8.8.8", 1, domain.NodeTypeStandalone)
	if err != drvErr {
		t.Fatalf("err = %v", err)
	}
	_ = drv
}

type errorBGPDriver struct {
	err error
}

func (d errorBGPDriver) Capabilities() []domain.CommandType { return nil }
func (d errorBGPDriver) TestConnection(context.Context) error { return nil }
func (d errorBGPDriver) Ping(context.Context, string, int) (*domain.PingResult, error) {
	return nil, nil
}
func (d errorBGPDriver) Traceroute(context.Context, string, int) (*domain.TracerouteResult, error) {
	return nil, nil
}
func (d errorBGPDriver) BGPRoute(context.Context, string) (*domain.BGPResult, error) {
	return nil, d.err
}

func TestSanitizeAndClassifyAllErrors(t *testing.T) {
	cases := []struct {
		err      error
		code     string
		sanitize string
	}{
		{domain.ErrNodeNotFound, "NODE_NOT_FOUND", "node not found"},
		{domain.ErrCommandDisabled, "COMMAND_DISABLED", "command not enabled for this node"},
		{domain.ErrDNSNotFound, "DNS_NOT_FOUND", "dns resolution failed"},
		{domain.ErrInvalidTarget, "INVALID_TARGET", "invalid target"},
		{domain.ErrTimeout, "COMMAND_TIMEOUT", "command timed out"},
		{domain.ErrQueryPoolFull, "POOL_FULL", "server is busy, try again later"},
		{domain.ErrRateLimited, "RATE_LIMITED", "rate limit exceeded"},
		{circuitbreaker.ErrCircuitOpen, "NODE_UNAVAILABLE", "command execution failed"},
		{context.Canceled, "COMMAND_TIMEOUT", "command timed out"},
		{context.DeadlineExceeded, "COMMAND_TIMEOUT", "command timed out"},
		{errors.New("other"), "INTERNAL_ERROR", "command execution failed"},
	}
	for _, tc := range cases {
		if got := ClassifyError(tc.err); got != tc.code {
			t.Errorf("%v: code = %q want %q", tc.err, got, tc.code)
		}
		if got := SanitizeErrorMsg(tc.err); got != tc.sanitize {
			t.Errorf("%v: sanitize = %q want %q", tc.err, got, tc.sanitize)
		}
	}
}

func TestPrefetchBGPASPath(t *testing.T) {
	agent := newAgentServer(t)
	defer agent.Close()
	repo := &idNodeRepo{nodes: map[int64]*domain.Node{1: lgNode(1, agent.URL)}}
	e := New(&QueryConfig{MaxConcurrent: 4, DefaultTimeoutSec: 5}, repo, nil, testGeoDB(t), nil, nil, 65000)

	done := make(chan struct{})
	var partials int
	_, _ = e.Execute(context.Background(), &domain.Query{
		ID: "q", NodeID: 1, Command: domain.CmdPing, Target: "8.8.8.8",
		Options: domain.QueryOptions{PingCount: 1},
	}, ExecuteOption{OnPartial: func(*domain.QueryResult) { partials++ }})
	close(done)
	time.Sleep(50 * time.Millisecond)
}

func TestExecuteEmitsRawWhenNotStreamed(t *testing.T) {
	agent := newAgentServer(t)
	defer agent.Close()
	repo := &idNodeRepo{nodes: map[int64]*domain.Node{1: lgNode(1, agent.URL)}}
	e := New(&QueryConfig{MaxConcurrent: 4, DefaultTimeoutSec: 5}, repo, nil, nil, nil, nil, 0)
	var lines []string
	result, err := e.Execute(context.Background(), &domain.Query{
		ID: "q", NodeID: 1, Command: domain.CmdPing, Target: "8.8.8.8",
	}, ExecuteOption{OnLine: func(line string) { lines = append(lines, line) }})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if result.Status != domain.StatusDone {
		t.Fatalf("status = %s err=%s", result.Status, result.ErrorMsg)
	}
	if len(lines) < 2 {
		t.Fatalf("lines = %v", lines)
	}
}

func TestEnrichASPathEmptyRoutes(t *testing.T) {
	e := New(&QueryConfig{MaxConcurrent: 4}, &mockNodeRepo{}, nil, testGeoDB(t), nil, nil, 0)
	result := &domain.QueryResult{ASPath: []uint32{1221}}
	e.enrichASPath(context.Background(), &domain.BGPResult{}, result, ExecuteOption{})
}

func TestEnrichASPathSetsFlagEmoji(t *testing.T) {
	e := New(&QueryConfig{MaxConcurrent: 4}, &mockNodeRepo{}, nil, testGeoDB(t), nil, nil, 0)
	br := &domain.BGPResult{Routes: []domain.BGPRoute{{ASPath: []uint32{1221}, Best: true, Prefix: "1.128.0.0/24"}}}
	result := &domain.QueryResult{ASPath: []uint32{1221}}
	e.enrichASPath(context.Background(), br, result, ExecuteOption{})
	if len(result.ASPathEnriched) == 0 {
		t.Fatal("expected enrichment")
	}
}

func TestExecutePingStreamedSkipsRawReplay(t *testing.T) {
	agent := newAgentServer(t)
	defer agent.Close()
	repo := &idNodeRepo{nodes: map[int64]*domain.Node{1: lgNode(1, agent.URL)}}
	e := New(&QueryConfig{MaxConcurrent: 4, DefaultTimeoutSec: 5}, repo, nil, nil, nil, nil, 0)
	var lines []string
	_, _ = e.Execute(context.Background(), &domain.Query{
		ID: "q", NodeID: 1, Command: domain.CmdPing, Target: "8.8.8.8",
	}, ExecuteOption{OnLine: func(line string) { lines = append(lines, line) }})
	if len(lines) == 0 {
		t.Fatal("expected streamed lines from SSE ping")
	}
}

func TestExecuteWithoutOptions(t *testing.T) {
	agent := newAgentServer(t)
	defer agent.Close()
	repo := &idNodeRepo{nodes: map[int64]*domain.Node{1: lgNode(1, agent.URL)}}
	e := New(&QueryConfig{MaxConcurrent: 4, DefaultTimeoutSec: 5, TracerouteTimeoutSec: 3}, repo, nil, nil, nil, nil, 0)
	result, err := e.Execute(context.Background(), &domain.Query{
		ID: "q", NodeID: 1, Command: domain.CmdTraceroute, Target: "8.8.8.8",
		Options: domain.QueryOptions{MaxHops: 5},
	})
	if err != nil || result.Status != domain.StatusDone {
		t.Fatalf("result=%+v err=%v", result, err)
	}
}

func TestPrefetchBGPASPathLookupError(t *testing.T) {
	e := New(&QueryConfig{MaxConcurrent: 4}, &mockNodeRepo{}, nil, nil, nil, nil, 0)
	e.prefetchBGPASPath(context.Background(), errorBGPDriver{err: errors.New("fail")}, "8.8.8.8", 1, domain.NodeTypeStandalone, ExecuteOption{})
}

func TestExecuteDriverErrorsInSwitch(t *testing.T) {
	agent := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-token" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		http.Error(w, "fail", http.StatusInternalServerError)
	}))
	defer agent.Close()
	repo := &idNodeRepo{nodes: map[int64]*domain.Node{1: lgNode(1, agent.URL)}}
	e := New(&QueryConfig{MaxConcurrent: 4, DefaultTimeoutSec: 5}, repo, nil, nil, nil, nil, 0)

	for _, cmd := range []domain.CommandType{domain.CmdPing, domain.CmdTraceroute, domain.CmdBGPRoute} {
		t.Run(string(cmd), func(t *testing.T) {
			result, _ := e.Execute(context.Background(), &domain.Query{
				ID: "q", NodeID: 1, Command: cmd, Target: "8.8.8.8",
				Options: domain.QueryOptions{PingCount: 1, MaxHops: 5},
			})
			if result.Status != domain.StatusError {
				t.Fatalf("status = %s", result.Status)
			}
		})
	}
}

func TestExecuteShouldStopContextDone(t *testing.T) {
	agent := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(500 * time.Millisecond)
		_ = json.NewEncoder(w).Encode(domain.PingResult{PacketsSent: 1, PacketsRecv: 1})
	}))
	defer agent.Close()
	repo := &idNodeRepo{nodes: map[int64]*domain.Node{1: lgNode(1, agent.URL)}}
	e := New(&QueryConfig{MaxConcurrent: 4, DefaultTimeoutSec: 5}, repo, nil, nil, nil, nil, 0)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	var stop atomic.Bool
	result, _ := e.Execute(ctx, &domain.Query{
		ID: "q", NodeID: 1, Command: domain.CmdPing, Target: "8.8.8.8",
	}, ExecuteOption{ShouldStop: func() bool { return stop.Load() }})
	if result.Status != domain.StatusError {
		t.Fatalf("status = %s", result.Status)
	}
}

func TestEnrichASPathSetsFlagFromCountry(t *testing.T) {
	dir := t.TempDir()
	blocksPath := filepath.Join(dir, "GeoLite2-ASN-Blocks-IPv4.csv")
	if err := os.WriteFile(blocksPath, []byte(`network,geoname_id,registered_country_geoname_id,represented_country_geoname_id,is_anonymous_proxy,is_satellite_provider,autonomous_system_number,autonomous_system_organization
0.0.0.0/0,,6252001,,0,0,1221,Telstra
`), 0644); err != nil {
		t.Fatal(err)
	}
	locPath := filepath.Join(dir, "GeoLite2-City-Locations-en.csv")
	if err := os.WriteFile(locPath, []byte(`geoname_id,locale_code,country_iso_code
6252001,en,AU
`), 0644); err != nil {
		t.Fatal(err)
	}
	g := geo.New("", "")
	asnPath := filepath.Join(dir, "GeoLite2-ASN.mmdb")
	if err := os.WriteFile(asnPath, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	g.SetPaths(asnPath, "")
	_ = g.Reload()
	e := New(&QueryConfig{MaxConcurrent: 4}, &mockNodeRepo{}, nil, g, nil, nil, 0)
	br := &domain.BGPResult{Routes: []domain.BGPRoute{{ASPath: []uint32{1221}, Best: true}}}
	result := &domain.QueryResult{ASPath: []uint32{1221}}
	e.enrichASPath(context.Background(), br, result, ExecuteOption{})
	for _, info := range result.ASPathEnriched {
		if info.ASN == 1221 && info.CountryCode != "" && info.FlagEmoji == "" {
			t.Fatalf("expected flag emoji for country, got %+v", info)
		}
	}
}

func TestNewWithRateLimit(t *testing.T) {
	e := New(&QueryConfig{
		MaxConcurrent:        4,
		FloodControlEnabled:  true,
		QueryRateLimitPerMin: 10,
	}, &mockNodeRepo{}, nil, nil, nil, nil, 0)
	if e.rateLimit == nil {
		t.Fatal("expected rate limiter")
	}
}

func TestMatchCommunitiesRepoError(t *testing.T) {
	e := New(&QueryConfig{MaxConcurrent: 4}, &mockNodeRepo{}, &mockCommunityRepo{err: errors.New("db")}, nil, nil, nil, 0)
	result := &domain.QueryResult{}
	e.matchCommunities(context.Background(), 1, &domain.BGPResult{Routes: []domain.BGPRoute{{}}}, result)
	if len(result.MatchedRules) != 0 {
		t.Fatal("expected no matches on repo error")
	}
}

func TestMatchCommunitiesDedupesRules(t *testing.T) {
	rule := &domain.CommunityRule{ID: 1, Community: "65000:100", Active: true}
	e := New(&QueryConfig{MaxConcurrent: 4}, &mockNodeRepo{}, &mockCommunityRepo{rules: []*domain.CommunityRule{rule, rule}}, nil, nil, nil, 0)
	br := &domain.BGPResult{Routes: []domain.BGPRoute{
		{Communities: []string{"65000:100"}},
		{Communities: []string{"65000:100"}},
	}}
	result := &domain.QueryResult{}
	e.matchCommunities(context.Background(), 1, br, result)
	if len(result.MatchedRules) != 1 {
		t.Fatalf("matched = %d", len(result.MatchedRules))
	}
}

func TestNodeNameRepoError(t *testing.T) {
	e := New(&QueryConfig{MaxConcurrent: 4}, &idNodeRepo{err: domain.ErrNodeNotFound}, nil, nil, nil, nil, 0)
	if e.nodeName(context.Background(), 1) != "" {
		t.Fatal("expected empty on repo error")
	}
}

func TestNodeNameForNeighborIPNilManager(t *testing.T) {
	e := New(&QueryConfig{MaxConcurrent: 4}, &mockNodeRepo{}, nil, nil, nil, nil, 0)
	resolver := e.nodeNameForNeighborIP(context.Background())
	if resolver("10.0.0.1") != "" {
		t.Fatal("expected empty without bgp manager")
	}
}

func TestApplyBGPASPathNoRoutesWithTargetAS(t *testing.T) {
	e := New(&QueryConfig{MaxConcurrent: 4}, &mockNodeRepo{}, nil, nil, nil, nil, 0)
	br := &domain.BGPResult{
		Raw:      "no matching BGP routes for 185.203.171.1",
		TargetAS: &domain.ASInfo{ASN: 9121, OrgName: "TURKNET"},
	}
	result := &domain.QueryResult{}
	var partials int
	e.applyBGPASPath(context.Background(), br, "185.203.171.1", result, ExecuteOption{
		OnPartial: func(*domain.QueryResult) { partials++ },
	})
	if len(result.ASPathEnriched) != 0 {
		t.Fatalf("ASPathEnriched = %v", result.ASPathEnriched)
	}
	if partials != 0 {
		t.Fatal("expected no partial AS path when BGP route is missing")
	}
}

func TestApplyBGPASPathNilRoute(t *testing.T) {
	e := New(&QueryConfig{MaxConcurrent: 4}, &mockNodeRepo{}, nil, nil, nil, nil, 0)
	result := &domain.QueryResult{}
	e.applyBGPASPath(context.Background(), &domain.BGPResult{}, "8.8.8.8", result, ExecuteOption{})
	if len(result.ASPath) != 0 {
		t.Fatalf("ASPath = %v", result.ASPath)
	}
}

func TestEnrichASPathLookupMiss(t *testing.T) {
	g := testGeoDB(t)
	e := New(&QueryConfig{MaxConcurrent: 4}, &mockNodeRepo{}, nil, g, nil, nil, 0)
	br := &domain.BGPResult{Routes: []domain.BGPRoute{{Prefix: "1.0.0.0/24", ASPath: []uint32{999999}, Best: true}}}
	result := &domain.QueryResult{ASPath: []uint32{999999}}
	e.enrichASPath(context.Background(), br, result, ExecuteOption{})
	if len(result.ASPathEnriched) != 1 || result.ASPathEnriched[0].ASN != 999999 {
		t.Fatalf("enriched = %+v", result.ASPathEnriched)
	}
}

func TestLookupBGPRoutesManagerError(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	mgr := bgp.NewSessionManager(config.BGPConfig{LocalAS: 65000, RouterID: "127.0.0.1", ListenPort: freeListenPort(t)})
	if err := mgr.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer mgr.Stop()
	_ = mgr.AddNeighbor(&domain.BGPNeighbor{
		ID: 1, NodeID: 1, LocalAS: 65000, RemoteAS: 174,
		PeeringIP: "127.0.0.1", NeighborIP: "10.4.4.3",
	})
	e := New(&QueryConfig{MaxConcurrent: 4}, &mockNodeRepo{}, nil, nil, mgr, nil, 65000)
	_, err := e.lookupBGPRoutes(ctx, stubBGPDriver{}, "bad|target", 1, domain.NodeTypeStandalone)
	if err == nil {
		t.Fatal("expected invalid target error from bgp manager path")
	}
}

func TestNodeNameForNeighborIPCache(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	mgr := bgp.NewSessionManager(config.BGPConfig{LocalAS: 65000, RouterID: "127.0.0.1", ListenPort: freeListenPort(t)})
	if err := mgr.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer mgr.Stop()
	_ = mgr.AddNeighbor(&domain.BGPNeighbor{
		ID: 1, NodeID: 2, LocalAS: 65000, RemoteAS: 174,
		PeeringIP: "127.0.0.1", NeighborIP: "10.4.4.3",
	})
	repo := &idNodeRepo{nodes: map[int64]*domain.Node{2: {ID: 2, Name: "cached-node"}}}
	e := New(&QueryConfig{MaxConcurrent: 4}, repo, nil, nil, mgr, nil, 0)
	resolver := e.nodeNameForNeighborIP(context.Background())
	if resolver("10.4.4.3") != "cached-node" || resolver("10.4.4.3") != "cached-node" {
		t.Fatal("expected cached neighbor name")
	}
}

func TestEnrichASPathSkipsDuplicateASN(t *testing.T) {
	e := New(&QueryConfig{MaxConcurrent: 4}, &mockNodeRepo{}, nil, testGeoDB(t), nil, nil, 0)
	br := &domain.BGPResult{Routes: []domain.BGPRoute{{ASPath: []uint32{1221, 1221}, Best: true}}}
	result := &domain.QueryResult{ASPath: []uint32{1221, 1221}}
	e.enrichASPath(context.Background(), br, result, ExecuteOption{})
	if len(result.ASPathEnriched) != 1 {
		t.Fatalf("enriched = %+v", result.ASPathEnriched)
	}
}

func TestExecuteContextCancel(t *testing.T) {
	agent := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(500 * time.Millisecond)
	}))
	defer agent.Close()
	repo := &idNodeRepo{nodes: map[int64]*domain.Node{1: lgNode(1, agent.URL)}}
	e := New(&QueryConfig{MaxConcurrent: 4, DefaultTimeoutSec: 1}, repo, nil, nil, nil, nil, 0)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	result, _ := e.Execute(ctx, &domain.Query{ID: "q", NodeID: 1, Command: domain.CmdPing, Target: "8.8.8.8"})
	if result.Status != domain.StatusError {
		t.Fatalf("status = %s", result.Status)
	}
}

func TestExecuteShouldStopDuringRun(t *testing.T) {
	agent := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		_ = json.NewEncoder(w).Encode(domain.PingResult{PacketsSent: 1, PacketsRecv: 1})
	}))
	defer agent.Close()
	repo := &idNodeRepo{nodes: map[int64]*domain.Node{1: lgNode(1, agent.URL)}}
	e := New(&QueryConfig{MaxConcurrent: 4, DefaultTimeoutSec: 5}, repo, nil, nil, nil, nil, 0)
	var stop atomic.Bool
	go func() {
		time.Sleep(50 * time.Millisecond)
		stop.Store(true)
	}()
	result, _ := e.Execute(context.Background(), &domain.Query{
		ID: "q", NodeID: 1, Command: domain.CmdPing, Target: "8.8.8.8",
	}, ExecuteOption{ShouldStop: func() bool { return stop.Load() }})
	if result.Status != domain.StatusDone {
		t.Fatalf("status = %s", result.Status)
	}
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
