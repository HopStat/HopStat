package bgp

import (
	"fmt"
	"testing"
)

func TestEventLogRingBuffer(t *testing.T) {
	log := newEventLog()
	for i := 0; i < maxLogEntriesPerNeighbor+10; i++ {
		log.add(1, "info", fmt.Sprintf("event-%d", i), "")
	}

	entries := log.list(1, 0)
	if len(entries) != maxLogEntriesPerNeighbor {
		t.Fatalf("len = %d, want %d", len(entries), maxLogEntriesPerNeighbor)
	}
	if entries[0].Message != "event-10" {
		t.Fatalf("oldest entry = %q, want event-10", entries[0].Message)
	}
	if entries[len(entries)-1].Message != fmt.Sprintf("event-%d", maxLogEntriesPerNeighbor+9) {
		t.Fatalf("latest entry = %q", entries[len(entries)-1].Message)
	}

	if got := log.list(99, 10); len(got) != 0 {
		t.Fatalf("unknown neighbor should return empty list")
	}
}

func TestEventLogRemove(t *testing.T) {
	log := newEventLog()
	log.add(1, "info", "hello", "")
	log.remove(1)
	if got := log.list(1, 10); len(got) != 0 {
		t.Fatalf("expected empty log after remove, got %d entries", len(got))
	}
}
