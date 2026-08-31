package bgp

import (
	"strings"
	"testing"
	"time"

	api "github.com/osrg/gobgp/v3/api"

	"github.com/HopStat/HopStat/internal/config"
	"github.com/HopStat/HopStat/internal/domain"
)

func TestSessionTransitionHint(t *testing.T) {
	hint := sessionTransitionHint(domain.BGPSessionOpenConfirm, domain.BGPSessionIdle)
	if !strings.Contains(hint, "NOTIFICATION") {
		t.Fatalf("hint = %q, want notification guidance", hint)
	}

	hint = sessionTransitionHint(domain.BGPSessionConnect, domain.BGPSessionActive)
	if !strings.Contains(hint, "TCP") {
		t.Fatalf("hint = %q, want TCP guidance", hint)
	}
}

func TestLogLevelForStateChange(t *testing.T) {
	if got := logLevelForStateChange(domain.BGPSessionOpenConfirm, domain.BGPSessionIdle); got != "error" {
		t.Fatalf("level = %q, want error", got)
	}
	if got := logLevelForStateChange(domain.BGPSessionIdle, domain.BGPSessionEstablished); got != "info" {
		t.Fatalf("level = %q, want info", got)
	}
}

func TestFormatNeighborConfig(t *testing.T) {
	n := &domain.BGPNeighbor{
		RemoteAS: 65002,
		Multihop: true,
	}
	got := formatNeighborConfig(n, 65001, "10.0.0.1", "203.0.113.5")
	for _, want := range []string{"type=eBGP", "local_as=65001", "remote_as=65002", "multihop=on", "bind=10.0.0.1", "peer=203.0.113.5"} {
		if !strings.Contains(got, want) {
			t.Fatalf("config = %q, missing %q", got, want)
		}
	}

	n.RemoteAS = 65001
	got = formatNeighborConfig(n, 65001, "10.0.0.1", "10.0.0.2")
	if !strings.Contains(got, "type=iBGP") {
		t.Fatalf("config = %q, want iBGP", got)
	}
	if !strings.Contains(got, "mode=outbound") {
		t.Fatalf("config = %q, want outbound by default", got)
	}

	n.PassiveMode = true
	got = formatNeighborConfig(n, 65001, "10.0.0.1", "10.0.0.2")
	if !strings.Contains(got, "mode=passive") {
		t.Fatalf("config = %q, want passive mode", got)
	}
}

func TestFormatPeerSnapshot(t *testing.T) {
	peer := &api.Peer{
		Conf: &api.PeerConf{Type: api.PeerType_EXTERNAL},
		State: &api.PeerState{
			AdminState: api.PeerState_UP,
			Flops:      3,
			Messages: &api.Messages{
				Received: &api.Message{Open: 1, Notification: 2},
				Sent:     &api.Message{Open: 1},
			},
		},
		Transport: &api.Transport{
			LocalAddress: "10.0.0.1",
			LocalPort:    45678,
			RemotePort:   179,
		},
		EbgpMultihop: &api.EbgpMultihop{Enabled: true, MultihopTtl: 5},
	}
	got := formatPeerSnapshot(peer)
	for _, want := range []string{"flops=3", "rx open=1", "notification=2", "local=10.0.0.1:45678", "multihop_ttl=5"} {
		if !strings.Contains(got, want) {
			t.Fatalf("snapshot = %q, missing %q", got, want)
		}
	}

	if formatPeerSnapshot(nil) != "" {
		t.Fatal("nil peer should return empty snapshot")
	}
	internal := &api.Peer{
		Conf:  &api.PeerConf{Type: api.PeerType_INTERNAL},
		State: &api.PeerState{AdminState: api.PeerState_DOWN, RouterId: "10.0.0.5"},
	}
	if got := formatPeerSnapshot(internal); !strings.Contains(got, "peer_type=iBGP") || !strings.Contains(got, "admin=down") {
		t.Fatalf("internal snapshot = %q", got)
	}
}

func TestFormatMessageStatsAndTransport(t *testing.T) {
	if got := formatMessageStats(nil); got != "messages=none" {
		t.Fatalf("nil messages = %q", got)
	}
	if got := formatMessageStats(&api.Messages{}); got != "messages=none" {
		t.Fatalf("empty messages = %q", got)
	}
	if got := formatTransport(nil); got != "transport=none" {
		t.Fatalf("nil transport = %q", got)
	}
	if got := formatTransport(&api.Transport{}); !strings.Contains(got, "unspecified") {
		t.Fatalf("unspecified transport = %q", got)
	}
	if got := formatTransport(&api.Transport{LocalAddress: "10.0.0.1"}); !strings.Contains(got, "no active socket") {
		t.Fatalf("inactive transport = %q", got)
	}
}

func TestPeerAdminState(t *testing.T) {
	if peerAdminState(nil) != "unknown" {
		t.Fatal("nil state should be unknown")
	}
	for state, want := range map[api.PeerState_AdminState]string{
		api.PeerState_UP:     "up",
		api.PeerState_DOWN:   "down",
		api.PeerState_PFX_CT: "pfx_ct",
		99:                   "unknown",
	} {
		if got := peerAdminState(&api.PeerState{AdminState: state}); got != want {
			t.Fatalf("admin state %v = %q, want %q", state, got, want)
		}
	}
}

func TestSessionTransitionHintAllBranches(t *testing.T) {
	cases := []struct {
		prev, next domain.BGPSessionState
		contains   string
	}{
		{domain.BGPSessionIdle, domain.BGPSessionEstablished, "established"},
		{domain.BGPSessionOpenConfirm, domain.BGPSessionIdle, "NOTIFICATION"},
		{domain.BGPSessionOpenSent, domain.BGPSessionIdle, "OPEN negotiation"},
		{domain.BGPSessionOpenSent, domain.BGPSessionActive, "OPEN negotiation"},
		{domain.BGPSessionConnect, domain.BGPSessionActive, "TCP connect failed"},
		{domain.BGPSessionActive, domain.BGPSessionIdle, "TCP connection attempts"},
		{domain.BGPSessionIdle, domain.BGPSessionActive, "outbound TCP"},
		{domain.BGPSessionIdle, domain.BGPSessionConnect, "initiating TCP"},
		{domain.BGPSessionConnect, domain.BGPSessionOpenSent, "sending BGP OPEN"},
		{domain.BGPSessionOpenSent, domain.BGPSessionOpenConfirm, "KEEPALIVE"},
		{domain.BGPSessionEstablished, domain.BGPSessionIdle, "established session lost"},
		{domain.BGPSessionConnect, domain.BGPSessionIdle, "returned to idle"},
	}
	for _, tc := range cases {
		hint := sessionTransitionHint(tc.prev, tc.next)
		if !strings.Contains(hint, tc.contains) {
			t.Fatalf("hint(%s→%s) = %q, want %q", tc.prev, tc.next, hint, tc.contains)
		}
	}
	if sessionTransitionHint(domain.BGPSessionIdle, domain.BGPSessionIdle) != "" {
		t.Fatal("expected empty hint for no-op transition")
	}
}

func TestLogLevelForStateChangeBranches(t *testing.T) {
	cases := []struct {
		prev, next domain.BGPSessionState
		want       string
	}{
		{domain.BGPSessionOpenSent, domain.BGPSessionIdle, "warn"},
		{domain.BGPSessionEstablished, domain.BGPSessionIdle, "warn"},
		{domain.BGPSessionActive, domain.BGPSessionIdle, "warn"},
		{domain.BGPSessionIdle, domain.BGPSessionActive, "info"},
		{domain.BGPSessionIdle, domain.BGPSessionConnect, "info"},
		{domain.BGPSessionIdle, domain.BGPSessionOpenSent, "info"},
	}
	for _, tc := range cases {
		if got := logLevelForStateChange(tc.prev, tc.next); got != tc.want {
			t.Fatalf("level(%s→%s) = %q, want %q", tc.prev, tc.next, got, tc.want)
		}
	}
}

func TestIsSessionFailureTransition(t *testing.T) {
	if isSessionFailureTransition(domain.BGPSessionIdle, domain.BGPSessionEstablished) {
		t.Fatal("establishment is not a failure")
	}
	if !isSessionFailureTransition(domain.BGPSessionEstablished, domain.BGPSessionIdle) {
		t.Fatal("established drop is a failure")
	}
	if !isSessionFailureTransition(domain.BGPSessionOpenConfirm, domain.BGPSessionIdle) {
		t.Fatal("open confirm regression is a failure")
	}
	if isSessionFailureTransition(domain.BGPSessionIdle, domain.BGPSessionConnect) {
		t.Fatal("normal progression is not a failure")
	}
}

func TestBuildStateChangeMessage(t *testing.T) {
	mgr := NewSessionManager(config.BGPConfig{})
	peer := &api.Peer{
		Conf:  &api.PeerConf{Type: api.PeerType_EXTERNAL},
		State: &api.PeerState{AdminState: api.PeerState_UP},
	}
	msg := mgr.buildStateChangeMessage(domain.BGPSessionOpenConfirm, domain.BGPSessionIdle, "10.0.0.2", peer, time.Minute)
	for _, want := range []string{"open_confirm", "idle", "NOTIFICATION", "admin=up", "in_state_for=1m0s", "firewall"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("message = %q, missing %q", msg, want)
		}
	}
}

func TestStuckStateHint(t *testing.T) {
	for state, want := range map[domain.BGPSessionState]string{
		domain.BGPSessionIdle:        "admin-up",
		domain.BGPSessionConnect:     "TCP/179",
		domain.BGPSessionActive:      "TCP/179",
		domain.BGPSessionOpenSent:    "BGP negotiation",
		domain.BGPSessionOpenConfirm: "BGP negotiation",
	} {
		if got := stuckStateHint(state); !strings.Contains(got, want) {
			t.Fatalf("hint(%s) = %q, want %q", state, got, want)
		}
	}
	if stuckStateHint(domain.BGPSessionEstablished) != "" {
		t.Fatal("established should have no stuck hint")
	}
}

func TestLogStuckSessions(t *testing.T) {
	mgr, ctx := startTestManager(t, config.BGPConfig{LocalAS: 65000, RouterID: "127.0.0.1"})
	if err := mgr.AddNeighbor(testNeighbor(50, 1, "127.0.0.1")); err != nil {
		t.Fatalf("AddNeighbor: %v", err)
	}
	mgr.mu.Lock()
	mgr.states[50] = domain.BGPSessionActive
	mgr.stateSince = map[int64]time.Time{50: time.Now().Add(-2 * time.Minute)}
	mgr.mu.Unlock()

	lastLogged := make(map[int64]time.Time)
	mgr.logStuckSessions(ctx, lastLogged)
	if _, ok := lastLogged[50]; !ok {
		t.Fatal("expected stuck session to be logged")
	}
	logs := mgr.GetNeighborLogs(50, 5)
	if len(logs) == 0 {
		t.Fatal("expected stuck session log entries")
	}

	// Unknown peer address path (no gobgp peer for fake IP).
	mgr.mu.Lock()
	mgr.neighbors[51] = &neighborEntry{
		neighbor:   testNeighbor(51, 1, "198.51.100.50"),
		neighborIP: "198.51.100.50",
	}
	mgr.states[51] = domain.BGPSessionConnect
	mgr.stateSince[51] = time.Now().Add(-2 * time.Minute)
	mgr.mu.Unlock()
	mgr.logStuckSessions(ctx, lastLogged)
	failLogs := mgr.GetNeighborLogs(51, 5)
	if len(failLogs) == 0 || !strings.Contains(failLogs[len(failLogs)-1].Message, "failed to fetch peer stats") {
		t.Fatalf("expected fetch failure log, got %+v", failLogs)
	}
}

func TestFormatNeighborConfigMultihopOff(t *testing.T) {
	n := &domain.BGPNeighbor{RemoteAS: 65002}
	got := formatNeighborConfig(n, 65001, "10.0.0.1", "10.0.0.2")
	if !strings.Contains(got, "multihop=off") {
		t.Fatalf("config = %q", got)
	}
}
