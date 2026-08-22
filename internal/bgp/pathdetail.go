package bgp

import (
	"context"
	"fmt"
	"net"
	"strings"

	api "github.com/osrg/gobgp/v3/api"
	"github.com/osrg/gobgp/v3/pkg/apiutil"

	"github.com/HopStat/HopStat/internal/domain"
	"github.com/HopStat/HopStat/internal/target"
)

// LookupPathDetails returns every path for a prefix without the interpretation the normal
// result applies: no best-path selection, no default-route synthesis, no local AS
// prepending. It answers "what did the neighbour actually send us".
func (m *SessionManager) LookupPathDetails(
	ctx context.Context,
	nodeID int64,
	prefix string,
	nodeNameForNeighbor func(string) string,
) ([]domain.BGPPathDetail, error) {
	if m == nil || m.bgpServer == nil {
		return nil, fmt.Errorf("bgp server not started")
	}

	normalized, err := target.NormalizeBGPLookup(ctx, prefix)
	if lookupNormalizeHook != nil {
		normalized, err = lookupNormalizeHook(ctx, prefix)
	}
	if err != nil {
		return nil, err
	}

	family, err := familyForPrefix(normalized)
	if err != nil {
		return nil, err
	}

	var nodeIPs map[string]struct{}
	if nodeID > 0 {
		nodeIPs = m.neighborIPsForNode(nodeID)
	}

	details := []domain.BGPPathDetail{}
	list := m.bgpServer.ListPath
	if lookupListPathHook != nil {
		list = lookupListPathHook
	}
	err = list(ctx, &api.ListPathRequest{
		TableType: api.TableType_GLOBAL,
		Family:    family,
		Prefixes:  []*api.TableLookupPrefix{{Prefix: normalized}},
	}, func(dst *api.Destination) {
		for _, path := range dst.Paths {
			if nodeID > 0 {
				if _, ok := nodeIPs[path.NeighborIp]; !ok {
					continue
				}
			}
			details = append(details, m.pathToDetail(path, dst.Prefix, nodeNameForNeighbor))
		}
	})
	if err != nil {
		return nil, fmt.Errorf("bgp path lookup: %w", err)
	}
	return details, nil
}

func familyForPrefix(prefix string) (*api.Family, error) {
	var ip net.IP
	var err error
	if strings.Contains(prefix, "/") {
		ip, _, err = net.ParseCIDR(prefix)
	} else {
		ip = net.ParseIP(prefix)
	}
	if ip == nil || err != nil {
		return nil, fmt.Errorf("invalid prefix: %s", prefix)
	}
	if ip.To4() == nil {
		return &api.Family{Afi: api.Family_AFI_IP6, Safi: api.Family_SAFI_UNICAST}, nil
	}
	return &api.Family{Afi: api.Family_AFI_IP, Safi: api.Family_SAFI_UNICAST}, nil
}

func (m *SessionManager) pathToDetail(path *api.Path, prefix string, nodeNameForNeighbor func(string) string) domain.BGPPathDetail {
	entry := m.pathToRouteEntry(path, prefix)

	detail := domain.BGPPathDetail{
		Prefix:      entry.Prefix,
		NeighborIP:  entry.NeighborIP,
		Identifier:  path.GetIdentifier(),
		SourceASN:   entry.SourceASN,
		Best:        entry.Best,
		Age:         entry.Age,
		NextHop:     entry.NextHop,
		ASPath:      entry.ASPath,
		Origin:      entry.Origin,
		LocalPref:   entry.LocalPref,
		MED:         entry.MED,
		Communities: entry.Communities,
		Attributes:  []string{},
	}
	if nodeNameForNeighbor != nil {
		detail.NodeName = nodeNameForNeighbor(entry.NeighborIP)
	}

	// Every attribute verbatim, including the ones the normal result never reads —
	// originator, cluster list, aggregator — since any of them may explain a choice.
	if attrs, err := apiutil.GetNativePathAttributes(path); err == nil {
		for _, attr := range attrs {
			detail.Attributes = append(detail.Attributes, strings.TrimSpace(attr.String()))
		}
	}

	return detail
}
