package querystore

import (
	"testing"

	"github.com/HopStat/HopStat/internal/domain"
)

func TestSignalMissingNotify(t *testing.T) {
	s := New()
	defer s.Stop()
	s.mu.Lock()
	s.results["no-notify"] = &entry{result: &domain.QueryResult{ID: "no-notify", Status: domain.StatusRunning}, notify: nil}
	s.mu.Unlock()
	s.signal("no-notify")
}
