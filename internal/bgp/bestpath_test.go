package bgp

import (
	"testing"

	"github.com/HopStat/HopStat/internal/domain"
)

func entriesFor(prefix string, in ...*domain.BGPRouteEntry) []*domain.BGPRouteEntry {
	for _, e := range in {
		e.Prefix = prefix
	}
	return in
}

func bestOf(entries []*domain.BGPRouteEntry) *domain.BGPRouteEntry {
	for _, e := range entries {
		if e.Best {
			return e
		}
	}
	return nil
}

// The live case: both paths were learned when our session came up, so they are the same
// age and arrive over the same neighbour. Only the next hop separates them, and the router
// picks the lower one.
func TestEnsureBestAmongEntriesBreaksTiesOnNextHop(t *testing.T) {
	entries := entriesFor("8.8.8.0/24",
		&domain.BGPRouteEntry{
			ASPath: "44901 15169", Origin: "IGP", LocalPref: "100", MED: "0",
			NextHop: "172.16.16.65", NeighborIP: "10.0.0.1", AgeSeconds: 209,
		},
		&domain.BGPRouteEntry{
			ASPath: "204457 15169", Origin: "IGP", LocalPref: "100", MED: "0",
			NextHop: "10.183.1.25", NeighborIP: "10.0.0.1", AgeSeconds: 209,
		},
	)

	ensureBestAmongEntries(entries)

	best := bestOf(entries)
	if best == nil || best.ASPath != "204457 15169" {
		t.Fatalf("best = %+v, want the lower next hop", best)
	}
}

func TestAddressKeyOrdersNumerically(t *testing.T) {
	if addressKey("9.0.0.1") >= addressKey("10.0.0.1") {
		t.Fatal("9.0.0.1 must sort below 10.0.0.1")
	}
	if addressKey("10.183.1.25") >= addressKey("172.16.16.65") {
		t.Fatal("unexpected ordering for the reported addresses")
	}
	// Anything unparseable still compares consistently rather than panicking.
	if addressKey("not-an-ip") != "not-an-ip" {
		t.Fatal("unparseable addresses should pass through")
	}
	if addressKey("2001:db8::1") == addressKey("2001:db8::2") {
		t.Fatal("IPv6 addresses must stay distinguishable")
	}
}

// A genuinely older path still wins before addresses are consulted.
func TestEnsureBestAmongEntriesPrefersTheEstablishedPath(t *testing.T) {
	entries := entriesFor("8.8.8.0/24",
		&domain.BGPRouteEntry{
			ASPath: "44901 15169", Origin: "IGP", LocalPref: "100",
			NeighborIP: "172.16.16.65", AgeSeconds: 5 * 24 * 3600,
		},
		&domain.BGPRouteEntry{
			ASPath: "204457 15169", Origin: "IGP", LocalPref: "100",
			NeighborIP: "10.183.1.25", AgeSeconds: 13*24*3600 + 13*3600,
		},
	)

	ensureBestAmongEntries(entries)

	best := bestOf(entries)
	if best == nil || best.ASPath != "204457 15169" {
		t.Fatalf("best = %+v, want the older 204457 path", best)
	}
	if entries[0].Best {
		t.Fatal("the newer path must not stay marked just because it came first")
	}
}

func TestEnsureBestAmongEntriesFollowsTheDecisionProcess(t *testing.T) {
	tests := []struct {
		name    string
		entries []*domain.BGPRouteEntry
		want    string
	}{
		{
			name: "higher local preference wins",
			entries: entriesFor("1.0.0.0/24",
				&domain.BGPRouteEntry{ASPath: "1 2", LocalPref: "100", Origin: "IGP"},
				&domain.BGPRouteEntry{ASPath: "3 4 5", LocalPref: "200", Origin: "IGP"},
			),
			want: "3 4 5",
		},
		{
			name: "then the shorter AS path",
			entries: entriesFor("1.0.0.0/24",
				&domain.BGPRouteEntry{ASPath: "1 2 3", LocalPref: "100", Origin: "IGP"},
				&domain.BGPRouteEntry{ASPath: "4 5", LocalPref: "100", Origin: "IGP"},
			),
			want: "4 5",
		},
		{
			name: "then the lower origin",
			entries: entriesFor("1.0.0.0/24",
				&domain.BGPRouteEntry{ASPath: "1 2", LocalPref: "100", Origin: "incomplete"},
				&domain.BGPRouteEntry{ASPath: "3 4", LocalPref: "100", Origin: "IGP"},
			),
			want: "3 4",
		},
		{
			name: "then the lower MED",
			entries: entriesFor("1.0.0.0/24",
				&domain.BGPRouteEntry{ASPath: "1 2", LocalPref: "100", Origin: "IGP", MED: "50"},
				&domain.BGPRouteEntry{ASPath: "3 4", LocalPref: "100", Origin: "IGP", MED: "10"},
			),
			want: "3 4",
		},
		{
			name: "a missing local preference counts as the default",
			entries: entriesFor("1.0.0.0/24",
				&domain.BGPRouteEntry{ASPath: "1 2", Origin: "IGP"},
				&domain.BGPRouteEntry{ASPath: "3 4", LocalPref: "90", Origin: "IGP"},
			),
			want: "1 2",
		},
		{
			name: "identical paths fall back to the neighbour address",
			entries: entriesFor("1.0.0.0/24",
				&domain.BGPRouteEntry{ASPath: "1 2", LocalPref: "100", Origin: "IGP", NeighborIP: "10.0.0.9"},
				&domain.BGPRouteEntry{ASPath: "1 2", LocalPref: "100", Origin: "IGP", NeighborIP: "10.0.0.2"},
			),
			want: "1 2",
		},
		{
			name: "the next hop is consulted before the neighbour",
			entries: entriesFor("1.0.0.0/24",
				&domain.BGPRouteEntry{ASPath: "1 2", LocalPref: "100", Origin: "IGP", NextHop: "10.0.0.9", NeighborIP: "10.0.0.1"},
				&domain.BGPRouteEntry{ASPath: "3 4", LocalPref: "100", Origin: "IGP", NextHop: "10.0.0.2", NeighborIP: "10.0.0.8"},
			),
			want: "3 4",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ensureBestAmongEntries(tt.entries)
			best := bestOf(tt.entries)
			if best == nil || best.ASPath != tt.want {
				t.Fatalf("best = %+v, want AS path %q", best, tt.want)
			}
			marked := 0
			for _, e := range tt.entries {
				if e.Best {
					marked++
				}
			}
			if marked != 1 {
				t.Fatalf("expected exactly one best path, got %d", marked)
			}
		})
	}
}

func TestEnsureBestAmongRoutesKeepsAVendorMarker(t *testing.T) {
	routes := []domain.BGPRoute{
		{Prefix: "1.0.0.0/24", ASPath: []uint32{1, 2, 3}, Best: true},
		{Prefix: "1.0.0.0/24", ASPath: []uint32{4}},
	}
	EnsureBestAmongRoutes(routes)
	if !routes[0].Best || routes[1].Best {
		t.Fatalf("the router's own marker must win: %+v", routes)
	}
}

func TestEnsureBestAmongRoutesSelectsWhenUnmarked(t *testing.T) {
	routes := []domain.BGPRoute{
		{Prefix: "1.0.0.0/24", ASPath: []uint32{1, 2, 3}, Origin: "IGP", NextHop: "10.0.0.1"},
		{Prefix: "1.0.0.0/24", ASPath: []uint32{4}, Origin: "IGP", NextHop: "10.0.0.2"},
	}
	EnsureBestAmongRoutes(routes)
	if routes[0].Best || !routes[1].Best {
		t.Fatalf("shorter AS path should win: %+v", routes)
	}

	// Two markers are as good as none — the vendor output was ambiguous.
	ambiguous := []domain.BGPRoute{
		{Prefix: "2.0.0.0/24", ASPath: []uint32{1, 2, 3}, Origin: "IGP", Best: true},
		{Prefix: "2.0.0.0/24", ASPath: []uint32{4}, Origin: "IGP", Best: true},
	}
	EnsureBestAmongRoutes(ambiguous)
	if ambiguous[0].Best || !ambiguous[1].Best {
		t.Fatalf("expected a fresh selection: %+v", ambiguous)
	}
}

func TestOriginRankCoversVendorSpellings(t *testing.T) {
	for _, s := range []string{"IGP", "i", " igp "} {
		if originRank(s) != 0 {
			t.Fatalf("originRank(%q) = %d", s, originRank(s))
		}
	}
	if originRank("EGP") != 1 || originRank("?") != 2 || originRank("nonsense") != 3 {
		t.Fatal("unexpected origin ranking")
	}
}
