package bgp

import (
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync/atomic"

	gobgplog "github.com/osrg/gobgp/v3/pkg/log"
)

// gobgpPeerDownMsg is the message GoBGP logs when an established session drops. It is the
// only place GoBGP states why (hold timer, NOTIFICATION, socket error); the watch API only
// reports the new state.
const gobgpPeerDownMsg = "Peer Down"

// exitProcess is os.Exit, swapped out in tests.
var exitProcess = os.Exit

// gobgpLogger routes GoBGP's own log lines into slog, and copies the reason a session
// went down into that neighbor's event log so it shows up in the admin panel.
type gobgpLogger struct {
	m     *SessionManager
	level atomic.Uint32
}

func newGobgpLogger(m *SessionManager) *gobgpLogger {
	l := &gobgpLogger{m: m}
	l.level.Store(uint32(gobgplog.InfoLevel))
	return l
}

func (l *gobgpLogger) enabled(level gobgplog.LogLevel) bool {
	return level <= gobgplog.LogLevel(l.level.Load())
}

func (l *gobgpLogger) Panic(msg string, fields gobgplog.Fields) {
	slog.Error("gobgp: "+msg, fieldsToAttrs(fields)...)
	panic(msg)
}

func (l *gobgpLogger) Fatal(msg string, fields gobgplog.Fields) {
	slog.Error("gobgp: "+msg, fieldsToAttrs(fields)...)
	exitProcess(1)
}

func (l *gobgpLogger) Error(msg string, fields gobgplog.Fields) {
	if l.enabled(gobgplog.ErrorLevel) {
		slog.Error("gobgp: "+msg, fieldsToAttrs(fields)...)
	}
}

func (l *gobgpLogger) Warn(msg string, fields gobgplog.Fields) {
	if l.enabled(gobgplog.WarnLevel) {
		slog.Warn("gobgp: "+msg, fieldsToAttrs(fields)...)
	}
}

func (l *gobgpLogger) Info(msg string, fields gobgplog.Fields) {
	if msg == gobgpPeerDownMsg {
		l.m.recordPeerDown(fields)
	}
	if l.enabled(gobgplog.InfoLevel) {
		slog.Info("gobgp: "+msg, fieldsToAttrs(fields)...)
	}
}

func (l *gobgpLogger) Debug(msg string, fields gobgplog.Fields) {
	if l.enabled(gobgplog.DebugLevel) {
		slog.Debug("gobgp: "+msg, fieldsToAttrs(fields)...)
	}
}

func (l *gobgpLogger) SetLevel(level gobgplog.LogLevel) { l.level.Store(uint32(level)) }

func (l *gobgpLogger) GetLevel() gobgplog.LogLevel { return gobgplog.LogLevel(l.level.Load()) }

func fieldsToAttrs(fields gobgplog.Fields) []any {
	attrs := make([]any, 0, len(fields)*2)
	for k, v := range fields {
		attrs = append(attrs, k, v)
	}
	return attrs
}

// recordPeerDown writes GoBGP's down reason to the neighbor that owns the address.
func (m *SessionManager) recordPeerDown(fields gobgplog.Fields) {
	addr, _ := fields["Key"].(string)
	reason := fmt.Sprint(fields["Reason"])
	id, ok := m.neighborIDForAddress(addr)
	if !ok {
		return
	}
	msg := "peer down; reason=" + reason
	if hint := peerDownHint(reason); hint != "" {
		msg += "; " + hint
	}
	m.recordEvent(id, "warn", msg, addr)
}

// peerDownHint explains GoBGP's FSM reason strings (fsmStateReason.String in GoBGP).
func peerDownHint(reason string) string {
	switch {
	case reason == "hold-timer-expired":
		return "nothing received from the peer within the negotiated hold time — keepalives are lost on the path (link loss, firewall/conntrack, MTU) or the peer stopped sending"
	case strings.HasPrefix(reason, "notification-received"):
		return "the peer closed the session — its NOTIFICATION code above says why; check the router log"
	case strings.HasPrefix(reason, "notification-sent"):
		return "HopStat closed the session — the NOTIFICATION code above says why"
	case reason == "hard-reset":
		return "the peer sent a hard reset NOTIFICATION"
	case reason == "read-failed":
		return "the TCP connection was closed or reset by the peer or the network"
	case reason == "write-failed":
		return "the TCP socket could not be written — the connection was already broken"
	default:
		return ""
	}
}

func (m *SessionManager) neighborIDForAddress(addr string) (int64, bool) {
	if addr == "" {
		return 0, false
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	for id, entry := range m.neighbors {
		if entry.neighborIP == addr || entry.ipv6NeighborIP == addr {
			return id, true
		}
	}
	return 0, false
}

func (m *SessionManager) isPassive(id int64) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	entry, ok := m.neighbors[id]
	return ok && entry.neighbor != nil && entry.neighbor.PassiveMode
}
