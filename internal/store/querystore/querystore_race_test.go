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

// Set must copy on the way in, the mirror of Get copying on the way out.
//
// Set used to store the caller's pointer, and MergePartial then mutated it in
// place through mergeASPathPartialFields — which runs before the status guard,
// so on every partial. The caller keeps using its own result afterwards,
// unlocked: SubmitQuery builds the audit entry from the same struct it has
// just handed over. Observed under -race at querystore.go:137.
func TestSetDoesNotRaceWithMergePartial(t *testing.T) {
	s := New()
	defer s.Stop()

	const id = "query-1"
	s.SetRunning(id)

	// The caller's result, which it still owns and still reads after handing it over.
	shared := &domain.QueryResult{ID: id, Status: domain.StatusRunning}

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 2000; i++ {
			s.MergePartial(id, &domain.QueryResult{ASPath: []uint32{1, 2, 3}})
		}
	}()

	for i := 0; i < 2000; i++ {
		s.Set(id, shared)
		_ = shared.ASPath
		_ = shared.Status
	}

	wg.Wait()
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

// The store must not hold a reference to the caller's struct at all: mutating
// the result after Set must not change what a subsequent Get returns.
func TestSetStoresAnIndependentCopy(t *testing.T) {
	s := New()
	defer s.Stop()

	const id = "query-1"
	s.SetRunning(id)

	caller := &domain.QueryResult{ID: id, Status: domain.StatusDone, Raw: "original"}
	s.Set(id, caller)

	caller.Raw = "mutated by caller"

	got, ok := s.Get(id)
	if !ok {
		t.Fatal("Get: not found")
	}
	if got.Raw != "original" {
		t.Fatalf("stored Raw = %q after mutating the caller's result; Set must store a copy", got.Raw)
	}
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

// MergePartial must not retain the caller's slices by reference.
//
// The caller keeps filling its result after OnPartial returns — engine.go:241-242
// writes result.ASPathNodes and calls applyBGPASPath — while a concurrent Get hands
// out a shallow copy. If ASPath still pointed at the engine's backing array, the
// engine's write and the reader's got.ASPath[0] would touch the same memory.
// Observed under -race.
func TestRetainedSlicesDoNotAliasCallerMemory(t *testing.T) {
	s := New()
	defer s.Stop()

	const id = "query-1"
	s.SetRunning(id)

	enginePath := []uint32{64500, 64512}
	s.MergePartial(id, &domain.QueryResult{Status: domain.StatusRunning, ASPath: enginePath})

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 2000; i++ {
			enginePath[0] = uint32(i)
			enginePath[1] = uint32(i)
		}
	}()

	for i := 0; i < 2000; i++ {
		got, ok := s.Get(id)
		if !ok || got == nil {
			continue
		}
		if len(got.ASPath) > 0 {
			_ = got.ASPath[0]
		}
	}

	wg.Wait()
}

// The stored ASPath must be a copy: rewriting the caller's slice after the merge
// must not change what Get hands back.
func TestMergePartialCopiesRetainedSlices(t *testing.T) {
	s := New()
	defer s.Stop()

	const id = "query-1"
	s.SetRunning(id)

	enginePath := []uint32{64500, 64512}
	rules := []*domain.CommunityRule{{ID: 1, Community: "no-export"}}
	s.MergePartial(id, &domain.QueryResult{
		Status:       domain.StatusRunning,
		ASPath:       enginePath,
		MatchedRules: rules,
	})

	enginePath[0] = 13335

	got, ok := s.Get(id)
	if !ok || got == nil {
		t.Fatal("Get: not found")
	}
	if len(got.ASPath) != 2 {
		t.Fatalf("stored ASPath length = %d, want 2", len(got.ASPath))
	}
	if got.ASPath[0] != 64500 {
		t.Fatalf("stored ASPath[0] = %d after the caller rewrote its slice; MergePartial must copy", got.ASPath[0])
	}
	if len(got.MatchedRules) != 1 || got.MatchedRules[0] != rules[0] {
		t.Fatal("stored MatchedRules must still reference the same rule objects")
	}
}
