package bgp

import (
	"context"
	"testing"

	"github.com/HopStat/HopStat/internal/domain"
)

type stubTargetASResolver struct {
	info *domain.ASInfo
}

func (s stubTargetASResolver) ResolveASN(_ context.Context, ip string) (*domain.ASInfo, error) {
	if ip != "8.8.8.8" {
		return nil, nil
	}
	return s.info, nil
}

func TestEnrichResultTargetAS(t *testing.T) {
	br := &domain.BGPResult{
		Routes: []domain.BGPRoute{{Prefix: "8.8.8.8/32", ASPath: []uint32{9121, 6453}}},
	}
	resolver := stubTargetASResolver{
		info: &domain.ASInfo{ASN: 15169, OrgName: "GOOGLE", CountryCode: "US"},
	}

	enrichResultTargetAS(context.Background(), resolver, br, "8.8.8.8")
	if br.TargetAS == nil || br.TargetAS.ASN != 15169 {
		t.Fatalf("target_as = %+v", br.TargetAS)
	}
	if br.TargetAS.OrgName != "GOOGLE" {
		t.Fatalf("org = %q", br.TargetAS.OrgName)
	}
}

func TestEnrichResultTargetASKeepsExisting(t *testing.T) {
	existing := &domain.ASInfo{ASN: 13335}
	br := &domain.BGPResult{TargetAS: existing}
	resolver := stubTargetASResolver{info: &domain.ASInfo{ASN: 15169}}

	enrichResultTargetAS(context.Background(), resolver, br, "8.8.8.8")
	if br.TargetAS != existing {
		t.Fatal("expected existing target_as to be preserved")
	}
}

func TestEnrichResultTargetASPublicAndQueryTargetIP(t *testing.T) {
	EnrichResultTargetAS(context.Background(), nil, &domain.BGPResult{}, "8.8.8.8")

	if got := queryTargetIP("10.0.0.0/8"); got != "10.0.0.0" {
		t.Fatalf("queryTargetIP cidr = %q", got)
	}
	if got := queryTargetIP("bad-prefix"); got != "" {
		t.Fatalf("queryTargetIP invalid = %q", got)
	}

	br := &domain.BGPResult{}
	enrichResultTargetAS(context.Background(), stubTargetASResolver{}, br, "not-an-ip")
	if br.TargetAS != nil {
		t.Fatal("expected no target AS for invalid prefix")
	}

	br2 := &domain.BGPResult{}
	enrichResultTargetAS(context.Background(), stubTargetASResolver{info: &domain.ASInfo{ASN: 0}}, br2, "8.8.8.8")
	if br2.TargetAS != nil {
		t.Fatal("expected nil for zero ASN")
	}
}

func TestEnrichResultTargetASFlagEmoji(t *testing.T) {
	br := &domain.BGPResult{}
	resolver := stubTargetASResolver{
		info: &domain.ASInfo{ASN: 15169, CountryCode: "US"},
	}
	enrichResultTargetAS(context.Background(), resolver, br, "8.8.8.8")
	if br.TargetAS == nil || br.TargetAS.FlagEmoji == "" {
		t.Fatalf("target_as = %+v", br.TargetAS)
	}
}
