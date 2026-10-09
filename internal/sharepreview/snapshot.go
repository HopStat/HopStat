// Package sharepreview gives a shared query link (/traceroute/1.1.1.1, /sofia/bgp/8.8.8.0/24)
// a link preview that says what the query found.
//
// Link-preview crawlers — WhatsApp, Slack, Telegram, X, LinkedIn — do not run JavaScript,
// so the meta tags the SPA writes at runtime never reach them. The server writes them into
// index.html instead. A link carries only the node, command and target, not a query id,
// and live results expire within minutes, so the outcome of the last finished query for
// that node, command and target is kept as a small snapshot to describe it from.
package sharepreview

import (
	"strconv"
	"strings"
	"time"

	"github.com/HopStat/HopStat/internal/domain"
)

// Snapshot is what a preview can say about a finished query. Only the fields of its
// command are set; it is stored as JSON and rendered in the site's language on request.
type Snapshot struct {
	Command string    `json:"command"`
	At      time.Time `json:"at"`

	// ping
	Sent    int     `json:"sent,omitempty"`
	Recv    int     `json:"recv,omitempty"`
	LossPct float64 `json:"loss_pct,omitempty"`
	AvgRTT  float64 `json:"avg_rtt,omitempty"`
	MinRTT  float64 `json:"min_rtt,omitempty"`
	MaxRTT  float64 `json:"max_rtt,omitempty"`

	// traceroute
	Hops    int     `json:"hops,omitempty"`
	Reached bool    `json:"reached,omitempty"`
	LastRTT float64 `json:"last_rtt,omitempty"`
	LastAS  string  `json:"last_as,omitempty"`

	// bgp_route
	NoRoute  bool     `json:"no_route,omitempty"`
	Prefix   string   `json:"prefix,omitempty"`
	ASPath   []uint32 `json:"as_path,omitempty"`
	OriginAS string   `json:"origin_as,omitempty"`
}

// FromResult condenses a finished query. resolvedTarget is the address the query actually
// ran against, which is how a traceroute tells that its last hop is the target. It reports
// false for anything not worth previewing: a failed query, or one with no parsed output.
func FromResult(command, resolvedTarget string, result *domain.QueryResult, at time.Time) (*Snapshot, bool) {
	if result == nil || result.Status != domain.StatusDone || result.Parsed == nil {
		return nil, false
	}
	snap := &Snapshot{Command: command, At: at.UTC()}

	switch p := result.Parsed.(type) {
	case *domain.PingResult:
		if p == nil || p.PacketsSent == 0 {
			return nil, false
		}
		snap.Sent, snap.Recv, snap.LossPct = p.PacketsSent, p.PacketsRecv, p.PacketLoss
		snap.AvgRTT, snap.MinRTT, snap.MaxRTT = p.AvgRTT, p.MinRTT, p.MaxRTT

	case *domain.TracerouteResult:
		if p == nil || len(p.Hops) == 0 {
			return nil, false
		}
		snap.Hops = p.Hops[len(p.Hops)-1].Number
		if snap.Hops == 0 {
			snap.Hops = len(p.Hops)
		}
		// The last hop that answered is where the path ends, whether or not it is the target.
		for i := len(p.Hops) - 1; i >= 0; i-- {
			hop := p.Hops[i]
			if hop.IP == "" || len(hop.RTT) == 0 {
				continue
			}
			snap.Reached = resolvedTarget != "" && hop.IP == resolvedTarget
			snap.LastRTT = average(hop.RTT)
			snap.LastAS = asLabel(hop.ASInfo, 0)
			break
		}

	case *domain.BGPResult:
		if p == nil || len(p.Routes) == 0 {
			snap.NoRoute = true
			break
		}
		route := p.Routes[0]
		for _, r := range p.Routes {
			if r.Best {
				route = r
				break
			}
		}
		snap.Prefix = route.Prefix
		snap.ASPath = append([]uint32(nil), route.ASPath...)
		var origin uint32
		if n := len(route.ASPath); n > 0 {
			origin = route.ASPath[n-1]
		}
		if p.TargetAS != nil && (origin == 0 || p.TargetAS.ASN == origin) {
			snap.OriginAS = asLabel(p.TargetAS, origin)
		} else if origin != 0 {
			snap.OriginAS = "AS" + strconv.FormatUint(uint64(origin), 10)
		}

	default:
		return nil, false
	}
	return snap, true
}

// average is only called with at least one value.
func average(values []float64) float64 {
	var sum float64
	for _, v := range values {
		sum += v
	}
	return sum / float64(len(values))
}

// asLabel reads "AS13335 CLOUDFLARENET": the number, then the short operator name.
func asLabel(info *domain.ASInfo, fallback uint32) string {
	asn := fallback
	name := ""
	if info != nil {
		if info.ASN != 0 {
			asn = info.ASN
		}
		name = strings.TrimSpace(info.ShortName)
		if name == "" {
			name = strings.TrimSpace(info.OrgName)
		}
	}
	if asn == 0 {
		return name
	}
	label := "AS" + strconv.FormatUint(uint64(asn), 10)
	if name != "" {
		label += " " + name
	}
	return label
}
