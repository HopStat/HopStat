package hoststats

import (
	"context"
	"testing"
)

func TestCollect(t *testing.T) {
	snap, err := Collect(context.Background())
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if snap.MemoryTotalBytes == 0 {
		t.Fatal("expected memory total > 0")
	}
	if snap.CPUCores < 1 {
		t.Fatalf("cpu cores = %d", snap.CPUCores)
	}
	if snap.CollectedAt == "" {
		t.Fatal("expected collected_at")
	}
}

func TestCollectCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Collect(ctx); err == nil {
		t.Fatal("expected error for cancelled context")
	}
}

func TestRoundHelpers(t *testing.T) {
	if round1(1.25) != 1.3 {
		t.Fatalf("round1 = %v", round1(1.25))
	}
	if roundLoad(1.234) != 1.23 {
		t.Fatalf("roundLoad = %v", roundLoad(1.234))
	}
}
