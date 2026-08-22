package bgp

import (
	"fmt"
	"net"
	"strconv"
	"strings"

	"github.com/HopStat/HopStat/internal/domain"
)

// pathPreference is the subset of a path the BGP decision process compares. Everything is
// pre-extracted so the comparison itself is pure ordering, not parsing.
type pathPreference struct {
	localPref  uint32
	asPathLen  int
	origin     int
	med        uint32
	reflected  bool
	clusterLen int
	ageSec     int64
	nextHop    string
	neighbor   string
}

// defaultLocalPref is what a path without an explicit LOCAL_PREF is worth, matching the
// value every major implementation assumes.
const defaultLocalPref = 100

// morePreferred reports whether a wins the BGP decision process against b.
//
// Evaluated from what a received path actually carries: local preference, AS path length,
// origin, MED, whether the advertiser learned it externally or by reflection, and cluster
// list length. The IGP metric to the next hop is the one real step we can never see. What
// follows that is chosen for determinism rather than fidelity — the same query must not
// flip its answer between refreshes.
func morePreferred(a, b pathPreference) bool {
	if a.localPref != b.localPref {
		return a.localPref > b.localPref
	}
	if a.asPathLen != b.asPathLen {
		return a.asPathLen < b.asPathLen
	}
	if a.origin != b.origin {
		return a.origin < b.origin
	}
	if a.med != b.med {
		return a.med < b.med
	}
	// "Prefer eBGP over iBGP" as far as a receiver can see it: a path carrying an
	// originator or a cluster list reached the advertiser through reflection, while one
	// without either was learned directly from an external peer.
	if a.reflected != b.reflected {
		return !a.reflected
	}
	// RFC 4456: the shorter cluster list wins. Placed ahead of age because the age we hold
	// is our own, and resets with the session.
	if a.clusterLen != b.clusterLen {
		return a.clusterLen < b.clusterLen
	}
	// An established path is preferred over a fresher one, so the choice does not flap.
	// Note this is the age in our own table, which resets whenever the session does — it
	// only separates paths learned at genuinely different times.
	if a.ageSec != b.ageSec {
		return a.ageSec > b.ageSec
	}
	// Everything the protocol lets us compare is equal at this point. BGP carries no
	// "this is my active path" signal — an ADD-PATH identifier is opaque per RFC 7911 —
	// so the remaining order is chosen for determinism, not because the router follows it:
	// the same query must not flip its answer between refreshes.
	if a.nextHop != b.nextHop {
		return a.nextHop < b.nextHop
	}
	return a.neighbor < b.neighbor
}

// addressKey makes IP addresses comparable in numeric order — "9.0.0.1" must sort below
// "10.0.0.1", which a plain string comparison gets backwards.
func addressKey(addr string) string {
	ip := net.ParseIP(strings.TrimSpace(addr))
	if ip == nil {
		return strings.ToLower(strings.TrimSpace(addr))
	}
	if v4 := ip.To4(); v4 != nil {
		return fmt.Sprintf("4:%03d.%03d.%03d.%03d", v4[0], v4[1], v4[2], v4[3])
	}
	return "6:" + ip.String()
}

func originRank(origin string) int {
	switch strings.ToUpper(strings.TrimSpace(origin)) {
	case "IGP", "I":
		return 0
	case "EGP", "E":
		return 1
	case "INCOMPLETE", "?":
		return 2
	default:
		return 3
	}
}

func parseUintOr(raw string, fallback uint32) uint32 {
	v, err := strconv.ParseUint(strings.TrimSpace(raw), 10, 32)
	if err != nil {
		return fallback
	}
	return uint32(v)
}

func entryPreference(e *domain.BGPRouteEntry) pathPreference {
	return pathPreference{
		localPref:  parseUintOr(e.LocalPref, defaultLocalPref),
		asPathLen:  len(parseASPath(e.ASPath)),
		origin:     originRank(e.Origin),
		med:        parseUintOr(e.MED, 0),
		reflected:  e.OriginatorID != "" || len(e.ClusterList) > 0,
		clusterLen: len(e.ClusterList),
		ageSec:     e.AgeSeconds,
		nextHop:    addressKey(e.NextHop),
		neighbor:   addressKey(e.NeighborIP),
	}
}

func routePreference(r *domain.BGPRoute) pathPreference {
	localPref := r.LocalPref
	if localPref == 0 {
		// Parsed output usually omits local preference rather than advertising zero.
		localPref = defaultLocalPref
	}
	return pathPreference{
		localPref: localPref,
		asPathLen: len(r.ASPath),
		origin:    originRank(r.Origin),
		med:       r.MED,
		nextHop:   addressKey(r.NextHop),
	}
}
