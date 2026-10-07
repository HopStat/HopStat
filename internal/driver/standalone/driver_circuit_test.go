package standalone

import (
	"context"
	"os/exec"
	"testing"

	"github.com/HopStat/HopStat/internal/config"
	"github.com/HopStat/HopStat/internal/domain"
)

func cbTestDriver(t *testing.T) *Driver {
	t.Helper()
	d, err := NewDriver(&domain.Node{
		Name:        "n",
		Type:        domain.NodeTypeStandalone,
		Active:      true,
		EnabledCmds: []domain.CommandType{domain.CmdPing},
	}, &config.Config{})
	if err != nil {
		t.Fatal(err)
	}
	return d
}

// failAllExecs makes every command exit non-zero, which is what an unreachable host looks
// like to ping: a real answer reporting 100% loss, not an exec failure. Ping swallows that
// error and returns a synthesised result, so the caller never learns the exec failed — but
// execCmd still hands that error to the circuit breaker, which counts it (circuitbreaker.go
// lines 68-77 increment on any non-nil error).
func failAllExecs(t *testing.T) {
	t.Helper()
	old := commandContext
	t.Cleanup(func() { commandContext = old })
	commandContext = func(ctx context.Context, _ string, _ ...string) *exec.Cmd {
		return exec.CommandContext(ctx, "sh", "-c", "exit 1")
	}
}

// A Driver held across requests DOES accumulate failures and opens at the threshold.
// This pins that behaviour rather than its absence. It is unreachable today only because
// nothing reuses a Driver, so if the driver is ever cached per node this test is what
// fails, and the fact that expected packet loss is counted as a node failure has to be
// revisited rather than discovered in production.
func TestDriverCircuitOpensAfterRepeatedFailedPings(t *testing.T) {
	failAllExecs(t)
	d := cbTestDriver(t)

	const threshold = 5
	for i := 1; i < threshold; i++ {
		if _, err := d.Ping(context.Background(), "192.0.2.1", 3); err != nil {
			t.Fatalf("ping %d: %v", i, err)
		}
		if got := d.circuitBreaker.State(); got != "closed" {
			t.Fatalf("circuit opened after %d failures, before the threshold of %d", i, threshold)
		}
	}

	if _, err := d.Ping(context.Background(), "192.0.2.1", 3); err != nil {
		t.Fatalf("ping %d: %v", threshold, err)
	}
	if got := d.circuitBreaker.State(); got != "open" {
		t.Fatalf("circuit state after %d consecutive failed pings = %q, want \"open\"", threshold, got)
	}
}

// The production shape: every call site builds a Driver per request (engine.go:148 and
// :435, factory.go:17, agent_api.go:100/:140/:196, agent_stream.go:44/:96), so the
// breaker is discarded before it can reach its threshold and its failure count never
// escapes a single request. This is what keeps the test above latent rather than active,
// and pinning it means a future change to cache a Driver has to confront it.
func TestDriverCircuitNeverOpensAcrossPerRequestDrivers(t *testing.T) {
	failAllExecs(t)

	for i := 1; i <= 12; i++ {
		d := cbTestDriver(t)
		if _, err := d.Ping(context.Background(), "192.0.2.1", 3); err != nil {
			t.Fatalf("request %d: %v", i, err)
		}
		if got := d.circuitBreaker.State(); got != "closed" {
			t.Fatalf("request %d: circuit state = %q, want \"closed\"; a per-request Driver must not inherit failures", i, got)
		}
	}
}

// A successful ping resets the counter, so unreachable hosts separated by a reachable one
// do not accumulate toward the threshold.
func TestDriverCircuitResetsAfterSuccessfulPing(t *testing.T) {
	old := commandContext
	t.Cleanup(func() { commandContext = old })

	d := cbTestDriver(t)
	reachable := true
	commandContext = func(ctx context.Context, _ string, _ ...string) *exec.Cmd {
		if reachable {
			return exec.CommandContext(ctx, "sh", "-c", "exit 0")
		}
		return exec.CommandContext(ctx, "sh", "-c", "exit 1")
	}

	ping := func() {
		t.Helper()
		if _, err := d.Ping(context.Background(), "192.0.2.1", 3); err != nil {
			t.Fatalf("ping: %v", err)
		}
	}

	reachable = false
	for i := 0; i < 4; i++ {
		ping()
	}
	reachable = true
	ping()
	reachable = false
	for i := 0; i < 4; i++ {
		ping()
	}

	if got := d.circuitBreaker.State(); got != "closed" {
		t.Fatalf("state = %q after a successful ping reset the counter; want \"closed\"", got)
	}
}
