package engine

import (
	"context"
	"testing"

	"github.com/HopStat/HopStat/internal/domain"
	"github.com/HopStat/HopStat/internal/geo"
)

func TestCollectBGPASPathASNsFromAllRoutes(t *testing.T) {
	result := &domain.QueryResult{ASPath: []uint32{43260, 15169}}
	br := &domain.BGPResult{Routes: []domain.BGPRoute{
		{ASPath: []uint32{43260, 204457, 15169}, Best: true},
		{ASPath: []uint32{43260, 3356}, Best: false},
	}}
	got := collectBGPASPathASNs(result, br)
	want := []uint32{43260, 15169, 204457, 3356}
	if len(got) != len(want) {
		t.Fatalf("asns = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("asns = %v, want %v", got, want)
		}
	}
}

func TestEnrichASPathIncludesSecondaryRouteASNs(t *testing.T) {
	g := testGeoDB(t)
	e := New(&QueryConfig{MaxConcurrent: 4}, &mockNodeRepo{}, nil, g, nil, nil, 0)
	originalLookupASForPath := lookupASForPath
	lookupASForPath = func(_ *geo.GeoIPDB, _ context.Context, asn uint32) (*domain.ASInfo, error) {
		return &domain.ASInfo{
			ASN:         asn,
			OrgName:     "Org",
			ShortName:   "Org",
			CountryCode: "US",
		}, nil
	}
	t.Cleanup(func() { lookupASForPath = originalLookupASForPath })

	br := &domain.BGPResult{Routes: []domain.BGPRoute{
		{ASPath: []uint32{43260, 15169}, Best: true},
		{ASPath: []uint32{43260, 3356}, Best: false},
	}}
	result := &domain.QueryResult{ASPath: []uint32{43260, 15169}}
	e.enrichASPath(context.Background(), br, result, ExecuteOption{})
	if len(result.ASPathEnriched) != 3 {
		t.Fatalf("ASPathEnriched = %+v", result.ASPathEnriched)
	}
}

func TestEnrichASPathEsenyurtCloudflareSecondaryRoute(t *testing.T) {
	g := testGeoDB(t)
	e := New(&QueryConfig{MaxConcurrent: 4}, &mockNodeRepo{}, nil, g, nil, nil, 0)
	originalLookupASForPath := lookupASForPath
	lookupASForPath = func(_ *geo.GeoIPDB, _ context.Context, asn uint32) (*domain.ASInfo, error) {
		switch asn {
		case 201178:
			return &domain.ASInfo{
				ASN:         201178,
				OrgName:     "EURONET - Euronet Telekomunikasyon A.S.",
				ShortName:   "EURONET",
				CountryCode: "TR",
			}, nil
		case 44901:
			return &domain.ASInfo{ASN: 44901, OrgName: "Belcloud LTD", ShortName: "Belcloud", CountryCode: "BG"}, nil
		default:
			return &domain.ASInfo{ASN: asn, OrgName: "Org", ShortName: "Org", CountryCode: "US"}, nil
		}
	}
	t.Cleanup(func() { lookupASForPath = originalLookupASForPath })

	br := &domain.BGPResult{Routes: []domain.BGPRoute{
		{Prefix: "1.1.1.0/24", ASPath: []uint32{43260, 44901, 13335}, Best: true, NodeName: "ESENYURT"},
		{Prefix: "1.1.1.0/24", ASPath: []uint32{43260, 201178, 13335}, NodeName: "ESENYURT"},
	}}
	result := &domain.QueryResult{ASPath: []uint32{43260, 44901, 13335}}
	e.enrichASPath(context.Background(), br, result, ExecuteOption{})

	var got201178 *domain.ASInfo
	for i := range result.ASPathEnriched {
		if result.ASPathEnriched[i].ASN == 201178 {
			got201178 = &result.ASPathEnriched[i]
			break
		}
	}
	if got201178 == nil || got201178.OrgName == "" {
		t.Fatalf("ASPathEnriched = %+v", result.ASPathEnriched)
	}
}
