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

// The case that exposed this: two equally preferred paths for 8.8.8.0/24 where the router
// picks the older one, but the RIB returned the newer one first.
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
