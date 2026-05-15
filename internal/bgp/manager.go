package bgp

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"strconv"
	"sync"
	"time"

	api "github.com/osrg/gobgp/v3/api"
	"github.com/osrg/gobgp/v3/pkg/apiutil"
	"github.com/osrg/gobgp/v3/pkg/packet/bgp"
	"github.com/osrg/gobgp/v3/pkg/server"

	"github.com/HopStat/HopStat/internal/config"
	"github.com/HopStat/HopStat/internal/domain"
)

type SessionManager struct {
	bgpServer *server.BgpServer
	cfg       config.BGPConfig

	mu        sync.RWMutex
	neighbors map[int64]*neighborEntry // keyed by domain BGPNeighbor.ID
	nodeMap   map[int64]int64          // nodeID → neighbor ID
	states    map[int64]domain.BGPSessionState

	cancelWatch context.CancelFunc
}

type neighborEntry struct {
	neighbor       *domain.BGPNeighbor
	neighborIP     string
	ipv6NeighborIP string // non-empty when an IPv6 session is configured
}

func NewSessionManager(cfg config.BGPConfig) *SessionManager {
	return &SessionManager{
		cfg:       cfg,
		neighbors: make(map[int64]*neighborEntry),
		nodeMap:   make(map[int64]int64),
		states:    make(map[int64]domain.BGPSessionState),
	}
}

func (m *SessionManager) Start(ctx context.Context) error {
	m.bgpServer = server.NewBgpServer()

	routerID := m.cfg.RouterID
	if routerID == "" {
		routerID = "0.0.0.0"
	}

	listenPort := int32(0)
	if m.cfg.ListenPort > 0 {
		listenPort = int32(m.cfg.ListenPort)
	}

	if err := m.bgpServer.StartBgp(ctx, &api.StartBgpRequest{
		Global: &api.Global{
			Asn:         0,
			RouterId:    routerID,
			ListenPort:  listenPort,
			ListenAddresses: []string{"0.0.0.0"},
		},
	}); err != nil {
		return fmt.Errorf("start bgp server: %w", err)
	}

	go func() {
		m.bgpServer.Serve()
		slog.Error("bgp server exited unexpectedly")
	}()

	watchCtx, cancel := context.WithCancel(ctx)
	m.cancelWatch = cancel
	go m.watchPeers(watchCtx)

	slog.Info("bgp session manager started", "router_id", routerID, "listen_port", listenPort)
	return nil
}

func (m *SessionManager) Stop() {
	if m.cancelWatch != nil {
		m.cancelWatch()
	}
	if m.bgpServer != nil {
		m.bgpServer.Stop()
	}
	slog.Info("bgp session manager stopped")
}

func (m *SessionManager) AddNeighbor(n *domain.BGPNeighbor) error {
	if m.bgpServer == nil {
		return fmt.Errorf("bgp server not started")
	}

	peer := m.buildPeerConfig(n, n.PeeringIP, n.NeighborIP)
	if err := m.bgpServer.AddPeer(context.Background(), &api.AddPeerRequest{Peer: peer}); err != nil {
		return fmt.Errorf("add bgp peer %s: %w", n.NeighborIP, err)
	}

	if n.IPv6NeighborIP != "" {
		peer6 := m.buildPeerConfig(n, n.IPv6PeeringIP, n.IPv6NeighborIP)
		if err := m.bgpServer.AddPeer(context.Background(), &api.AddPeerRequest{Peer: peer6}); err != nil {
			slog.Warn("bgp ipv6 peer add failed", "id", n.ID, "neighbor_ip", n.IPv6NeighborIP, "err", err)
		} else {
			slog.Info("bgp ipv6 neighbor added", "id", n.ID, "neighbor_ip", n.IPv6NeighborIP)
		}
	}

	m.mu.Lock()
	m.neighbors[n.ID] = &neighborEntry{neighbor: n, neighborIP: n.NeighborIP, ipv6NeighborIP: n.IPv6NeighborIP}
	m.nodeMap[n.NodeID] = n.ID
	m.states[n.ID] = domain.BGPSessionIdle
	m.mu.Unlock()

	slog.Info("bgp neighbor added", "id", n.ID, "neighbor_ip", n.NeighborIP, "remote_as", n.RemoteAS)
	return nil
}

func (m *SessionManager) RemoveNeighbor(id int64) error {
	if m.bgpServer == nil {
		return fmt.Errorf("bgp server not started")
	}

	m.mu.RLock()
	entry, ok := m.neighbors[id]
	m.mu.RUnlock()
	if !ok {
		return nil
	}

	if err := m.bgpServer.DeletePeer(context.Background(), &api.DeletePeerRequest{Address: entry.neighborIP}); err != nil {
		return fmt.Errorf("remove bgp peer %s: %w", entry.neighborIP, err)
	}
	if entry.ipv6NeighborIP != "" {
		if err := m.bgpServer.DeletePeer(context.Background(), &api.DeletePeerRequest{Address: entry.ipv6NeighborIP}); err != nil {
			slog.Warn("bgp ipv6 peer remove failed", "neighbor_ip", entry.ipv6NeighborIP, "err", err)
		}
	}

	m.mu.Lock()
	delete(m.neighbors, id)
	delete(m.states, id)
	for nodeID, nID := range m.nodeMap {
		if nID == id {
			delete(m.nodeMap, nodeID)
		}
	}
	m.mu.Unlock()

	slog.Info("bgp neighbor removed", "id", id)
	return nil
}

func (m *SessionManager) UpdateNeighbor(n *domain.BGPNeighbor) error {
	// Get old entry for rollback
	m.mu.RLock()
	oldEntry, hadOld := m.neighbors[n.ID]
	m.mu.RUnlock()

	// Add new peer first, before removing old one
	if err := m.AddNeighbor(n); err != nil {
		return fmt.Errorf("add updated neighbor: %w", err)
	}

	// Only remove old peer if it exists and has a different address
	if hadOld && oldEntry.neighborIP != n.NeighborIP {
		if err := m.bgpServer.DeletePeer(context.Background(), &api.DeletePeerRequest{Address: oldEntry.neighborIP}); err != nil {
			slog.Warn("bgp old peer removal failed during update", "neighbor_ip", oldEntry.neighborIP, "err", err)
		}
	}
	if hadOld && oldEntry.ipv6NeighborIP != "" && oldEntry.ipv6NeighborIP != n.IPv6NeighborIP {
		if err := m.bgpServer.DeletePeer(context.Background(), &api.DeletePeerRequest{Address: oldEntry.ipv6NeighborIP}); err != nil {
			slog.Warn("bgp old ipv6 peer removal failed during update", "neighbor_ip", oldEntry.ipv6NeighborIP, "err", err)
		}
	}
	return nil
}

func (m *SessionManager) GetStatus(id int64) domain.BGPSessionState {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.states[id]
}

func (m *SessionManager) GetAllStatuses() map[int64]domain.BGPSessionState {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make(map[int64]domain.BGPSessionState, len(m.states))
	for k, v := range m.states {
		out[k] = v
	}
	return out
}

func (m *SessionManager) GetSessionStatuses() []*domain.BGPSessionStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var out []*domain.BGPSessionStatus
	for id, entry := range m.neighbors {
		state := m.states[id]
		status := &domain.BGPSessionStatus{
			NeighborID: id,
			NodeID:     entry.neighbor.NodeID,
			State:      state,
			RemoteAS:   entry.neighbor.RemoteAS,
			NeighborIP: entry.neighbor.NeighborIP,
		}
		out = append(out, status)
	}
	return out
}

func (m *SessionManager) HasActiveSession(nodeID int64) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	nID, ok := m.nodeMap[nodeID]
	if !ok {
		return false
	}
	return m.states[nID] == domain.BGPSessionEstablished
}

func (m *SessionManager) LookupRoute(ctx context.Context, nodeID int64, prefix string) ([]*domain.BGPRouteEntry, error) {
	if m.bgpServer == nil {
		return nil, fmt.Errorf("bgp server not started")
	}

	ip := net.ParseIP(prefix)
	if ip == nil {
		return nil, fmt.Errorf("invalid prefix: %s", prefix)
	}

	family := &api.Family{Afi: api.Family_AFI_IP, Safi: api.Family_SAFI_UNICAST}
	if ip.To4() == nil {
		family = &api.Family{Afi: api.Family_AFI_IP6, Safi: api.Family_SAFI_UNICAST}
	}

	var results []*domain.BGPRouteEntry

	err := m.bgpServer.ListPath(ctx, &api.ListPathRequest{
		TableType: api.TableType_GLOBAL,
		Family:    family,
		Prefixes: []*api.TableLookupPrefix{
			{Prefix: prefix},
		},
	}, func(dst *api.Destination) {
		for _, path := range dst.Paths {
			entry := m.pathToRouteEntry(path, dst.Prefix)
			results = append(results, entry)
		}
	})
	if err != nil {
		return nil, fmt.Errorf("bgp route lookup: %w", err)
	}
	return results, nil
}

const maxASPathResults = 1000

func (m *SessionManager) LookupASPath(ctx context.Context, nodeID int64, asn uint32) ([]*domain.BGPRouteEntry, error) {
	if m.bgpServer == nil {
		return nil, fmt.Errorf("bgp server not started")
	}

	var (
		results []*domain.BGPRouteEntry
		cbMu    sync.Mutex
	)

	for _, family := range []*api.Family{
		{Afi: api.Family_AFI_IP, Safi: api.Family_SAFI_UNICAST},
		{Afi: api.Family_AFI_IP6, Safi: api.Family_SAFI_UNICAST},
	} {
		cbMu.Lock()
		atMax := len(results) >= maxASPathResults
		cbMu.Unlock()
		if atMax {
			break
		}
		err := m.bgpServer.ListPath(ctx, &api.ListPathRequest{
			TableType: api.TableType_GLOBAL,
			Family:    family,
		}, func(dst *api.Destination) {
			for _, path := range dst.Paths {
				cbMu.Lock()
				if len(results) >= maxASPathResults {
					cbMu.Unlock()
					return
				}
				if pathContainsASN(path, asn) {
					entry := m.pathToRouteEntry(path, dst.Prefix)
					results = append(results, entry)
				}
				cbMu.Unlock()
			}
		})
		if err != nil {
			slog.Warn("bgp as-path lookup error", "family", family, "err", err)
		}
	}
	return results, nil
}

func (m *SessionManager) LoadNeighbors(ctx context.Context, repo domain.BGPNeighborRepository) error {
	neighbors, err := repo.GetAll(ctx)
	if err != nil {
		return fmt.Errorf("load bgp neighbors: %w", err)
	}
	for _, n := range neighbors {
		if err := m.AddNeighbor(n); err != nil {
			slog.Warn("failed to add bgp neighbor on load", "id", n.ID, "err", err)
		}
	}
	return nil
}

func (m *SessionManager) buildPeerConfig(n *domain.BGPNeighbor, localAddr, neighborAddr string) *api.Peer {
	peer := &api.Peer{
		Conf: &api.PeerConf{
			LocalAsn:        n.LocalAS,
			NeighborAddress: neighborAddr,
			PeerAsn:         n.RemoteAS,
		},
		Transport: &api.Transport{
			LocalAddress: localAddr,
			PassiveMode:  true,
		},
		Timers: &api.Timers{
			Config: &api.TimersConfig{
				ConnectRetry: 10,
				HoldTime:     90,
				KeepaliveInterval: 30,
			},
		},
		AfiSafis: []*api.AfiSafi{
			{
				Config: &api.AfiSafiConfig{
					Family:  &api.Family{Afi: api.Family_AFI_IP, Safi: api.Family_SAFI_UNICAST},
					Enabled: true,
				},
			},
			{
				Config: &api.AfiSafiConfig{
					Family:  &api.Family{Afi: api.Family_AFI_IP6, Safi: api.Family_SAFI_UNICAST},
					Enabled: true,
				},
			},
		},
	}

	if n.Multihop {
		peer.EbgpMultihop = &api.EbgpMultihop{
			Enabled:     true,
			MultihopTtl: 5,
		}
	}

	return peer
}

func (m *SessionManager) watchPeers(ctx context.Context) {
	err := m.bgpServer.WatchEvent(ctx, &api.WatchEventRequest{
		Peer: &api.WatchEventRequest_Peer{},
	}, func(ev *api.WatchEventResponse) {
		peerEv := ev.GetPeer()
		if peerEv == nil {
			return
		}
		peer := peerEv.GetPeer()
		if peer == nil || peer.State == nil {
			return
		}

		neighborAddr := peer.State.NeighborAddress
		state := apiStateToDomain(peer.State.SessionState)

		m.mu.RLock()
		for id, entry := range m.neighbors {
			if entry.neighborIP == neighborAddr || entry.ipv6NeighborIP == neighborAddr {
				m.mu.RUnlock()
				m.mu.Lock()
				m.states[id] = state
				m.mu.Unlock()
				slog.Debug("bgp peer state changed", "neighbor", neighborAddr, "state", state)
				return
			}
		}
		m.mu.RUnlock()
	})
	if err != nil && ctx.Err() == nil {
		slog.Error("bgp peer watch error", "err", err)
	}
}

func (m *SessionManager) pathToRouteEntry(path *api.Path, prefix string) *domain.BGPRouteEntry {
	entry := &domain.BGPRouteEntry{
		Prefix:     prefix,
		NeighborIP: path.NeighborIp,
		SourceASN:  path.SourceAsn,
		Best:       path.Best,
		Age:        time.Since(path.GetAge().AsTime()).Truncate(time.Second).String(),
	}

	attrs, err := apiutil.GetNativePathAttributes(path)
	if err == nil {
		for _, attr := range attrs {
			switch a := attr.(type) {
			case *bgp.PathAttributeAsPath:
				entry.ASPath = asPathToString(a)
			case *bgp.PathAttributeNextHop:
				if a.Value != nil {
					entry.NextHop = a.Value.String()
				}
			case *bgp.PathAttributeMpReachNLRI:
				if a.Nexthop != nil {
					entry.NextHop = a.Nexthop.String()
				}
			case *bgp.PathAttributeOrigin:
				entry.Origin = originToString(a)
			case *bgp.PathAttributeLocalPref:
				entry.LocalPref = strconv.FormatUint(uint64(a.Value), 10)
			case *bgp.PathAttributeMultiExitDisc:
				entry.MED = strconv.FormatUint(uint64(a.Value), 10)
			}
		}
	}

	return entry
}

func pathContainsASN(path *api.Path, asn uint32) bool {
	attrs, err := apiutil.GetNativePathAttributes(path)
	if err != nil {
		return false
	}
	for _, attr := range attrs {
		if asp, ok := attr.(*bgp.PathAttributeAsPath); ok {
			for _, seg := range asp.Value {
				for _, as := range seg.GetAS() {
					if as == asn {
						return true
					}
				}
			}
		}
	}
	return false
}

func apiStateToDomain(s api.PeerState_SessionState) domain.BGPSessionState {
	switch s {
	case api.PeerState_IDLE:
		return domain.BGPSessionIdle
	case api.PeerState_CONNECT:
		return domain.BGPSessionConnect
	case api.PeerState_ACTIVE:
		return domain.BGPSessionActive
	case api.PeerState_OPENSENT:
		return domain.BGPSessionOpenSent
	case api.PeerState_OPENCONFIRM:
		return domain.BGPSessionOpenConfirm
	case api.PeerState_ESTABLISHED:
		return domain.BGPSessionEstablished
	default:
		return domain.BGPSessionIdle
	}
}

func asPathToString(attr *bgp.PathAttributeAsPath) string {
	var segments []string
	for _, seg := range attr.Value {
		var asns []string
		for _, as := range seg.GetAS() {
			asns = append(asns, strconv.FormatUint(uint64(as), 10))
		}
		segments = append(segments, fmt.Sprintf("%v", asns))
	}
	return fmt.Sprintf("%v", segments)
}

func originToString(attr *bgp.PathAttributeOrigin) string {
	switch attr.Value {
	case bgp.BGP_ORIGIN_ATTR_TYPE_IGP:
		return "IGP"
	case bgp.BGP_ORIGIN_ATTR_TYPE_EGP:
		return "EGP"
	case bgp.BGP_ORIGIN_ATTR_TYPE_INCOMPLETE:
		return "incomplete"
	default:
		return strconv.FormatInt(int64(attr.Value), 10)
	}
}
