package bgp

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	api "github.com/osrg/gobgp/v3/api"
	"github.com/osrg/gobgp/v3/pkg/apiutil"
	"github.com/osrg/gobgp/v3/pkg/packet/bgp"
	"github.com/osrg/gobgp/v3/pkg/server"

	"github.com/HopStat/HopStat/internal/config"
	"github.com/HopStat/HopStat/internal/domain"
	"github.com/HopStat/HopStat/internal/target"
)

type SessionManager struct {
	bgpServer *server.BgpServer
	cfg       config.BGPConfig

	mu            sync.RWMutex
	neighbors     map[int64]*neighborEntry // keyed by domain BGPNeighbor.ID
	nodeNeighbors map[int64]map[int64]struct{} // nodeID → neighbor IDs
	states        map[int64]domain.BGPSessionState
	stateSince    map[int64]time.Time
	events        *eventLog
	globalAS      uint32

	prefixCountMu sync.RWMutex
	prefixCounts  map[int64]prefixCountEntry

	cancelWatch context.CancelFunc
}

type neighborEntry struct {
	neighbor       *domain.BGPNeighbor
	neighborIP     string
	ipv6NeighborIP string // non-empty when an IPv6 session is configured
}

func NewSessionManager(cfg config.BGPConfig) *SessionManager {
	return &SessionManager{
		cfg:           cfg,
		neighbors:     make(map[int64]*neighborEntry),
		nodeNeighbors: make(map[int64]map[int64]struct{}),
		states:        make(map[int64]domain.BGPSessionState),
		events:        newEventLog(),
		prefixCounts:  make(map[int64]prefixCountEntry),
	}
}

func (m *SessionManager) recordEvent(neighborID int64, level, message, address string) {
	if m.events == nil {
		return
	}
	m.events.add(neighborID, level, message, address)
}

func (m *SessionManager) GetNeighborLogs(neighborID int64, limit int) []LogEntry {
	if m.events == nil {
		return []LogEntry{}
	}
	return m.events.list(neighborID, limit)
}

func (m *SessionManager) neighborEntry(id int64) (*neighborEntry, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	entry, ok := m.neighbors[id]
	if !ok {
		return nil, fmt.Errorf("neighbor %d is not active in the bgp session manager", id)
	}
	return entry, nil
}

func (m *SessionManager) peerAddresses(entry *neighborEntry) []string {
	var addrs []string
	if entry.neighborIP != "" {
		addrs = append(addrs, entry.neighborIP)
	}
	if entry.ipv6NeighborIP != "" {
		addrs = append(addrs, entry.ipv6NeighborIP)
	}
	return addrs
}

func (m *SessionManager) StopNeighbor(id int64) error {
	if m.bgpServer == nil {
		return fmt.Errorf("bgp server not started")
	}
	entry, err := m.neighborEntry(id)
	if err != nil {
		return err
	}

	var firstErr error
	for _, addr := range m.peerAddresses(entry) {
		if err := m.bgpServer.DisablePeer(context.Background(), &api.DisablePeerRequest{Address: addr}); err != nil {
			m.recordEvent(id, "error", fmt.Sprintf("stop failed: %v", err), addr)
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		m.recordEvent(id, "info", "session stopped (admin down)", addr)
	}
	return firstErr
}

func (m *SessionManager) RestartNeighbor(id int64) error {
	if m.bgpServer == nil {
		return fmt.Errorf("bgp server not started")
	}
	entry, err := m.neighborEntry(id)
	if err != nil {
		return err
	}

	var firstErr error
	for _, addr := range m.peerAddresses(entry) {
		enable := m.bgpServer.EnablePeer
		if enablePeerHook != nil {
			enable = enablePeerHook
		}
		if err := enable(context.Background(), &api.EnablePeerRequest{Address: addr}); err != nil {
			m.recordEvent(id, "warn", fmt.Sprintf("enable before restart failed: %v", err), addr)
		}
		reset := m.bgpServer.ResetPeer
		if resetPeerHook != nil {
			reset = resetPeerHook
		}
		if err := reset(context.Background(), &api.ResetPeerRequest{
			Address: addr,
			Soft:    false,
		}); err != nil {
			m.recordEvent(id, "error", fmt.Sprintf("restart failed: %v", err), addr)
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		m.recordEvent(id, "info", "session restarted (reset sent)", addr)
	}
	return firstErr
}

func (m *SessionManager) Start(ctx context.Context) error {
	m.bgpServer = server.NewBgpServer()

	// Serve must run before StartBgp: mgmt APIs block on mgmtCh until the event loop is active.
	go func() {
		m.bgpServer.Serve()
		slog.Error("bgp server exited unexpectedly")
	}()

	watchCtx, cancel := context.WithCancel(ctx)
	m.cancelWatch = cancel
	go m.watchPeers(watchCtx)
	go m.watchStuckSessions(watchCtx)

	if m.cfg.LocalAS > 0 {
		if err := m.ensureBGPGlobalStarted(ctx, m.cfg.LocalAS, m.cfg.RouterID); err != nil {
			return err
		}
		slog.Info("bgp session manager started", "local_as", m.globalAS, "router_id", m.effectiveRouterID(""))
		return nil
	}

	slog.Warn("bgp.local_as is not set — configure bgp.local_as in config.yaml and restart to enable BGP sessions")
	return nil
}

func (m *SessionManager) BGPConfig() config.BGPConfig {
	return m.cfg
}

func (m *SessionManager) IsReady() bool {
	return m.bgpServer != nil && m.globalAS > 0
}

func (m *SessionManager) effectiveRouterID(preferred string) string {
	routerID := strings.TrimSpace(preferred)
	if routerID == "" || routerID == "0.0.0.0" {
		routerID = strings.TrimSpace(m.cfg.RouterID)
	}
	if routerID == "" || routerID == "0.0.0.0" {
		routerID = "127.0.0.1"
	}
	return routerID
}

func (m *SessionManager) ensureBGPGlobalStarted(ctx context.Context, localAS uint32, routerID string) error {
	if m.bgpServer == nil {
		return fmt.Errorf("bgp server not started")
	}
	if localAS == 0 {
		return fmt.Errorf("bgp.local_as must be greater than 0 in config.yaml")
	}
	if m.globalAS != 0 {
		return nil
	}

	routerID = m.effectiveRouterID(routerID)
	if net.ParseIP(routerID) == nil {
		return fmt.Errorf("invalid bgp router_id: %s", routerID)
	}

	listenPort := int32(11790)
	if m.cfg.ListenPort > 0 {
		listenPort = int32(m.cfg.ListenPort)
	}

	listenAddrs := m.cfg.ListenAddresses
	if len(listenAddrs) == 0 {
		listenAddrs = []string{"127.0.0.1"}
	}

	if err := m.bgpServer.StartBgp(ctx, &api.StartBgpRequest{
		Global: &api.Global{
			Asn:             localAS,
			RouterId:        routerID,
			ListenPort:      listenPort,
			ListenAddresses: listenAddrs,
		},
	}); err != nil {
		return fmt.Errorf("start bgp server: %w", err)
	}

	m.globalAS = localAS
	slog.Info("bgp global session started", "local_as", localAS, "router_id", routerID, "listen_port", listenPort)
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
	if m.globalAS == 0 {
		return fmt.Errorf("bgp.local_as is not configured — set bgp.local_as in config.yaml and restart HopStat")
	}

	neighborIP := strings.TrimSpace(n.NeighborIP)
	ipv6NeighborIP := strings.TrimSpace(n.IPv6NeighborIP)

	if neighborIP != "" {
		peer := m.buildPeerConfig(n, n.PeeringIP, neighborIP)
		if err := m.bgpServer.AddPeer(context.Background(), &api.AddPeerRequest{Peer: peer}); err != nil {
			m.recordEvent(n.ID, "error", fmt.Sprintf("add ipv4 peer failed: %v", err), neighborIP)
			return fmt.Errorf("add bgp peer %s: %w", neighborIP, err)
		}
		m.recordEvent(n.ID, "info", fmt.Sprintf("ipv4 peer added; %s", formatNeighborConfig(n, m.globalAS, n.PeeringIP, neighborIP)), neighborIP)
	}

	ipv6IP := ""
	if ipv6NeighborIP != "" {
		peer6 := m.buildPeerConfig(n, n.IPv6PeeringIP, n.IPv6NeighborIP)
		if err := m.bgpServer.AddPeer(context.Background(), &api.AddPeerRequest{Peer: peer6}); err != nil {
			m.recordEvent(n.ID, "warn", fmt.Sprintf("add ipv6 peer failed: %v", err), n.IPv6NeighborIP)
			slog.Warn("bgp ipv6 peer add failed", "id", n.ID, "neighbor_ip", n.IPv6NeighborIP, "err", err)
		} else {
			ipv6IP = n.IPv6NeighborIP
			m.recordEvent(n.ID, "info", fmt.Sprintf("ipv6 peer added; %s", formatNeighborConfig(n, m.globalAS, n.IPv6PeeringIP, n.IPv6NeighborIP)), n.IPv6NeighborIP)
			slog.Info("bgp ipv6 neighbor added", "id", n.ID, "neighbor_ip", n.IPv6NeighborIP)
		}
	}

	m.mu.Lock()
	m.neighbors[n.ID] = &neighborEntry{neighbor: n, neighborIP: neighborIP, ipv6NeighborIP: ipv6IP}
	if m.nodeNeighbors[n.NodeID] == nil {
		m.nodeNeighbors[n.NodeID] = make(map[int64]struct{})
	}
	m.nodeNeighbors[n.NodeID][n.ID] = struct{}{}
	m.states[n.ID] = domain.BGPSessionIdle
	m.mu.Unlock()

	if neighborIP != "" {
		slog.Info("bgp neighbor added", "id", n.ID, "neighbor_ip", neighborIP, "remote_as", n.RemoteAS)
	}
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

	if entry.neighborIP != "" {
		deletePeer := m.bgpServer.DeletePeer
		if deletePeerHook != nil {
			deletePeer = deletePeerHook
		}
		if err := deletePeer(context.Background(), &api.DeletePeerRequest{Address: entry.neighborIP}); err != nil {
			m.recordEvent(id, "error", fmt.Sprintf("remove ipv4 peer failed: %v", err), entry.neighborIP)
			return fmt.Errorf("remove bgp peer %s: %w", entry.neighborIP, err)
		}
		m.recordEvent(id, "info", "ipv4 peer removed", entry.neighborIP)
	}
	if entry.ipv6NeighborIP != "" {
		deletePeer := m.bgpServer.DeletePeer
		if deletePeerHook != nil {
			deletePeer = deletePeerHook
		}
		if err := deletePeer(context.Background(), &api.DeletePeerRequest{Address: entry.ipv6NeighborIP}); err != nil {
			m.recordEvent(id, "warn", fmt.Sprintf("remove ipv6 peer failed: %v", err), entry.ipv6NeighborIP)
			slog.Warn("bgp ipv6 peer remove failed", "neighbor_ip", entry.ipv6NeighborIP, "err", err)
		} else {
			m.recordEvent(id, "info", "ipv6 peer removed", entry.ipv6NeighborIP)
		}
	}

	m.mu.Lock()
	delete(m.neighbors, id)
	delete(m.states, id)
	for nodeID, ids := range m.nodeNeighbors {
		delete(ids, id)
		if len(ids) == 0 {
			delete(m.nodeNeighbors, nodeID)
		}
	}
	m.mu.Unlock()
	m.events.remove(id)
	m.invalidatePrefixCount(id)

	slog.Info("bgp neighbor removed", "id", id)
	return nil
}

func (m *SessionManager) UpdateNeighbor(n *domain.BGPNeighbor) error {
	// Remove the existing peer (if any) then re-add with the new config.
	// This avoids AddPeer failing with "already exists" when the IP hasn't changed.
	if err := m.RemoveNeighbor(n.ID); err != nil {
		slog.Warn("bgp update: failed to remove old neighbor", "id", n.ID, "err", err)
	}
	return m.AddNeighbor(n)
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
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	m.mu.RLock()
	entries := make(map[int64]*neighborEntry, len(m.neighbors))
	states := make(map[int64]domain.BGPSessionState, len(m.states))
	for id, entry := range m.neighbors {
		entries[id] = entry
		states[id] = m.states[id]
	}
	m.mu.RUnlock()

	var out []*domain.BGPSessionStatus
	for id, entry := range entries {
		status := &domain.BGPSessionStatus{
			NeighborID: id,
			NodeID:     entry.neighbor.NodeID,
			State:      states[id],
			RemoteAS:   entry.neighbor.RemoteAS,
			NeighborIP: entry.neighbor.NeighborIP,
		}
		if states[id] == domain.BGPSessionEstablished {
			status.PrefixesReceived = m.prefixesReceivedForEntry(ctx, entry)
		}
		out = append(out, status)
	}
	return out
}

func (m *SessionManager) GetPrefixesReceived(ctx context.Context, neighborID int64) int {
	m.mu.RLock()
	entry, ok := m.neighbors[neighborID]
	state := m.states[neighborID]
	m.mu.RUnlock()
	if !ok || m.bgpServer == nil || state != domain.BGPSessionEstablished {
		m.invalidatePrefixCount(neighborID)
		return 0
	}
	if total, ok := m.cachedPrefixCount(neighborID); ok {
		return total
	}
	total := m.prefixesReceivedForEntry(ctx, entry)
	m.storePrefixCount(neighborID, total)
	return total
}

func (m *SessionManager) prefixesReceivedForEntry(ctx context.Context, entry *neighborEntry) int {
	total := 0
	for _, addr := range m.peerAddresses(entry) {
		for _, afi := range []api.Family_Afi{api.Family_AFI_IP, api.Family_AFI_IP6} {
			n, err := m.countAdjPrefixes(ctx, addr, afi)
			if err != nil {
				slog.Debug("bgp adj-in prefix count failed", "peer", addr, "afi", afi, "error", err)
				continue
			}
			total += n
		}
	}
	return total
}

func (m *SessionManager) countAdjPrefixes(ctx context.Context, peerAddr string, afi api.Family_Afi) (int, error) {
	if peerAddr == "" || m.bgpServer == nil {
		return 0, nil
	}
	resp, err := m.bgpServer.GetTable(ctx, &api.GetTableRequest{
		TableType: api.TableType_ADJ_IN,
		Name:      peerAddr,
		Family:    &api.Family{Afi: afi, Safi: api.Family_SAFI_UNICAST},
	})
	if err != nil {
		return 0, err
	}
	return int(resp.NumDestination), nil
}

func (m *SessionManager) NodeIDForNeighborIP(neighborIP string) (int64, bool) {
	if neighborIP == "" {
		return 0, false
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, entry := range m.neighbors {
		if entry.neighborIP == neighborIP || entry.ipv6NeighborIP == neighborIP {
			return entry.neighbor.NodeID, true
		}
	}
	return 0, false
}

func (m *SessionManager) HasActiveSession(nodeID int64) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for nID := range m.nodeNeighbors[nodeID] {
		if m.states[nID] == domain.BGPSessionEstablished {
			return true
		}
	}
	return false
}

func (m *SessionManager) HasNeighbors(nodeID int64) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.nodeNeighbors[nodeID]) > 0
}

func (m *SessionManager) neighborIPsForNode(nodeID int64) map[string]struct{} {
	m.mu.RLock()
	defer m.mu.RUnlock()
	ips := make(map[string]struct{})
	for neighborID := range m.nodeNeighbors[nodeID] {
		entry, ok := m.neighbors[neighborID]
		if !ok {
			continue
		}
		if entry.neighborIP != "" {
			ips[entry.neighborIP] = struct{}{}
		}
		if entry.ipv6NeighborIP != "" {
			ips[entry.ipv6NeighborIP] = struct{}{}
		}
	}
	return ips
}

func (m *SessionManager) LookupRoute(ctx context.Context, nodeID int64, prefix string) ([]*domain.BGPRouteEntry, error) {
	if m.bgpServer == nil {
		return nil, fmt.Errorf("bgp server not started")
	}

	normalized, err := target.NormalizeBGPLookup(ctx, prefix)
	if lookupNormalizeHook != nil {
		normalized, err = lookupNormalizeHook(ctx, prefix)
	}
	if err != nil {
		return nil, err
	}
	prefix = normalized

	var ip net.IP
	if strings.Contains(prefix, "/") {
		ip, _, err = net.ParseCIDR(prefix)
	} else {
		ip = net.ParseIP(prefix)
	}
	if ip == nil {
		return nil, fmt.Errorf("invalid prefix: %s", prefix)
	}

	family := &api.Family{Afi: api.Family_AFI_IP, Safi: api.Family_SAFI_UNICAST}
	if ip.To4() == nil {
		family = &api.Family{Afi: api.Family_AFI_IP6, Safi: api.Family_SAFI_UNICAST}
	}

	var results []*domain.BGPRouteEntry
	var nodeIPs map[string]struct{}
	if nodeID > 0 {
		nodeIPs = m.neighborIPsForNode(nodeID)
	}

	list := m.bgpServer.ListPath
	if lookupListPathHook != nil {
		list = lookupListPathHook
	}
	err = list(ctx, &api.ListPathRequest{
		TableType: api.TableType_GLOBAL,
		Family:    family,
		Prefixes: []*api.TableLookupPrefix{
			{Prefix: prefix},
		},
	}, func(dst *api.Destination) {
		for _, path := range dst.Paths {
			if nodeID > 0 {
				if _, ok := nodeIPs[path.NeighborIp]; !ok {
					continue
				}
			}
			entry := m.pathToRouteEntry(path, dst.Prefix)
			results = append(results, entry)
		}
	})
	if err != nil {
		return nil, fmt.Errorf("bgp route lookup: %w", err)
	}
	ensureBestAmongEntries(results)
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

func (m *SessionManager) configuredLocalAS() uint32 {
	if m.globalAS > 0 {
		return m.globalAS
	}
	return m.cfg.LocalAS
}

func (m *SessionManager) buildPeerConfig(n *domain.BGPNeighbor, localAddr, neighborAddr string) *api.Peer {
	isV6 := strings.Contains(neighborAddr, ":")
	afiSafis := []*api.AfiSafi{
		m.buildAfiSafi(api.Family_AFI_IP, api.Family_SAFI_UNICAST, !isV6),
		m.buildAfiSafi(api.Family_AFI_IP6, api.Family_SAFI_UNICAST, isV6),
	}

	peer := &api.Peer{
		Conf: &api.PeerConf{
			LocalAsn:        m.globalAS,
			NeighborAddress: neighborAddr,
			PeerAsn:         n.RemoteAS,
			Type:            peerTypeFromNeighbor(n, m.configuredLocalAS()),
		},
		Transport: &api.Transport{
			LocalAddress: localAddr,
			PassiveMode:  false,
		},
		Timers: &api.Timers{
			Config: &api.TimersConfig{
				ConnectRetry:      10,
				HoldTime:          90,
				KeepaliveInterval: 30,
			},
		},
		AfiSafis: afiSafis,
	}

	if n.Multihop && domain.PeerTypeFor(m.configuredLocalAS(), n.RemoteAS) == domain.BGPPeerExternal {
		peer.EbgpMultihop = &api.EbgpMultihop{
			Enabled:     true,
			MultihopTtl: 5,
		}
	}

	return peer
}

func peerTypeFromNeighbor(n *domain.BGPNeighbor, localAS uint32) api.PeerType {
	if domain.PeerTypeFor(localAS, n.RemoteAS).IsInternal() {
		return api.PeerType_INTERNAL
	}
	return api.PeerType_EXTERNAL
}

func (m *SessionManager) buildAfiSafi(afi api.Family_Afi, safi api.Family_Safi, enabled bool) *api.AfiSafi {
	af := &api.AfiSafi{
		Config: &api.AfiSafiConfig{
			Family:  &api.Family{Afi: afi, Safi: safi},
			Enabled: enabled,
		},
	}
	if enabled && m.cfg.AddPathReceive {
		af.AddPaths = &api.AddPaths{
			Config: &api.AddPathsConfig{
				Receive: true,
			},
		}
	}
	return af
}

// injectPeerWatchErr, when set, overrides the error returned from WatchEvent (tests only).
var injectPeerWatchErr error

// lookupListPathHook, when set, replaces bgpServer.ListPath in LookupRoute (tests only).
var lookupListPathHook func(context.Context, *api.ListPathRequest, func(*api.Destination)) error

// listPeerHook, when set, replaces bgpServer.ListPeer in fetchPeerByAddress (tests only).
var listPeerHook func(context.Context, *api.ListPeerRequest, func(*api.Peer)) error

// enablePeerHook, when set, replaces bgpServer.EnablePeer in RestartNeighbor (tests only).
var enablePeerHook func(context.Context, *api.EnablePeerRequest) error

// resetPeerHook, when set, replaces bgpServer.ResetPeer in RestartNeighbor (tests only).
var resetPeerHook func(context.Context, *api.ResetPeerRequest) error

// deletePeerHook, when set, replaces bgpServer.DeletePeer in RemoveNeighbor (tests only).
var deletePeerHook func(context.Context, *api.DeletePeerRequest) error

// lookupNormalizeHook, when set, overrides target.NormalizeBGPLookup in LookupRoute (tests only).
var lookupNormalizeHook func(context.Context, string) (string, error)

func (m *SessionManager) peerWatchDone(err error, ctx context.Context) {
	if injectPeerWatchErr != nil {
		err = injectPeerWatchErr
	}
	if err != nil && ctx.Err() == nil {
		slog.Error("bgp peer watch error", "err", err)
	}
}

func (m *SessionManager) watchPeers(ctx context.Context) {
	err := m.bgpServer.WatchEvent(ctx, &api.WatchEventRequest{
		Peer: &api.WatchEventRequest_Peer{},
	}, m.handleWatchPeerEvent)
	m.peerWatchDone(err, ctx)
}

func (m *SessionManager) handleWatchPeerEvent(ev *api.WatchEventResponse) {
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
			prev := m.states[id]
			m.mu.RUnlock()
			m.handlePeerStateChange(id, neighborAddr, prev, state, peerEv)
			return
		}
	}
	m.mu.RUnlock()
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
			case *bgp.PathAttributeCommunities:
				for _, com := range a.Value {
					entry.Communities = append(entry.Communities, communityToString(com))
				}
			case *bgp.PathAttributeLargeCommunities:
				for _, com := range a.Values {
					if com != nil {
						entry.Communities = append(entry.Communities, com.String())
					}
				}
			case *bgp.PathAttributeExtendedCommunities:
				for _, com := range a.Value {
					if com != nil {
						entry.Communities = append(entry.Communities, com.String())
					}
				}
			}
		}
	}

	return entry
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

func communityToString(value uint32) string {
	asn := value >> 16
	local := value & 0xFFFF
	return fmt.Sprintf("%d:%d", asn, local)
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
