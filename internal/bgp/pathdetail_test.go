package bgp

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	api "github.com/osrg/gobgp/v3/api"
	"github.com/osrg/gobgp/v3/pkg/apiutil"
	"github.com/osrg/gobgp/v3/pkg/packet/bgp"

	"github.com/HopStat/HopStat/internal/domain"
)

// serveAddPathRIB mimics one neighbour advertising several paths for a prefix, each with
// its own ADD-PATH identifier.
func serveAddPathRIB(t *testing.T, neighborIP string, paths [][]uint32) {
	t.Helper()
	old := lookupListPathHook
	lookupListPathHook = func(_ context.Context, _ *api.ListPathRequest, fn func(*api.Destination)) error {
		var out []*api.Path
		for i, asPath := range paths {
			attrs := []bgp.PathAttributeInterface{
				bgp.NewPathAttributeOrigin(bgp.BGP_ORIGIN_ATTR_TYPE_IGP),
				bgp.NewPathAttributeNextHop(fmt.Sprintf("10.0.%d.1", i)),
				bgp.NewPathAttributeAsPath([]bgp.AsPathParamInterface{bgp.NewAs4PathParam(2, asPath)}),
				bgp.NewPathAttributeLocalPref(100),
			}
			p, err := apiutil.NewPath(bgp.NewIPAddrPrefix(24, "8.8.8.0"), false, attrs, time.Now())
			if err != nil {
				t.Fatalf("NewPath: %v", err)
			}
			p.NeighborIp = neighborIP
			p.Identifier = uint32(i + 1)
			p.Best = i == 0
			out = append(out, p)
		}
		fn(&api.Destination{Prefix: "8.8.8.0/24", Paths: out})
		return nil
	}
	t.Cleanup(func() { lookupListPathHook = old })
}

func TestLookupPathDetailsReportsWhatArrived(t *testing.T) {
	mgr, ctx := mapTestManager(t, 9121)
	serveAddPathRIB(t, "10.0.0.1", [][]uint32{{204457, 15169}, {44901, 15169}})

	details, err := mgr.LookupPathDetails(ctx, 0, "8.8.8.0/24", func(ip string) string { return "ESENYURT" })
	if err != nil {
		t.Fatalf("LookupPathDetails: %v", err)
	}
	if len(details) != 2 {
		t.Fatalf("details = %+v", details)
	}

	// The ADD-PATH identifier is the whole point of the tool: it is the only per-path
	// signal the normal result throws away.
	if details[0].Identifier != 1 || details[1].Identifier != 2 {
		t.Fatalf("identifiers = %d, %d", details[0].Identifier, details[1].Identifier)
	}
	// The AS path is kept in the speaker's own notation — this is a raw view.
	if !strings.Contains(details[0].ASPath, "204457") || !strings.Contains(details[0].ASPath, "15169") {
		t.Fatalf("as path = %q", details[0].ASPath)
	}
	if details[0].NodeName != "ESENYURT" {
		t.Fatalf("node name = %q", details[0].NodeName)
	}
	// Attributes are dumped verbatim, including ones no other view reads.
	joined := strings.Join(details[0].Attributes, " ")
	if !strings.Contains(joined, "204457") || !strings.Contains(joined, "LocalPref") {
		t.Fatalf("attributes = %v", details[0].Attributes)
	}
	// No best-path selection is applied here — the flag is whatever GoBGP said.
	if !details[0].Best || details[1].Best {
		t.Fatalf("best flags = %v, %v", details[0].Best, details[1].Best)
	}
}

func TestLookupPathDetailsFiltersByNode(t *testing.T) {
	mgr, ctx := mapTestManager(t, 9121)
	serveAddPathRIB(t, "10.0.0.2", [][]uint32{{44901, 15169}})

	own, err := mgr.LookupPathDetails(ctx, 2, "8.8.8.0/24", nil)
	if err != nil || len(own) != 1 {
		t.Fatalf("own = %+v, err = %v", own, err)
	}
	other, err := mgr.LookupPathDetails(ctx, 1, "8.8.8.0/24", nil)
	if err != nil {
		t.Fatalf("other: %v", err)
	}
	if len(other) != 0 {
		t.Fatalf("another node's session must not appear: %+v", other)
	}
}

func TestLookupPathDetailsRejectsUnusableInput(t *testing.T) {
	var nilMgr *SessionManager
	if _, err := nilMgr.LookupPathDetails(context.Background(), 0, "8.8.8.0/24", nil); err == nil {
		t.Fatal("expected an error without a running speaker")
	}

	mgr, ctx := mapTestManager(t, 9121)
	if _, err := mgr.LookupPathDetails(ctx, 0, "   ", nil); err == nil {
		t.Fatal("expected an error for an empty prefix")
	}

	oldNorm := lookupNormalizeHook
	lookupNormalizeHook = func(context.Context, string) (string, error) { return "not-a-prefix", nil }
	if _, err := mgr.LookupPathDetails(ctx, 0, "8.8.8.0/24", nil); err == nil {
		t.Fatal("expected an error when the normalized target is unusable")
	}
	lookupNormalizeHook = oldNorm

	old := lookupListPathHook
	lookupListPathHook = func(context.Context, *api.ListPathRequest, func(*api.Destination)) error {
		return fmt.Errorf("rib unavailable")
	}
	defer func() { lookupListPathHook = old }()
	if _, err := mgr.LookupPathDetails(ctx, 0, "8.8.8.0/24", nil); err == nil {
		t.Fatal("expected the lookup error to surface")
	}
}

func TestFamilyForPrefix(t *testing.T) {
	v4, err := familyForPrefix("8.8.8.0/24")
	if err != nil || v4.Afi != api.Family_AFI_IP {
		t.Fatalf("v4 = %+v, err = %v", v4, err)
	}
	v6, err := familyForPrefix("2001:db8::1")
	if err != nil || v6.Afi != api.Family_AFI_IP6 {
		t.Fatalf("v6 = %+v, err = %v", v6, err)
	}
	if _, err := familyForPrefix("nonsense"); err == nil {
		t.Fatal("expected an error for an unparseable prefix")
	}
	if _, err := familyForPrefix("8.8.8.0/99"); err == nil {
		t.Fatal("expected an error for an invalid mask")
	}
}

var _ = domain.BGPPathDetail{}
