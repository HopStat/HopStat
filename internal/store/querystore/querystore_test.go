package querystore

import (
	"testing"
	"time"

	"github.com/HopStat/HopStat/internal/domain"
)

func TestSetAndGet(t *testing.T) {
	s := New()
	defer s.Stop()

	r := &domain.QueryResult{
		ID:         "q-1",
		Status:     domain.StatusDone,
		Raw:        "raw output",
		Parsed:     map[string]string{"key": "val"},
		DurationMS: 42,
	}
	s.Set("q-1", r)

	got, ok := s.Get("q-1")
	if !ok {
		t.Fatal("expected to find result for id q-1, got false")
	}
	if got.ID != r.ID {
		t.Errorf("ID = %q, want %q", got.ID, r.ID)
	}
	if got.Status != r.Status {
		t.Errorf("Status = %q, want %q", got.Status, r.Status)
	}
	if got.Raw != r.Raw {
		t.Errorf("Raw = %q, want %q", got.Raw, r.Raw)
	}
	if got.DurationMS != r.DurationMS {
		t.Errorf("DurationMS = %d, want %d", got.DurationMS, r.DurationMS)
	}
}

func TestGetNonExistent(t *testing.T) {
	s := New()
	defer s.Stop()

	got, ok := s.Get("no-such-id")
	if ok {
		t.Error("expected ok=false for missing id, got true")
	}
	if got != nil {
		t.Errorf("expected nil result, got %+v", got)
	}
}

func TestDelete(t *testing.T) {
	s := New()
	defer s.Stop()

	r := &domain.QueryResult{
		ID:     "q-del",
		Status: domain.StatusDone,
	}
	s.Set("q-del", r)

	_, ok := s.Get("q-del")
	if !ok {
		t.Fatal("expected result to exist before delete")
	}

	s.Delete("q-del")

	_, ok = s.Get("q-del")
	if ok {
		t.Error("expected result to be gone after delete")
	}
}

func TestStop(t *testing.T) {
	s := New()
	s.Stop()
	// If Stop() panics or deadlocks, the test will hang or fail.
}

func TestMarkOutputComplete(t *testing.T) {
	s := New()
	defer s.Stop()

	s.SetRunning("q-out")
	if s.IsOutputComplete("q-out") {
		t.Fatal("expected outputComplete=false before mark")
	}

	s.MarkOutputComplete("q-out")
	if !s.IsOutputComplete("q-out") {
		t.Fatal("expected outputComplete=true after mark")
	}
	if s.IsOutputComplete("missing") {
		t.Fatal("expected false for missing id")
	}
}

func TestNotifyChSignalsOnAppendLine(t *testing.T) {
	s := New()
	defer s.Stop()

	s.SetRunning("q-notify")
	ch := s.NotifyCh("q-notify")

	s.AppendLine("q-notify", "line 1")
	select {
	case <-ch:
	case <-time.After(time.Second):
		t.Fatal("expected notify after append line")
	}
}

func TestOverwrite(t *testing.T) {
	s := New()
	defer s.Stop()

	first := &domain.QueryResult{
		ID:         "q-ow",
		Status:     domain.StatusDone,
		Raw:        "first",
		DurationMS: 10,
	}
	s.Set("q-ow", first)

	second := &domain.QueryResult{
		ID:         "q-ow",
		Status:     domain.StatusDone,
		Raw:        "second",
		DurationMS: 20,
	}
	s.Set("q-ow", second)

	got, ok := s.Get("q-ow")
	if !ok {
		t.Fatal("expected to find result after overwrite")
	}
	if got.Raw != "second" {
		t.Errorf("Raw = %q, want %q (overwritten value)", got.Raw, "second")
	}
	if got.DurationMS != 20 {
		t.Errorf("DurationMS = %d, want %d (overwritten value)", got.DurationMS, 20)
	}
}

func TestGetLines(t *testing.T) {
	s := New()
	defer s.Stop()

	s.SetRunning("q-lines")
	s.AppendLine("q-lines", "a")
	s.AppendLine("q-lines", "b")

	lines, ok := s.GetLines("q-lines")
	if !ok || len(lines) != 2 || lines[0] != "a" {
		t.Fatalf("GetLines = %#v ok=%v", lines, ok)
	}
	if _, ok := s.GetLines("missing"); ok {
		t.Fatal("expected false for missing id")
	}
}

func TestMergePartial(t *testing.T) {
	s := New()
	defer s.Stop()

	s.SetRunning("q-merge")
	ch := s.NotifyCh("q-merge")

	s.MergePartial("q-merge", nil)
	s.MergePartial("missing", &domain.QueryResult{Raw: "x"})

	partial := &domain.QueryResult{
		Parsed:         map[string]string{"k": "v"},
		Raw:            "partial raw",
		ASPath:         []uint32{65001, 65002},
		ASPathPrefix:   "8.8.8.0/24",
		ASPathEnriched: []domain.ASInfo{{ASN: 65001}},
		MatchedRules:   []*domain.CommunityRule{{Community: "65001:100"}},
	}
	s.MergePartial("q-merge", partial)

	got, ok := s.Get("q-merge")
	if !ok {
		t.Fatal("expected result")
	}
	if got.Raw != "partial raw" || got.ASPathPrefix != "8.8.8.0/24" || len(got.MatchedRules) != 1 {
		t.Fatalf("merged = %+v", got)
	}

	s.Set("q-done", &domain.QueryResult{ID: "q-done", Status: domain.StatusDone})
	s.MergePartial("q-done", &domain.QueryResult{Raw: "ignored"})
	select {
	case <-ch:
	case <-time.After(time.Second):
		t.Fatal("expected notify after merge")
	}
}

func TestNotifyChMissingID(t *testing.T) {
	s := New()
	defer s.Stop()
	ch := s.NotifyCh("missing")
	select {
	case <-ch:
	default:
		t.Fatal("expected closed notify channel")
	}
}

func TestCleanupRemovesExpired(t *testing.T) {
	s := New()
	s.expiry = 5 * time.Millisecond
	s.Set("old", &domain.QueryResult{ID: "old", Status: domain.StatusDone})
	time.Sleep(15 * time.Millisecond)
	if _, ok := s.Get("old"); ok {
		t.Fatal("expected expired entry to be removed")
	}
	s.Stop()
}

func TestMergeASPathPartialFieldsCarriesNodeMap(t *testing.T) {
	dest := &domain.QueryResult{}
	mergeASPathPartialFields(dest, &domain.QueryResult{
		ASPathNodes: []domain.NodeASPath{{NodeID: 1, NodeName: "BURSA"}},
	})
	if len(dest.ASPathNodes) != 1 {
		t.Fatalf("dest = %+v", dest.ASPathNodes)
	}

	// An empty partial must not wipe what is already there.
	mergeASPathPartialFields(dest, &domain.QueryResult{})
	if len(dest.ASPathNodes) != 1 {
		t.Fatalf("node map cleared by empty partial: %+v", dest.ASPathNodes)
	}
}
