package querystore

import (
	"sync"
	"testing"

	"github.com/HopStat/HopStat/internal/domain"
)

// Get must hand back a copy, not the stored pointer.
//
// MergePartial writes fields into e.result in place while holding the store
// lock. Every caller of Get reads the result after Get has already released
// that lock — GetResult serialises it straight into the HTTP response — so
// returning e.result races the query goroutine's OnPartial writes. Under
// -race this reported a read/write conflict on the same struct field.
func TestGetDoesNotRaceWithMergePartial(t *testing.T) {
	s := New()
	defer s.Stop()

	const id = "query-1"
	s.SetRunning(id)

	var wg sync.WaitGroup

	// The query goroutine: partial results keep arriving while the query runs.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 2000; i++ {
			s.MergePartial(id, &domain.QueryResult{Raw: "partial output"})
		}
	}()

	// The reader: exactly what GetResult does — fetch the result, then read a field.
	for i := 0; i < 2000; i++ {
		res, ok := s.Get(id)
		if !ok || res == nil {
			continue
		}
		_ = res.Raw
	}

	wg.Wait()
}

// An entry can hold a nil result: Set is called with whatever the engine returned,
// which is nil when execution failed. Get must report that as found-but-nil rather
// than dereferencing it.
func TestGetHandlesNilStoredResult(t *testing.T) {
	s := New()
	defer s.Stop()

	const id = "query-nil"
	s.Set(id, nil)

	got, ok := s.Get(id)
	if !ok {
		t.Fatal("Get reported not-found for an entry that exists")
	}
	if got != nil {
		t.Fatalf("Get = %+v, want nil for a nil stored result", got)
	}
}

// The copy must be a copy: mutating what Get returned must not corrupt the
// stored result, or a caller could edit the entry without the store's lock.
func TestGetReturnsAnIndependentCopy(t *testing.T) {
	s := New()
	defer s.Stop()

	const id = "query-1"
	s.SetRunning(id)
	s.MergePartial(id, &domain.QueryResult{Raw: "original"})

	got, ok := s.Get(id)
	if !ok {
		t.Fatal("Get: not found")
	}
	if got.Raw != "original" {
		t.Fatalf("Raw = %q, want %q", got.Raw, "original")
	}

	got.Raw = "mutated by caller"

	again, ok := s.Get(id)
	if !ok {
		t.Fatal("Get: not found")
	}
	if again.Raw != "original" {
		t.Fatalf("stored Raw = %q after mutating the returned value; Get must return a copy", again.Raw)
	}
}
