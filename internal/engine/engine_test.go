package engine

import (
	"context"
	"testing"

	"github.com/yourorg/lg-looking-glass/internal/domain"
)

type mockNodeRepo struct {
	node *domain.Node
	err  error
}

func (m *mockNodeRepo) GetByID(ctx context.Context, id int64) (*domain.Node, error) {
	return m.node, m.err
}
func (m *mockNodeRepo) GetActive(ctx context.Context) ([]*domain.Node, error) { return nil, nil }
func (m *mockNodeRepo) GetAll(ctx context.Context) ([]*domain.Node, error)    { return nil, nil }
func (m *mockNodeRepo) Create(ctx context.Context, n *domain.Node) (*domain.Node, error) {
	return n, nil
}
func (m *mockNodeRepo) Update(ctx context.Context, n *domain.Node) (*domain.Node, error) {
	return n, nil
}
func (m *mockNodeRepo) Delete(ctx context.Context, id int64) error { return nil }
func (m *mockNodeRepo) UpdateEnabledCmds(ctx context.Context, id int64, cmds []domain.CommandType) error {
	return nil
}

func TestExecuteRateLimited(t *testing.T) {
	repo := &mockNodeRepo{
		node: &domain.Node{
			EnabledCmds: []domain.CommandType{domain.CmdPing},
		},
	}
	e := New(&QueryConfig{MaxConcurrent: 10}, repo, nil, nil)

	// Exhaust rate limit (10 per minute)
	for i := 0; i < 10; i++ {
		_, _ = e.Execute(context.Background(), &domain.Query{
			ID:        "q",
			NodeID:    1,
			Command:   domain.CmdPing,
			Target:    "8.8.8.8",
			SourceIP:  "1.2.3.4",
		})
	}

	// Next request should be rate limited
	result, _ := e.Execute(context.Background(), &domain.Query{
		ID:       "q-limited",
		NodeID:   1,
		Command:  domain.CmdPing,
		Target:   "8.8.8.8",
		SourceIP: "1.2.3.4",
	})
	if result.ErrorCode != "RATE_LIMITED" {
		t.Errorf("expected RATE_LIMITED, got %s", result.ErrorCode)
	}
}

func TestExecuteNodeNotFound(t *testing.T) {
	repo := &mockNodeRepo{err: domain.ErrNodeNotFound}
	e := New(&QueryConfig{MaxConcurrent: 10}, repo, nil, nil)

	result, _ := e.Execute(context.Background(), &domain.Query{
		ID:       "q",
		NodeID:   999,
		Command:  domain.CmdPing,
		Target:   "8.8.8.8",
		SourceIP: "1.2.3.4",
	})
	if result.ErrorCode != "NODE_NOT_FOUND" {
		t.Errorf("expected NODE_NOT_FOUND, got %s", result.ErrorCode)
	}
}

func TestExecuteCommandDisabled(t *testing.T) {
	repo := &mockNodeRepo{
		node: &domain.Node{EnabledCmds: []domain.CommandType{domain.CmdTraceroute}},
	}
	e := New(&QueryConfig{MaxConcurrent: 10}, repo, nil, nil)

	result, _ := e.Execute(context.Background(), &domain.Query{
		ID:       "q",
		NodeID:   1,
		Command:  domain.CmdPing,
		Target:   "8.8.8.8",
		SourceIP: "1.2.3.4",
	})
	if result.ErrorCode != "COMMAND_DISABLED" {
		t.Errorf("expected COMMAND_DISABLED, got %s", result.ErrorCode)
	}
}

func TestClassifyError(t *testing.T) {
	tests := []struct {
		err      error
		expected string
	}{
		{domain.ErrNodeNotFound, "NODE_NOT_FOUND"},
		{domain.ErrCommandDisabled, "COMMAND_DISABLED"},
		{domain.ErrInvalidTarget, "INVALID_TARGET"},
		{domain.ErrTimeout, "COMMAND_TIMEOUT"},
		{domain.ErrQueryPoolFull, "POOL_FULL"},
		{context.DeadlineExceeded, "COMMAND_TIMEOUT"},
	}
	for _, tt := range tests {
		got := classifyError(tt.err)
		if got != tt.expected {
			t.Errorf("classifyError(%v) = %q, want %q", tt.err, got, tt.expected)
		}
	}
}

func TestNewEngine(t *testing.T) {
	cfg := &QueryConfig{
		MaxConcurrent:        50,
		DefaultTimeoutSec:    30,
		MTRTimeoutSec:        120,
		TracerouteTimeoutSec: 60,
	}
	e := New(cfg, &mockNodeRepo{}, nil, nil)
	if e == nil {
		t.Fatal("expected non-nil engine")
	}
}

func TestEngineEnrichHopsNilGeo(t *testing.T) {
	e := New(&QueryConfig{MaxConcurrent: 10}, &mockNodeRepo{}, nil, nil)
	// Should not panic with nil geoDB
	hops := []domain.Hop{{IP: "8.8.8.8"}}
	e.enrichHops(context.Background(), hops)
	if hops[0].ASInfo != nil {
		t.Error("expected nil ASInfo with nil geoDB")
	}
}

func TestEngineEnrichMTRHopsNilGeo(t *testing.T) {
	e := New(&QueryConfig{MaxConcurrent: 10}, &mockNodeRepo{}, nil, nil)
	hops := []domain.MTRHop{{Host: "8.8.8.8"}}
	e.enrichMTRHops(context.Background(), hops)
	if hops[0].ASInfo != nil {
		t.Error("expected nil ASInfo with nil geoDB")
	}
}

func TestEngineEnrichASPathNilGeo(t *testing.T) {
	e := New(&QueryConfig{MaxConcurrent: 10}, &mockNodeRepo{}, nil, nil)
	br := &domain.BGPResult{
		Routes: []domain.BGPRoute{
			{Prefix: "1.1.1.0/24", ASPath: []uint32{13335}},
		},
	}
	result := &domain.QueryResult{}
	e.enrichASPath(context.Background(), br, result)
	// With nil geoDB, should return early without enrichment
}
