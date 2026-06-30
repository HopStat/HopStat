package bgp

import (
	"testing"

	"github.com/HopStat/HopStat/internal/domain"
)

func TestNewRouteFallback(t *testing.T) {
	fb := NewRouteFallback(9121, 6453)
	if fb == nil || fb.LocalAS != 9121 || fb.DefaultRouteAS != 6453 {
		t.Fatalf("unexpected fallback: %+v", fb)
	}
	if NewRouteFallback(0, 6453) != nil {
		t.Fatal("expected nil without local AS")
	}
	if NewRouteFallback(9121, 0) != nil {
		t.Fatal("expected nil without default route AS")
	}
}

func TestSynthesizeDefaultRouteResult(t *testing.T) {
	fb := RouteFallback{LocalAS: 9121, DefaultRouteAS: 6453}
	br := SynthesizeDefaultRouteResult("8.8.8.8", []*domain.BGPRouteEntry{{
		Prefix:  "0.0.0.0/0",
		NextHop: "10.0.0.1",
		Origin:  "IGP",
		Best:    true,
	}}, fb)
	if len(br.Routes) != 1 {
		t.Fatalf("expected 1 route, got %d", len(br.Routes))
	}
	r := br.Routes[0]
	if r.Prefix != "8.8.8.8/32" {
		t.Fatalf("prefix = %q", r.Prefix)
	}
	if len(r.ASPath) != 2 || r.ASPath[0] != 9121 || r.ASPath[1] != 6453 {
		t.Fatalf("as_path = %v", r.ASPath)
	}
	if !r.ViaDefaultRoute || r.NextHop != "10.0.0.1" {
		t.Fatalf("route = %+v", r)
	}
}

func TestDefaultRoutePrefix(t *testing.T) {
	if got := DefaultRoutePrefix("8.8.8.8"); got != "0.0.0.0/0" {
		t.Fatalf("ipv4 default = %q", got)
	}
	if got := DefaultRoutePrefix("2001:db8::1"); got != "::/0" {
		t.Fatalf("ipv6 default = %q", got)
	}
}

func TestApplyOriginASPathToRoutes(t *testing.T) {
	routes := []domain.BGPRoute{
		{Prefix: "1.1.1.0/24", ASPath: []uint32{15169, 13335}},
		{Prefix: "8.8.8.8/32", ASPath: []uint32{9121, 6453, 15169}, ViaDefaultRoute: true},
	}
	ApplyOriginASPathToRoutes(routes, 9121)

	if len(routes[0].ASPath) != 3 || routes[0].ASPath[0] != 9121 || routes[0].ASPath[1] != 15169 {
		t.Fatalf("learned route as_path = %v", routes[0].ASPath)
	}
	if len(routes[1].ASPath) != 3 || routes[1].ASPath[0] != 9121 || routes[1].ASPath[1] != 6453 {
		t.Fatalf("default route as_path = %v", routes[1].ASPath)
	}
}

func TestPrependOriginASPath(t *testing.T) {
	fb := RouteFallback{LocalAS: 9121, DefaultRouteAS: 6453}

	got := PrependOriginASPath([]uint32{15169, 13335}, &fb, false)
	want := []uint32{9121, 15169, 13335}
	if len(got) != len(want) {
		t.Fatalf("got %v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v want %v", got, want)
		}
	}

	got = PrependOriginASPath([]uint32{9121, 6453, 15169}, &fb, true)
	want = []uint32{9121, 6453, 15169}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("duplicate prefix: got %v want %v", got, want)
		}
	}

	got = PrependOriginASPath([]uint32{9121, 15169}, &fb, false)
	want = []uint32{9121, 15169}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("local only prefix: got %v want %v", got, want)
		}
	}

	got = PrependOriginASPath([]uint32{9121, 15169}, &fb, true)
	want = []uint32{9121, 6453, 15169}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("default route prefix: got %v want %v", got, want)
		}
	}

	got = PrependOriginASPath([]uint32{15169}, nil, false)
	if len(got) != 1 || got[0] != 15169 {
		t.Fatalf("nil fallback should preserve path, got %v", got)
	}
}

func TestDefaultEntryForNeighbor(t *testing.T) {
	defaults := []*domain.BGPRouteEntry{
		{NeighborIP: "10.0.0.1", Prefix: "0.0.0.0/0"},
		{NeighborIP: "10.0.0.2", Prefix: "0.0.0.0/0", Best: true},
	}
	if got := defaultEntryForNeighbor(defaults, "10.0.0.1", ""); got == nil || got.NeighborIP != "10.0.0.1" {
		t.Fatalf("expected ipv4 neighbor entry, got %+v", got)
	}
	if got := defaultEntryForNeighbor(defaults, "10.0.0.9", ""); got != nil {
		t.Fatalf("expected nil for unknown neighbor, got %+v", got)
	}
}

func TestTargetIPAndDisplayPrefix(t *testing.T) {
	if ip := targetIP("10.0.0.0/8"); ip == nil || ip.String() != "10.0.0.0" {
		t.Fatalf("targetIP cidr = %v", ip)
	}
	if ip := targetIP("not-an-ip"); ip != nil {
		t.Fatalf("targetIP invalid = %v", ip)
	}
	if got := displayPrefix("8.8.8.8"); got != "8.8.8.8/32" {
		t.Fatalf("displayPrefix v4 = %q", got)
	}
	if got := displayPrefix("2001:db8::1"); got != "2001:db8::1/128" {
		t.Fatalf("displayPrefix v6 = %q", got)
	}
	if got := displayPrefix("already/24"); got != "already/24" {
		t.Fatalf("displayPrefix existing = %q", got)
	}
	if got := displayPrefix("8.8.8.8/"); got != "8.8.8.8/32" {
		t.Fatalf("displayPrefix trailing slash = %q", got)
	}
	if got := displayPrefix("::1/"); got != "::1/128" {
		t.Fatalf("displayPrefix v6 trailing slash = %q", got)
	}
	if got := displayPrefix("bad"); got != "bad" {
		t.Fatalf("displayPrefix bad = %q", got)
	}
	if got := displayPrefix(""); got != "" {
		t.Fatalf("displayPrefix empty = %q", got)
	}
}

func TestIsDefaultRoutePrefix(t *testing.T) {
	if !isDefaultRoutePrefix("0.0.0.0/0") || !isDefaultRoutePrefix("::/0") {
		t.Fatal("expected default route prefixes")
	}
	if isDefaultRoutePrefix("8.8.8.8/32") {
		t.Fatal("host prefix should not be default")
	}
}

func TestOriginPrefixAndPrependEmpty(t *testing.T) {
	fb := RouteFallback{LocalAS: 0, DefaultRouteAS: 6453}
	if got := fb.originPrefix(true); got != nil {
		t.Fatalf("originPrefix no local AS = %v", got)
	}
	got := PrependOriginASPath(nil, &RouteFallback{LocalAS: 9121, DefaultRouteAS: 6453}, true)
	if len(got) != 2 || got[0] != 9121 {
		t.Fatalf("prepend empty path = %v", got)
	}
}

func TestApplyOriginASPathToRoutesNoop(t *testing.T) {
	ApplyOriginASPathToRoutes(nil, 9121)
	ApplyOriginASPathToRoutes([]domain.BGPRoute{{Prefix: "1.0.0.0/24"}}, 0)
}
