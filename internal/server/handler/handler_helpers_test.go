package handler

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/HopStat/HopStat/internal/domain"
)

func TestSettingInt(t *testing.T) {
	settings := map[string]string{"ping_count": "7", "bad": "nope"}
	if got := settingInt(settings, "ping_count", 5, 1, 20); got != 7 {
		t.Errorf("got %d", got)
	}
	if got := settingInt(settings, "missing", 5, 1, 20); got != 5 {
		t.Errorf("got %d", got)
	}
	if got := settingInt(settings, "bad", 5, 1, 20); got != 5 {
		t.Errorf("got %d", got)
	}
	if got := settingInt(map[string]string{"x": "99"}, "x", 5, 1, 20); got != 20 {
		t.Errorf("got %d, want clamped max", got)
	}
}

func TestFormatInt64Ptr(t *testing.T) {
	if formatInt64Ptr(nil) != "" {
		t.Error("expected empty for nil")
	}
	v := int64(42)
	if formatInt64Ptr(&v) != "42" {
		t.Errorf("got %q", formatInt64Ptr(&v))
	}
}

func TestFriendlyConnError(t *testing.T) {
	if msg := friendlyConnError(errors.New("connection refused")); msg == "" || msg == "internal error" {
		t.Errorf("got %q", msg)
	}
	if msg := friendlyConnError(errors.New("health check failed: 401")); msg != "invalid agent token" {
		t.Errorf("got %q", msg)
	}
}

func TestSkipASPathWaitForBGP(t *testing.T) {
	withRoutes := &domain.QueryResult{
		Status: domain.StatusDone,
		Parsed: &domain.BGPResult{Routes: []domain.BGPRoute{{Prefix: "8.8.8.8/32"}}},
	}
	if skipASPathWaitForBGP("bgp_route", withRoutes) {
		t.Error("expected false when routes present")
	}
	emptyRoutes := &domain.QueryResult{
		Status: domain.StatusDone,
		Parsed: &domain.BGPResult{},
	}
	if !skipASPathWaitForBGP("bgp_route", emptyRoutes) {
		t.Error("expected true when bgp routes empty")
	}
	if skipASPathWaitForBGP("ping", withRoutes) {
		t.Error("expected false for ping")
	}
}

func TestMergeASPathFields(t *testing.T) {
	dest := &domain.QueryResult{}
	stored := &domain.QueryResult{
		ASPath:         []uint32{15169},
		ASPathEnriched: []domain.ASInfo{{ASN: 15169, OrgName: "GOOGLE"}},
	}
	mergeASPathFields(dest, stored)
	if len(dest.ASPath) != 1 || dest.ASPath[0] != 15169 {
		t.Fatalf("ASPath = %+v", dest.ASPath)
	}
	if len(dest.ASPathEnriched) != 1 {
		t.Fatalf("ASPathEnriched = %+v", dest.ASPathEnriched)
	}
}

func TestUniqueASNsInPath(t *testing.T) {
	if n := uniqueASNsInPath([]uint32{64512, 64512, 15169}); n != 2 {
		t.Errorf("got %d", n)
	}
}

func TestDistinctPublicIPs(t *testing.T) {
	ips := distinctPublicIPs([]string{"  1.2.3.4 ms", "1.2.3.4 ms", "  127.0.0.1 ms"})
	if len(ips) != 1 || ips[0] != "1.2.3.4" {
		t.Fatalf("got %v", ips)
	}
}

func TestAsPathEnrichedHasLabels(t *testing.T) {
	if asPathEnrichedHasLabels([]domain.ASInfo{{OrgName: "Google"}}) != true {
		t.Fatal("expected true")
	}
	if asPathEnrichedHasLabels([]domain.ASInfo{{ASN: 1}}) {
		t.Fatal("expected false for empty labels")
	}
}

func TestAsPathEnrichmentReady(t *testing.T) {
	if !asPathEnrichmentReady(nil) {
		t.Fatal("nil result should be ready")
	}
	if !asPathEnrichmentReady(&domain.QueryResult{}) {
		t.Fatal("empty AS path should be ready")
	}
	partial := &domain.QueryResult{
		ASPath:         []uint32{64512, 15169},
		ASPathEnriched: []domain.ASInfo{{ASN: 64512}},
	}
	if asPathEnrichmentReady(partial) {
		t.Fatal("expected not ready until all ASNs enriched")
	}
	partial.ASPathEnriched = append(partial.ASPathEnriched, domain.ASInfo{ASN: 15169})
	if !asPathEnrichmentReady(partial) {
		t.Fatal("expected ready when all unique ASNs present")
	}
}

func TestMergeASPathFields_AllBranches(t *testing.T) {
	dest := &domain.QueryResult{ASPathEnriched: []domain.ASInfo{{ASN: 1}}}
	stored := &domain.QueryResult{
		ASPath:         []uint32{64512},
		ASPathPrefix:   "8.8.8.8/32",
		ASPathEnriched: []domain.ASInfo{{ASN: 64512, OrgName: "X"}},
	}
	mergeASPathFields(dest, stored)
	if len(dest.ASPath) != 1 || dest.ASPathPrefix == "" {
		t.Fatalf("dest = %+v", dest)
	}

	mergeASPathFields(nil, stored)
	mergeASPathFields(dest, nil)

	dest2 := &domain.QueryResult{
		ASPathEnriched: []domain.ASInfo{{ASN: 1, OrgName: "Labeled"}},
	}
	stored2 := &domain.QueryResult{ASPathEnriched: []domain.ASInfo{{ASN: 1}}}
	mergeASPathFields(dest2, stored2)
	if len(dest2.ASPathEnriched) != 1 || dest2.ASPathEnriched[0].OrgName == "" {
		t.Fatalf("expected labeled enrichment preserved")
	}
}

func TestSkipASPathWaitForBGP_NonBGPResult(t *testing.T) {
	if skipASPathWaitForBGP("bgp_route", &domain.QueryResult{Parsed: &domain.PingResult{}}) {
		t.Fatal("expected false for non-bgp parsed type")
	}
}

func TestEnrichLineWithASAndSuffix(t *testing.T) {
	geo := stubGeo{byIP: map[string]*domain.ASInfo{
		"8.8.8.8": {ASN: 15169, OrgName: "Google"},
	}}
	line := enrichLineWithAS(context.Background(), geo, " 1  dns.google (8.8.8.8)  1 ms")
	if !strings.Contains(line, "[AS15169") {
		t.Fatalf("got %q", line)
	}
	if suffix := asSuffixForIP(context.Background(), geo, "127.0.0.1"); suffix != "" {
		t.Fatalf("private ip suffix = %q", suffix)
	}
}

func TestSplitTracerouteProbesEmpty(t *testing.T) {
	if splitTracerouteProbes("") != nil {
		t.Fatal("expected nil for empty body")
	}
	if probes := splitTracerouteProbes("no-ip-here 1 ms"); len(probes) != 1 {
		t.Fatalf("got %v", probes)
	}
}

func TestFormatTracerouteLineEmptyAndNoMatch(t *testing.T) {
	if got := formatTracerouteLine(context.Background(), stubGeo{}, ""); got != "" {
		t.Fatalf("got %q", got)
	}
	if got := formatTracerouteLine(context.Background(), stubGeo{}, "not a hop line"); got == "" {
		t.Fatal("expected passthrough")
	}
}

func TestAdminDiagError(t *testing.T) {
	if adminDiagError(nil) != "" {
		t.Error("expected empty for nil")
	}
	if adminDiagError(errors.New("boom")) != "boom" {
		t.Error("expected message passthrough")
	}
}
