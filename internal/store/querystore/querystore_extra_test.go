package querystore

import (
	"testing"
	"time"

	"github.com/HopStat/HopStat/internal/domain"
)

func TestSignalWhenNotifyChannelFull(t *testing.T) {
	s := New()
	defer s.Stop()
	s.SetRunning("q-full")
	s.mu.RLock()
	ch := s.results["q-full"].notify
	s.mu.RUnlock()
	ch <- struct{}{}
	s.signal("q-full")
}

func TestMergePartialNotRunning(t *testing.T) {
	s := New()
	defer s.Stop()
	s.Set("done", &domain.QueryResult{ID: "done", Status: domain.StatusDone})
	s.MergePartial("done", &domain.QueryResult{Raw: "ignored"})
}

func TestAppendLineMissingID(t *testing.T) {
	s := New()
	defer s.Stop()
	s.AppendLine("missing", "line")
}

func TestSetUpdatesExistingEntry(t *testing.T) {
	s := New()
	defer s.Stop()
	s.SetRunning("q-set")
	s.Set("q-set", &domain.QueryResult{ID: "q-set", Status: domain.StatusDone, Raw: "final"})
	got, ok := s.Get("q-set")
	if !ok || got.Status != domain.StatusDone {
		t.Fatalf("got = %+v", got)
	}
}

func TestCleanupStop(t *testing.T) {
	s := New()
	s.expiry = time.Millisecond
	s.Set("x", &domain.QueryResult{ID: "x", Status: domain.StatusDone})
	time.Sleep(10 * time.Millisecond)
	s.Stop()
}
