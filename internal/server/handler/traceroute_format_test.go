package handler

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/HopStat/HopStat/internal/domain"
	"github.com/HopStat/HopStat/internal/geo"
)

type stubGeo struct {
	byIP map[string]*domain.ASInfo
}

func (s stubGeo) ResolveASN(_ context.Context, ip string) (*domain.ASInfo, error) {
	if info, ok := s.byIP[ip]; ok {
		return info, nil
	}
	return &domain.ASInfo{}, nil
}

func TestFormatTracerouteLineSplitsECMPHop(t *testing.T) {
	line := " 7  142.250.56.110 (142.250.56.110)  15.640 ms 142.251.227.252 (142.251.227.252)  14.155 ms 209.85.248.178 (209.85.248.178)  16.606 ms"
	geo := stubGeo{byIP: map[string]*domain.ASInfo{
		"142.250.56.110":  {ASN: 15169, OrgName: "Google LLC"},
		"142.251.227.252": {ASN: 15169, OrgName: "Google LLC"},
		"209.85.248.178":  {ASN: 15169, OrgName: "Google LLC"},
	}}

	got := formatTracerouteLine(context.Background(), geo, line)
	parts := strings.Split(got, "\n")
	if len(parts) != 3 {
		t.Fatalf("expected 3 lines, got %d:\n%s", len(parts), got)
	}
	if !strings.HasPrefix(parts[0], " 7  ") {
		t.Fatalf("first line should keep hop number: %q", parts[0])
	}
	if !strings.HasPrefix(parts[1], "    ") {
		t.Fatalf("second line should be indented: %q", parts[1])
	}
	for i, part := range parts {
		if !strings.Contains(part, "[AS15169") {
			t.Fatalf("line %d missing AS tag: %q", i, part)
		}
	}
}

func TestFormatTracerouteLineKeepsSingleResponderHop(t *testing.T) {
	line := " 1  10.20.10.17 (10.20.10.17)  2.048 ms  2.077 ms  2.062 ms"
	got := formatTracerouteLine(context.Background(), stubGeo{}, line)
	if strings.Contains(got, "\n") {
		t.Fatalf("expected single line for one responder, got %q", got)
	}
}

func TestFormatTracerouteLineEnrichesPublicSingleHop(t *testing.T) {
	line := " 5  google.atlantisnet.com.tr (87.121.22.53)  13.104 ms  13.056 ms  13.068 ms"
	geo := stubGeo{byIP: map[string]*domain.ASInfo{
		"87.121.22.53": {ASN: 204457, OrgName: "Atlantis Telekomunikasyon"},
	}}
	got := formatTracerouteLine(context.Background(), geo, line)
	if strings.Count(got, "[AS204457") != 1 {
		t.Fatalf("expected one AS tag at end, got %q", got)
	}
}

func TestTracerouteLineIPPrefersParenthesizedAddress(t *testing.T) {
	line := " 9  166.103.192.193.static.turk.net (193.192.103.166)  12.695 ms"
	if got := tracerouteLineIP(line); got != "193.192.103.166" {
		t.Fatalf("got %q, want parenthesized probe IP", got)
	}
}

func TestFormatTracerouteLineTurkNetReversedHostname(t *testing.T) {
	line := " 9  166.103.192.193.static.turk.net (193.192.103.166)  12.695 ms  12.606 ms"
	geo := stubGeo{byIP: map[string]*domain.ASInfo{
		"193.192.103.166": {ASN: 12735, OrgName: "TurkNet Iletisim Hizmetleri A.S."},
		"166.103.192.193": {ASN: 99999, OrgName: "Wrong Hostname IP"},
	}}
	got := formatTracerouteLine(context.Background(), geo, line)
	if !strings.Contains(got, "[AS12735") {
		t.Fatalf("expected TurkNet AS from parenthesized IP, got %q", got)
	}
	if strings.Contains(got, "99999") {
		t.Fatalf("should not use dotted quad from hostname: %q", got)
	}
}

func TestFormatTracerouteLineTurkNetSRVHostname(t *testing.T) {
	line := " 11  33.92.146.159.srv.turk.net (159.146.92.33)  12.706 ms"
	geo := stubGeo{byIP: map[string]*domain.ASInfo{
		"159.146.92.33": {ASN: 12735, OrgName: "TurkNet Iletisim Hizmetleri A.S."},
		"33.92.146.159": {ASN: 749, OrgName: "DoD Wrong"},
	}}
	got := formatTracerouteLine(context.Background(), geo, line)
	if !strings.Contains(got, "[AS12735") {
		t.Fatalf("expected TurkNet AS from parenthesized IP, got %q", got)
	}
	if strings.Contains(got, "[AS749") {
		t.Fatalf("should not use dotted quad from hostname: %q", got)
	}
}

func TestFormatTracerouteLineLiveTurkNet(t *testing.T) {
	asnPath := filepath.Join("..", "..", "..", "data", "geoip", "GeoLite2-ASN.mmdb")
	cityPath := filepath.Join("..", "..", "..", "data", "geoip", "GeoLite2-City.mmdb")
	if _, err := os.Stat(asnPath); err != nil {
		t.Skip("local geoip db not present")
	}

	g := geo.New(asnPath, cityPath)
	if err := g.Reload(); err != nil {
		t.Fatalf("reload: %v", err)
	}

	line := " 9  166.103.192.193.static.turk.net (193.192.103.166)  12.695 ms  12.606 ms"
	got := formatTracerouteLine(context.Background(), g, line)
	if !strings.Contains(got, "[AS12735") {
		t.Fatalf("expected TurkNet AS12735 from blocks CSV, got %q", got)
	}
}
