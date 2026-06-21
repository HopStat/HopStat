package engine

import (
	"context"
	"testing"

	"github.com/HopStat/HopStat/internal/domain"
	"github.com/HopStat/HopStat/internal/geo"
)

func TestLookupASForPathNilGeo(t *testing.T) {
	info, err := lookupASForPath(nil, context.Background(), 15169)
	if err != nil || info != nil {
		t.Fatalf("info=%+v err=%v", info, err)
	}
}

func TestExecuteTraceroutePoolError(t *testing.T) {
	repo := &idNodeRepo{nodes: map[int64]*domain.Node{1: lgNode(1, "http://127.0.0.1:1")}}
	e := New(&QueryConfig{MaxConcurrent: 4, DefaultTimeoutSec: 1, TracerouteTimeoutSec: 1}, repo, nil, nil, nil, nil, 0)
	result, _ := e.Execute(context.Background(), &domain.Query{
		ID: "q", NodeID: 1, Command: domain.CmdTraceroute, Target: "8.8.8.8",
		Options: domain.QueryOptions{MaxHops: 5},
	})
	if result.Status != domain.StatusError {
		t.Fatalf("status = %s", result.Status)
	}
}

func TestEnrichASPathSetsFlagEmojiFromCountry(t *testing.T) {
	old := lookupASForPath
	lookupASForPath = func(_ *geo.GeoIPDB, _ context.Context, asn uint32) (*domain.ASInfo, error) {
		return &domain.ASInfo{ASN: asn, OrgName: "Example", CountryCode: "US"}, nil
	}
	t.Cleanup(func() { lookupASForPath = old })

	e := New(&QueryConfig{MaxConcurrent: 4}, &mockNodeRepo{}, nil, testGeoDB(t), nil, nil, 0)
	br := &domain.BGPResult{Routes: []domain.BGPRoute{{ASPath: []uint32{15169}, Best: true}}}
	result := &domain.QueryResult{ASPath: []uint32{15169}}
	e.enrichASPath(context.Background(), br, result, ExecuteOption{})
	if len(result.ASPathEnriched) != 1 || result.ASPathEnriched[0].FlagEmoji == "" {
		t.Fatalf("enriched = %+v", result.ASPathEnriched)
	}
}
