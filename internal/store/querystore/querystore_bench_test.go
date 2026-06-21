package querystore_test

import (
	"testing"

	"github.com/HopStat/HopStat/internal/domain"
	"github.com/HopStat/HopStat/internal/store/querystore"
)

func TestStoreNotifyOnPartial(t *testing.T) {
	s := querystore.New()
	defer s.Stop()
	s.SetRunning("q1")

	ch := s.NotifyCh("q1")
	s.MergePartial("q1", &domain.QueryResult{
		ASPath:       []uint32{64500, 64501},
		ASPathPrefix: "203.0.113.0/24",
	})

	select {
	case <-ch:
	default:
		t.Fatal("expected notify on partial merge")
	}
}

func BenchmarkAppendLine(b *testing.B) {
	s := querystore.New()
	defer s.Stop()
	s.SetRunning("bench")
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		s.AppendLine("bench", "1  1.1.1.1  1.234 ms")
	}
}
