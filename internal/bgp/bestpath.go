package bgp

import (
	"strconv"
	"strings"

	"github.com/HopStat/HopStat/internal/domain"
)

// pathPreference is the subset of a path the BGP decision process compares. Everything is
// pre-extracted so the comparison itself is pure ordering, not parsing.
type pathPreference struct {
	localPref uint32
	asPathLen int
	origin    int
	med       uint32
	ageSec    int64
	tieBreak  string
}

// defaultLocalPref is what a path without an explicit LOCAL_PREF is worth, matching the
// value every major implementation assumes.
const defaultLocalPref = 100

// morePreferred reports whether a wins the BGP decision process against b.
//
// The steps we can actually evaluate from a received path: local preference, AS path
// length, origin, and MED. IGP metric and eBGP-over-iBGP need routing state we do not
// have. The remaining steps — oldest path first, then a stable address comparison — are
// what routers fall back on, and are what makes the choice deterministic instead of
// dependent on the order the RIB happened to return.
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
	// An established path is preferred over a fresher one, so the choice does not flap.
	if a.ageSec != b.ageSec {
		return a.ageSec > b.ageSec
	}
	return a.tieBreak < b.tieBreak
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
		localPref: parseUintOr(e.LocalPref, defaultLocalPref),
		asPathLen: len(parseASPath(e.ASPath)),
		origin:    originRank(e.Origin),
		med:       parseUintOr(e.MED, 0),
		ageSec:    e.AgeSeconds,
		tieBreak:  e.NeighborIP,
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
		tieBreak:  r.NextHop,
	}
}
