package lgnode

import (
	"context"
	"errors"
	"testing"

	"github.com/HopStat/HopStat/internal/circuitbreaker"
	"github.com/HopStat/HopStat/internal/config"
	"github.com/HopStat/HopStat/internal/domain"
)

func cbTestLgnodeDriver(t *testing.T) *Driver {
	t.Helper()
	d, err := NewDriver(&domain.Node{
		Name:        "agent",
		Type:        domain.NodeTypeLGNode,
		Active:      true,
		AgentURL:    "http://agent.example.com:8080",
		AgentToken:  "tok",
		EnabledCmds: []domain.CommandType{domain.CmdPing},
	}, &config.Config{})
	if err != nil {
		t.Fatal(err)
	}
	return d
}

// A Driver held across requests DOES accumulate failures and opens at the threshold.
// This mirrors internal/driver/standalone/driver_circuit_test.go for the agent driver, so
// caching a Driver per node fails a test rather than failing silently in production.
//
// Unlike the standalone driver there is no semantic mismatch here: every lgnode path
// reports a transport failure as an error, so the breaker only ever sees genuine agent
// failures. The threshold is inert purely because of the per-request allocation.
func TestLgnodeDriverCircuitOpensAfterFiveFailedRequests(t *testing.T) {
	stubAgentNet(t, &stubDialer{}, "203.0.113.10")
	d := cbTestLgnodeDriver(t)

	const threshold = 5
	for i := 1; i < threshold; i++ {
		if err := d.TestConnection(context.Background()); err == nil {
			t.Fatalf("request %d: want a dial error, got nil", i)
		}
		if got := d.circuitBreaker.State(); got != "closed" {
			t.Fatalf("circuit opened after %d failures, before the threshold of %d", i, threshold)
		}
	}

	if err := d.TestConnection(context.Background()); err == nil {
		t.Fatalf("request %d: want a dial error, got nil", threshold)
	}
	if got := d.circuitBreaker.State(); got != "open" {
		t.Fatalf("circuit state after %d consecutive failed requests = %q, want \"open\"", threshold, got)
	}

	// Once open the breaker short-circuits without dialling at all.
	if err := d.TestConnection(context.Background()); !errors.Is(err, circuitbreaker.ErrCircuitOpen) {
		t.Fatalf("request after open: err = %v, want ErrCircuitOpen", err)
	}
}

// The production shape: factory.go:19 hands back a freshly allocated Driver, so the
// breaker is discarded before it can reach its threshold and its failure count never
// escapes a single request. Every request must therefore surface the real dial failure,
// never the breaker short-circuit.
func TestLgnodeDriverCircuitNeverOpensAcrossPerRequestDrivers(t *testing.T) {
	stubAgentNet(t, &stubDialer{}, "203.0.113.10")

	for i := 1; i <= 12; i++ {
		d := cbTestLgnodeDriver(t)
		err := d.TestConnection(context.Background())
		if err == nil {
			t.Fatalf("request %d: want a dial error, got nil", i)
		}
		if errors.Is(err, circuitbreaker.ErrCircuitOpen) {
			t.Fatalf("request %d: err = ErrCircuitOpen; a per-request Driver must not inherit a previous request's failures", i)
		}
		if got := d.circuitBreaker.State(); got != "closed" {
			t.Fatalf("request %d: circuit state = %q, want \"closed\"; a per-request Driver must not inherit failures", i, got)
		}
	}
}
