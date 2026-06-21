package bgp

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	api "github.com/osrg/gobgp/v3/api"

	"github.com/HopStat/HopStat/internal/domain"
)

const stuckSessionLogInterval = 3 * time.Minute

// stuckSessionsPollInterval is the watchStuckSessions ticker period (overridable in tests).
var stuckSessionsPollInterval = 90 * time.Second

func formatNeighborConfig(n *domain.BGPNeighbor, localAS uint32, localAddr, neighborAddr string) string {
	peerType := domain.PeerTypeFor(localAS, n.RemoteAS)
	typeLabel := "eBGP"
	if peerType.IsInternal() {
		typeLabel = "iBGP"
	}
	parts := []string{
		fmt.Sprintf("type=%s", typeLabel),
		fmt.Sprintf("local_as=%d", localAS),
		fmt.Sprintf("remote_as=%d", n.RemoteAS),
		fmt.Sprintf("bind=%s", localAddr),
		fmt.Sprintf("peer=%s", neighborAddr),
	}
	if peerType == domain.BGPPeerExternal {
		if n.Multihop {
			parts = append(parts, "multihop=on ttl=5")
		} else {
			parts = append(parts, "multihop=off")
		}
	}
	parts = append(parts, "mode=outbound", "timers=10/90/30")
	return strings.Join(parts, " ")
}

func formatMessageStats(messages *api.Messages) string {
	if messages == nil {
		return "messages=none"
	}
	var parts []string
	if r := messages.GetReceived(); r != nil {
		parts = append(parts, fmt.Sprintf("rx open=%d keepalive=%d update=%d notification=%d discarded=%d",
			r.GetOpen(), r.GetKeepalive(), r.GetUpdate(), r.GetNotification(), r.GetDiscarded()))
	}
	if s := messages.GetSent(); s != nil {
		parts = append(parts, fmt.Sprintf("tx open=%d keepalive=%d update=%d notification=%d",
			s.GetOpen(), s.GetKeepalive(), s.GetUpdate(), s.GetNotification()))
	}
	if len(parts) == 0 {
		return "messages=none"
	}
	return strings.Join(parts, "; ")
}

func formatTransport(t *api.Transport) string {
	if t == nil {
		return "transport=none"
	}
	local := strings.TrimSpace(t.GetLocalAddress())
	if local == "" {
		local = "unspecified"
	}
	remotePort := t.GetRemotePort()
	localPort := t.GetLocalPort()
	if remotePort == 0 && localPort == 0 {
		return fmt.Sprintf("transport local=%s (no active socket)", local)
	}
	return fmt.Sprintf("transport local=%s:%d remote_port=%d passive=%t",
		local, localPort, remotePort, t.GetPassiveMode())
}

func formatPeerSnapshot(peer *api.Peer) string {
	if peer == nil {
		return ""
	}
	var parts []string
	if conf := peer.GetConf(); conf != nil {
		parts = append(parts, fmt.Sprintf("admin=%s", peerAdminState(peer.GetState())))
		if conf.GetType() == api.PeerType_INTERNAL {
			parts = append(parts, "peer_type=iBGP")
		} else {
			parts = append(parts, "peer_type=eBGP")
		}
	}
	if st := peer.GetState(); st != nil {
		if rid := strings.TrimSpace(st.GetRouterId()); rid != "" && rid != "0.0.0.0" {
			parts = append(parts, fmt.Sprintf("peer_router_id=%s", rid))
		}
		if st.GetFlops() > 0 {
			parts = append(parts, fmt.Sprintf("flops=%d", st.GetFlops()))
		}
		parts = append(parts, formatMessageStats(st.GetMessages()))
	}
	parts = append(parts, formatTransport(peer.GetTransport()))
	if mh := peer.GetEbgpMultihop(); mh != nil && mh.GetEnabled() {
		parts = append(parts, fmt.Sprintf("multihop_ttl=%d", mh.GetMultihopTtl()))
	}
	return strings.Join(parts, "; ")
}

func peerAdminState(st *api.PeerState) string {
	if st == nil {
		return "unknown"
	}
	switch st.GetAdminState() {
	case api.PeerState_UP:
		return "up"
	case api.PeerState_DOWN:
		return "down"
	case api.PeerState_PFX_CT:
		return "pfx_ct"
	default:
		return "unknown"
	}
}

func sessionTransitionHint(prev, next domain.BGPSessionState) string {
	switch {
	case next == domain.BGPSessionEstablished:
		return "session established"
	case prev == domain.BGPSessionOpenConfirm && next == domain.BGPSessionIdle:
		return "session failed after OPEN exchange — peer may have sent NOTIFICATION (check notification count), hold timer expired, or capabilities mismatch"
	case prev == domain.BGPSessionOpenSent && (next == domain.BGPSessionIdle || next == domain.BGPSessionActive):
		return "OPEN negotiation failed — verify remote AS, MD5 password, TTL/multihop, and that the peer accepts our OPEN"
	case prev == domain.BGPSessionConnect && next == domain.BGPSessionActive:
		return "outbound TCP connect failed — GoBGP will retry on alternate source address or port"
	case prev == domain.BGPSessionActive && next == domain.BGPSessionIdle:
		return "TCP connection attempts exhausted — check reachability to peer, firewall (TCP/179), correct local bind address, and eBGP multihop if not directly connected"
	case next == domain.BGPSessionActive:
		return "attempting outbound TCP connection to peer BGP port (179)"
	case next == domain.BGPSessionConnect:
		return "initiating TCP connection to peer"
	case next == domain.BGPSessionOpenSent:
		return "TCP connected, sending BGP OPEN"
	case next == domain.BGPSessionOpenConfirm:
		return "received peer OPEN, waiting for KEEPALIVE"
	case prev == domain.BGPSessionEstablished && next != domain.BGPSessionEstablished:
		return "established session lost — check peer logs and link stability"
	case next == domain.BGPSessionIdle && prev != domain.BGPSessionIdle:
		return "returned to idle — session reset or connect retry timer expired"
	default:
		return ""
	}
}

func logLevelForStateChange(prev, next domain.BGPSessionState) string {
	if next == domain.BGPSessionEstablished {
		return "info"
	}
	switch {
	case prev == domain.BGPSessionOpenConfirm && next == domain.BGPSessionIdle:
		return "error"
	case prev == domain.BGPSessionOpenSent && (next == domain.BGPSessionIdle || next == domain.BGPSessionActive):
		return "warn"
	case prev == domain.BGPSessionEstablished:
		return "warn"
	case prev == domain.BGPSessionActive && next == domain.BGPSessionIdle:
		return "warn"
	case next == domain.BGPSessionActive || next == domain.BGPSessionConnect:
		return "info"
	default:
		return "info"
	}
}

func isSessionFailureTransition(prev, next domain.BGPSessionState) bool {
	if next == domain.BGPSessionEstablished {
		return false
	}
	if prev == domain.BGPSessionEstablished {
		return true
	}
	regressions := map[domain.BGPSessionState]map[domain.BGPSessionState]bool{
		domain.BGPSessionOpenConfirm: {domain.BGPSessionIdle: true, domain.BGPSessionActive: true},
		domain.BGPSessionOpenSent:    {domain.BGPSessionIdle: true, domain.BGPSessionActive: true, domain.BGPSessionConnect: true},
		domain.BGPSessionConnect:     {domain.BGPSessionIdle: true, domain.BGPSessionActive: true},
		domain.BGPSessionActive:      {domain.BGPSessionIdle: true},
	}
	if targets, ok := regressions[prev]; ok {
		return targets[next]
	}
	return false
}

func (m *SessionManager) fetchPeerByAddress(ctx context.Context, addr string) (*api.Peer, error) {
	if m.bgpServer == nil {
		return nil, fmt.Errorf("bgp server not started")
	}
	var found *api.Peer
	list := m.bgpServer.ListPeer
	if listPeerHook != nil {
		list = listPeerHook
	}
	err := list(ctx, &api.ListPeerRequest{Address: addr}, func(p *api.Peer) {
		found = p
	})
	if err != nil {
		return nil, err
	}
	if found == nil {
		return nil, fmt.Errorf("peer not found: %s", addr)
	}
	return found, nil
}

func (m *SessionManager) buildStateChangeMessage(prev, next domain.BGPSessionState, neighborAddr string, peer *api.Peer, inStateFor time.Duration) string {
	msg := fmt.Sprintf("state %s → %s", prev, next)
	if hint := sessionTransitionHint(prev, next); hint != "" {
		msg += "; " + hint
	}
	if peer != nil {
		if snap := formatPeerSnapshot(peer); snap != "" {
			msg += "; " + snap
		}
	}
	if inStateFor > 0 && next != domain.BGPSessionEstablished {
		msg += fmt.Sprintf("; in_state_for=%s", inStateFor.Truncate(time.Second))
	}
	if neighborAddr != "" && isSessionFailureTransition(prev, next) {
		msg += "; check peer reachability, AS/password, multihop/TTL, and firewall rules for TCP/179"
	}
	return msg
}

func (m *SessionManager) handlePeerStateChange(id int64, neighborAddr string, prev, next domain.BGPSessionState, peerEv *api.WatchEventResponse_PeerEvent) {
	if peerEv != nil {
		switch peerEv.GetType() {
		case api.WatchEventResponse_PeerEvent_END_OF_INIT:
			return
		case api.WatchEventResponse_PeerEvent_INIT:
			m.recordEvent(id, "info", fmt.Sprintf("peer watcher initialized for %s", neighborAddr), neighborAddr)
			return
		}
	}
	if prev == next {
		return
	}

	now := time.Now()
	m.mu.Lock()
	if m.stateSince == nil {
		m.stateSince = make(map[int64]time.Time)
	}
	inStateFor := now.Sub(m.stateSince[id])
	m.stateSince[id] = now
	m.states[id] = next
	m.mu.Unlock()

	m.invalidatePrefixCount(id)

	level := logLevelForStateChange(prev, next)
	var peer *api.Peer
	if peerEv != nil {
		peer = peerEv.GetPeer()
	}
	if isSessionFailureTransition(prev, next) || next != domain.BGPSessionEstablished {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		if snap, err := m.fetchPeerByAddress(ctx, neighborAddr); err == nil {
			peer = snap
		}
		cancel()
	}

	msg := m.buildStateChangeMessage(prev, next, neighborAddr, peer, inStateFor)
	m.recordEvent(id, level, msg, neighborAddr)
	slogFields := []any{"neighbor_id", id, "neighbor", neighborAddr, "prev", prev, "state", next, "level", level}
	if hint := sessionTransitionHint(prev, next); hint != "" {
		slogFields = append(slogFields, "hint", hint)
	}
	switch level {
	case "error":
		slog.Error("bgp peer state changed", slogFields...)
	case "warn":
		slog.Warn("bgp peer state changed", slogFields...)
	default:
		slog.Info("bgp peer state changed", slogFields...)
	}
}

func (m *SessionManager) watchStuckSessions(ctx context.Context) {
	ticker := time.NewTicker(stuckSessionsPollInterval)
	defer ticker.Stop()

	lastLogged := make(map[int64]time.Time)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.logStuckSessions(ctx, lastLogged)
		}
	}
}

func (m *SessionManager) logStuckSessions(ctx context.Context, lastLogged map[int64]time.Time) {
	m.mu.RLock()
	type stuck struct {
		id      int64
		entry   *neighborEntry
		state   domain.BGPSessionState
		since   time.Time
	}
	var peers []stuck
	for id, entry := range m.neighbors {
		state := m.states[id]
		if state == domain.BGPSessionEstablished || state == "" {
			continue
		}
		since := m.stateSince[id]
		if since.IsZero() {
			since = time.Now()
		}
		peers = append(peers, stuck{id: id, entry: entry, state: state, since: since})
	}
	m.mu.RUnlock()

	for _, p := range peers {
		if time.Since(lastLogged[p.id]) < stuckSessionLogInterval {
			continue
		}
		inStateFor := time.Since(p.since)
		if inStateFor < 45*time.Second {
			continue
		}

		for _, addr := range m.peerAddresses(p.entry) {
			reqCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
			snap, err := m.fetchPeerByAddress(reqCtx, addr)
			cancel()
			if err != nil {
				m.recordEvent(p.id, "warn",
					fmt.Sprintf("session not established (state=%s, in_state_for=%s); failed to fetch peer stats: %v",
						p.state, inStateFor.Truncate(time.Second), err),
					addr)
				continue
			}
			msg := fmt.Sprintf("session not established (state=%s, in_state_for=%s); %s",
				p.state, inStateFor.Truncate(time.Second), formatPeerSnapshot(snap))
			if hint := stuckStateHint(p.state); hint != "" {
				msg += "; " + hint
			}
			m.recordEvent(p.id, "warn", msg, addr)
		}
		lastLogged[p.id] = time.Now()
	}
}

func stuckStateHint(state domain.BGPSessionState) string {
	switch state {
	case domain.BGPSessionIdle:
		return "waiting before next connect attempt — verify peer is configured and admin-up"
	case domain.BGPSessionConnect, domain.BGPSessionActive:
		return "still trying TCP/179 — verify routing, ACLs, and local bind address"
	case domain.BGPSessionOpenSent, domain.BGPSessionOpenConfirm:
		return "BGP negotiation in progress or failing — verify AS numbers, auth, and capabilities"
	default:
		return ""
	}
}
