package lgnode

import (
	"context"
	"errors"
	"net"
	"strings"
	"testing"
)

// errDialed lets the stub dialer report that a connection was attempted, without
// opening a socket, so the tests stay off the network and stay fast.
var errDialed = errors.New("stub dialer: recorded the address")

type stubDialer struct{ called string }

func (s *stubDialer) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	s.called = addr
	return nil, errDialed
}

// stubAgentNet replaces both seams and restores them.
func stubAgentNet(t *testing.T, lookup *stubDialer, ips ...string) *stubDialer {
	t.Helper()
	oldLookup, oldDialer := agentLookupIP, agentDialer
	t.Cleanup(func() { agentLookupIP, agentDialer = oldLookup, oldDialer })
	agentLookupIP = func(ctx context.Context, host string) ([]net.IPAddr, error) {
		out := make([]net.IPAddr, 0, len(ips))
		for _, s := range ips {
			out = append(out, net.IPAddr{IP: net.ParseIP(s)})
		}
		return out, nil
	}
	agentDialer = lookup
	return lookup
}

// The agent URL is validated once, when the node is saved. The stored hostname is
// resolved again on every connection, so a name that answered with a public address at
// save time can answer with cloud metadata or an internal address later. The address
// actually dialled therefore has to be re-checked at connect time.
func TestAgentDialContextRejectsRebindingToBlockedAddress(t *testing.T) {
	for _, blocked := range []string{"169.254.169.254", "10.0.0.5", "192.168.1.1", "224.0.0.1"} {
		t.Run(blocked, func(t *testing.T) {
			dialer := stubAgentNet(t, &stubDialer{}, blocked)

			_, err := agentDialContext(context.Background(), "tcp", "agent.example.com:8080")
			if err == nil {
				t.Fatalf("agent host resolving to %s was dialled", blocked)
			}
			if !strings.Contains(err.Error(), "not allowed") {
				t.Fatalf("err = %v, want a not-allowed rejection rather than a dial failure", err)
			}
			if dialer.called != "" {
				t.Fatalf("dialer was called with %q for a blocked address", dialer.called)
			}
		})
	}
}

// A permitted address must still connect, and to the address literal that was checked.
// Dialling the name again would re-resolve it and reopen the very window this closes.
func TestAgentDialContextDialsTheValidatedAddressNotTheName(t *testing.T) {
	dialer := stubAgentNet(t, &stubDialer{}, "203.0.113.10")

	_, err := agentDialContext(context.Background(), "tcp", "agent.example.com:8080")
	if !errors.Is(err, errDialed) {
		t.Fatalf("err = %v, want the dial to be attempted for a permitted address", err)
	}
	if dialer.called != "203.0.113.10:8080" {
		t.Fatalf("dialled %q, want the validated literal 203.0.113.10:8080", dialer.called)
	}
}

// Loopback stays allowed: agents are commonly co-located on the same host. This guards
// the re-check from over-blocking and breaking that documented case.
func TestAgentDialContextStillAllowsLoopbackAgent(t *testing.T) {
	dialer := stubAgentNet(t, &stubDialer{}, "127.0.0.1")

	_, err := agentDialContext(context.Background(), "tcp", "agent.example.com:8080")
	if !errors.Is(err, errDialed) {
		t.Fatalf("err = %v, want a loopback agent to remain dialable", err)
	}
	if dialer.called != "127.0.0.1:8080" {
		t.Fatalf("dialled %q, want 127.0.0.1:8080", dialer.called)
	}
}

// A literal IP in the configured URL skips DNS, so it is checked directly.
func TestAgentDialContextChecksLiteralAddress(t *testing.T) {
	dialer := stubAgentNet(t, &stubDialer{})

	_, err := agentDialContext(context.Background(), "tcp", "169.254.169.254:80")
	if err == nil || !strings.Contains(err.Error(), "not allowed") {
		t.Fatalf("err = %v, want a not-allowed rejection for a literal link-local address", err)
	}
	if dialer.called != "" {
		t.Fatalf("dialer was called with %q for a blocked literal address", dialer.called)
	}
}

// A malformed address must fail before any lookup or dial is attempted.
func TestAgentDialContextRejectsMalformedAddress(t *testing.T) {
	dialer := stubAgentNet(t, &stubDialer{}, "203.0.113.10")

	if _, err := agentDialContext(context.Background(), "tcp", "agent.example.com"); err == nil {
		t.Fatal("an address with no port was accepted")
	}
	if dialer.called != "" {
		t.Fatalf("dialer was called with %q for a malformed address", dialer.called)
	}
}

// A resolver failure is reported as-is, and nothing is dialled.
func TestAgentDialContextReportsResolverFailure(t *testing.T) {
	oldLookup, oldDialer := agentLookupIP, agentDialer
	t.Cleanup(func() { agentLookupIP, agentDialer = oldLookup, oldDialer })

	wantErr := errors.New("stub resolver: no such host")
	agentLookupIP = func(ctx context.Context, host string) ([]net.IPAddr, error) {
		return nil, wantErr
	}
	dialer := &stubDialer{}
	agentDialer = dialer

	_, err := agentDialContext(context.Background(), "tcp", "agent.example.com:8080")
	if !errors.Is(err, wantErr) {
		t.Fatalf("err = %v, want the resolver error to be propagated", err)
	}
	if dialer.called != "" {
		t.Fatalf("dialer was called with %q despite the lookup failing", dialer.called)
	}
}

// A name that resolves to nothing is an error, not a silent pass.
func TestAgentDialContextRejectsEmptyAnswer(t *testing.T) {
	dialer := stubAgentNet(t, &stubDialer{})

	_, err := agentDialContext(context.Background(), "tcp", "agent.example.com:8080")
	if err == nil || !strings.Contains(err.Error(), "no addresses") {
		t.Fatalf("err = %v, want a no-addresses rejection", err)
	}
	if dialer.called != "" {
		t.Fatalf("dialer was called with %q for an empty answer", dialer.called)
	}
}
