package engine

import (
	"context"
	"testing"

	"github.com/HopStat/HopStat/internal/domain"
)

type stubBGPDriver struct {
	result *domain.BGPResult
}

func (d stubBGPDriver) Capabilities() []domain.CommandType { return nil }
func (d stubBGPDriver) TestConnection(context.Context) error { return nil }
func (d stubBGPDriver) Ping(context.Context, string, int) (*domain.PingResult, error) {
	return nil, nil
}
func (d stubBGPDriver) Traceroute(context.Context, string, int) (*domain.TracerouteResult, error) {
	return nil, nil
}
func (d stubBGPDriver) BGPRoute(context.Context, string) (*domain.BGPResult, error) {
	copyRoutes := append([]domain.BGPRoute(nil), d.result.Routes...)
	return &domain.BGPResult{Routes: copyRoutes}, nil
}

func TestLookupBGPRoutesSkipsOriginPrependForLGNode(t *testing.T) {
	e := New(&QueryConfig{MaxConcurrent: 10}, &mockNodeRepo{}, nil, nil, nil, nil, 9121)

	drv := stubBGPDriver{
		result: &domain.BGPResult{
			Routes: []domain.BGPRoute{
				{Prefix: "8.8.8.8/32", ASPath: []uint32{6453, 15169}, Best: true},
			},
		},
	}

	got, err := e.lookupBGPRoutes(context.Background(), drv, "8.8.8.8", 1, domain.NodeTypeLGNode)
	if err != nil {
		t.Fatalf("lookupBGPRoutes error: %v", err)
	}
	if len(got.Routes[0].ASPath) != 2 || got.Routes[0].ASPath[0] != 6453 {
		t.Fatalf("lg_node as_path = %v, want remote path unchanged", got.Routes[0].ASPath)
	}

	gotStandalone, err := e.lookupBGPRoutes(context.Background(), drv, "8.8.8.8", 1, domain.NodeTypeStandalone)
	if err != nil {
		t.Fatalf("lookupBGPRoutes standalone error: %v", err)
	}
	if len(gotStandalone.Routes[0].ASPath) != 3 || gotStandalone.Routes[0].ASPath[0] != 9121 {
		t.Fatalf("standalone as_path = %v, want local AS prepended", gotStandalone.Routes[0].ASPath)
	}
}
