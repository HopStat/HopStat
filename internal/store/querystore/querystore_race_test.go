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

// Parsed must not alias the caller's result struct.
//
// engine.go:211/:220/:232 assign *PingResult, *TracerouteResult and *BGPResult
// into result.Parsed; the engine keeps writing that struct after OnPartial returns,
// while a concurrent GetResult dereferences cp.Parsed. Observed under -race: the
// engine's br.Raw write against the reader's parsed.Raw on the same memory.
func TestParsedDoesNotRaceWithCallerWritingResult(t *testing.T) {
	s := New()
	defer s.Stop()

	const id = "query-1"
	s.SetRunning(id)

	br := &domain.BGPResult{Raw: "initial", Routes: []domain.BGPRoute{{Prefix: "10.0.0.0/8"}}}
	s.MergePartial(id, &domain.QueryResult{Status: domain.StatusRunning, Parsed: br})

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 2000; i++ {
			br.Raw = "engine still writing"
			br.Routes[0].Best = i%2 == 0
		}
	}()

	for i := 0; i < 2000; i++ {
		got, ok := s.Get(id)
		if !ok || got == nil {
			continue
		}
		if parsed, isBGP := got.Parsed.(*domain.BGPResult); isBGP && parsed != nil {
			_ = parsed.Raw
		}
	}

	wg.Wait()
}

// Every type the engine can put in Parsed must be copied, and an unknown type must
// pass through untouched rather than being mangled. The nil cases cover a typed nil
// pointer, which is non-nil as an interface and so reaches the type switch.
func TestCloneParsedCoversEveryKnownType(t *testing.T) {
	s := New()
	defer s.Stop()

	t.Run("ping", func(t *testing.T) {
		const id = "q-ping"
		s.SetRunning(id)
		orig := &domain.PingResult{Raw: "p", AvgRTT: 12.5}
		s.MergePartial(id, &domain.QueryResult{Status: domain.StatusRunning, Parsed: orig})
		orig.Raw = "changed"
		got, _ := s.Get(id)
		if p, ok := got.Parsed.(*domain.PingResult); !ok || p.Raw != "p" {
			t.Fatalf("PingResult not copied: %+v", got.Parsed)
		}
	})

	t.Run("traceroute", func(t *testing.T) {
		const id = "q-tr"
		s.SetRunning(id)
		orig := &domain.TracerouteResult{Raw: "t", Hops: []domain.Hop{{Number: 1, IP: "10.0.0.1"}}}
		s.MergePartial(id, &domain.QueryResult{Status: domain.StatusRunning, Parsed: orig})
		orig.Hops[0].IP = "changed"
		got, _ := s.Get(id)
		if tr, ok := got.Parsed.(*domain.TracerouteResult); !ok || tr.Hops[0].IP != "10.0.0.1" {
			t.Fatalf("TracerouteResult not copied: %+v", got.Parsed)
		}
	})

	// A typed nil pointer is a non-nil interface holding a nil pointer, so the
	// returned interface compares != nil even though the value it holds does not.
	// Assert on the concrete pointer, which is what actually matters.
	t.Run("typed nil is preserved", func(t *testing.T) {
		if c, ok := cloneParsed((*domain.PingResult)(nil)).(*domain.PingResult); !ok || c != nil {
			t.Fatalf("cloneParsed(typed nil PingResult) = %v, want (*PingResult)(nil)", c)
		}
		if c, ok := cloneParsed((*domain.TracerouteResult)(nil)).(*domain.TracerouteResult); !ok || c != nil {
			t.Fatalf("cloneParsed(typed nil TracerouteResult) = %v, want (*TracerouteResult)(nil)", c)
		}
		if c, ok := cloneParsed((*domain.BGPResult)(nil)).(*domain.BGPResult); !ok || c != nil {
			t.Fatalf("cloneParsed(typed nil BGPResult) = %v, want (*BGPResult)(nil)", c)
		}
	})

	t.Run("unknown type passes through", func(t *testing.T) {
		unknown := map[string]string{"a": "b"}
		if c := cloneParsed(unknown); c == nil {
			t.Fatal("cloneParsed dropped an unknown type")
		}
	})
}
