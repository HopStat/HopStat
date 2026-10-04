package bgp

import (
	"strings"
	"testing"
	"time"

	gobgplog "github.com/osrg/gobgp/v3/pkg/log"

	"github.com/HopStat/HopStat/internal/config"
	"github.com/HopStat/HopStat/internal/domain"
)

func managerWithNeighbor(t *testing.T, passive bool) *SessionManager {
	t.Helper()
	mgr := NewSessionManager(config.BGPConfig{LocalAS: 65000})
	n := testNeighbor(7, 1, "10.4.4.221")
	n.PassiveMode = passive
	mgr.mu.Lock()
	mgr.neighbors[7] = &neighborEntry{neighbor: n, neighborIP: "10.4.4.221", ipv6NeighborIP: "2001:db8::1"}
	mgr.states[7] = domain.BGPSessionEstablished
	mgr.mu.Unlock()
	return mgr
}

func lastLog(t *testing.T, mgr *SessionManager, id int64) LogEntry {
	t.Helper()
	logs := mgr.GetNeighborLogs(id, 50)
	if len(logs) == 0 {
		t.Fatal("expected a log entry")
	}
	return logs[len(logs)-1]
}

func TestGobgpLoggerRecordsPeerDownReason(t *testing.T) {
	mgr := managerWithNeighbor(t, false)
	l := newGobgpLogger(mgr)

	l.Info(gobgpPeerDownMsg, gobgplog.Fields{"Key": "10.4.4.221", "Reason": "hold-timer-expired"})
	got := lastLog(t, mgr, 7)
	if got.Level != "warn" || !strings.Contains(got.Message, "reason=hold-timer-expired") || !strings.Contains(got.Message, "hold time") {
		t.Fatalf("log = %+v", got)
	}

	// The IPv6 session of the same neighbor is matched too.
	l.Info(gobgpPeerDownMsg, gobgplog.Fields{"Key": "2001:db8::1", "Reason": "read-failed"})
	if got := lastLog(t, mgr, 7); !strings.Contains(got.Message, "reason=read-failed") {
		t.Fatalf("log = %+v", got)
	}

	before := len(mgr.GetNeighborLogs(7, 50))
	l.Info(gobgpPeerDownMsg, gobgplog.Fields{"Key": "192.0.2.99", "Reason": "read-failed"})
	l.Info(gobgpPeerDownMsg, gobgplog.Fields{"Reason": "read-failed"})
	l.Info("Add a peer configuration", gobgplog.Fields{"Key": "10.4.4.221"})
	if after := len(mgr.GetNeighborLogs(7, 50)); after != before {
		t.Fatalf("unrelated lines recorded: %d → %d", before, after)
	}
}

func TestPeerDownHint(t *testing.T) {
	cases := map[string]string{
		"hold-timer-expired":                       "hold time",
		"notification-received hold timer expired": "peer closed",
		"notification-sent cease":                  "HopStat closed",
		"hard-reset":                               "hard reset",
		"read-failed":                              "closed or reset",
		"write-failed":                             "could not be written",
	}
	for reason, want := range cases {
		if got := peerDownHint(reason); !strings.Contains(got, want) {
			t.Fatalf("peerDownHint(%q) = %q, want %q", reason, got, want)
		}
	}
	if peerDownHint("graceful-restart") != "" {
		t.Fatal("expected no hint for an unlisted reason")
	}

	mgr := managerWithNeighbor(t, false)
	mgr.recordPeerDown(gobgplog.Fields{"Key": "10.4.4.221", "Reason": "graceful-restart"})
	if got := lastLog(t, mgr, 7); got.Message != "peer down; reason=graceful-restart" {
		t.Fatalf("log = %q", got.Message)
	}
}

func TestGobgpLoggerLevels(t *testing.T) {
	l := newGobgpLogger(NewSessionManager(config.BGPConfig{}))
	if l.GetLevel() != gobgplog.InfoLevel {
		t.Fatalf("level = %v", l.GetLevel())
	}
	fields := gobgplog.Fields{"Topic": "Peer"}
	l.Error("e", fields)
	l.Warn("w", fields)
	l.Info("i", fields)
	l.Debug("d", fields)

	l.SetLevel(gobgplog.DebugLevel)
	l.Debug("d", fields)
	l.SetLevel(gobgplog.PanicLevel)
	l.Error("e", fields)
	l.Warn("w", fields)
	l.Info("i", fields)
	if l.GetLevel() != gobgplog.PanicLevel {
		t.Fatalf("level = %v", l.GetLevel())
	}
}

func TestGobgpLoggerPanicAndFatal(t *testing.T) {
	l := newGobgpLogger(NewSessionManager(config.BGPConfig{}))

	func() {
		defer func() {
			if r := recover(); r != "boom" {
				t.Fatalf("recover = %v", r)
			}
		}()
		l.Panic("boom", nil)
	}()

	code := -1
	prev := exitProcess
	exitProcess = func(c int) { code = c }
	t.Cleanup(func() { exitProcess = prev })
	l.Fatal("fatal", gobgplog.Fields{"a": 1})
	if code != 1 {
		t.Fatalf("exit code = %d", code)
	}
}

func TestPassiveTransitionHint(t *testing.T) {
	if got := transitionHint(domain.BGPSessionIdle, domain.BGPSessionActive, true); !strings.Contains(got, "waiting for the peer") {
		t.Fatalf("hint = %q", got)
	}
	if got := transitionHint(domain.BGPSessionActive, domain.BGPSessionIdle, true); !strings.Contains(got, "no inbound connection") {
		t.Fatalf("hint = %q", got)
	}
	if got := transitionHint(domain.BGPSessionOpenConfirm, domain.BGPSessionEstablished, true); got != "session established" {
		t.Fatalf("hint = %q", got)
	}
	if got := transitionHint(domain.BGPSessionIdle, domain.BGPSessionActive, false); !strings.Contains(got, "outbound") {
		t.Fatalf("hint = %q", got)
	}
}

func TestHandlePeerStateChangePassiveAndFirstDuration(t *testing.T) {
	mgr := managerWithNeighbor(t, true)
	mgr.mu.Lock()
	mgr.states[7] = domain.BGPSessionIdle
	mgr.mu.Unlock()

	// No recorded start: the message must not carry an overflowed in_state_for.
	mgr.handlePeerStateChange(7, "10.4.4.221", domain.BGPSessionIdle, domain.BGPSessionActive, nil)
	got := lastLog(t, mgr, 7)
	if strings.Contains(got.Message, "in_state_for") {
		t.Fatalf("unexpected duration in %q", got.Message)
	}
	if !strings.Contains(got.Message, "passive mode") || strings.Contains(got.Message, "attempting outbound") {
		t.Fatalf("message = %q", got.Message)
	}
	if mgr.isPassive(99) {
		t.Fatal("unknown neighbor reported passive")
	}
}

func TestAddNeighborRecordsStateSince(t *testing.T) {
	mgr, _ := startTestManager(t, config.BGPConfig{LocalAS: 65000, RouterID: "127.0.0.1"})
	mgr.stateSince = nil
	before := time.Now()
	if err := mgr.AddNeighbor(testNeighbor(60, 1, "127.0.0.1")); err != nil {
		t.Fatalf("AddNeighbor: %v", err)
	}
	mgr.mu.RLock()
	since := mgr.stateSince[60]
	mgr.mu.RUnlock()
	if since.Before(before) {
		t.Fatalf("stateSince = %v, want ≥ %v", since, before)
	}
}

func TestLogStuckSessionsPassiveHint(t *testing.T) {
	mgr, ctx := startTestManager(t, config.BGPConfig{LocalAS: 65000, RouterID: "127.0.0.1"})
	n := testNeighbor(61, 1, "127.0.0.1")
	n.PassiveMode = true
	if err := mgr.AddNeighbor(n); err != nil {
		t.Fatalf("AddNeighbor: %v", err)
	}
	mgr.mu.Lock()
	mgr.states[61] = domain.BGPSessionActive
	mgr.stateSince[61] = time.Now().Add(-2 * time.Minute)
	mgr.mu.Unlock()

	mgr.logStuckSessions(ctx, make(map[int64]time.Time))
	if got := lastLog(t, mgr, 61); !strings.Contains(got.Message, "no inbound TCP/179 connection") {
		t.Fatalf("message = %q", got.Message)
	}
}
