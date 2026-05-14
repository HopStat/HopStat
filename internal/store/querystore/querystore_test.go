package querystore

import (
	"testing"

	"github.com/yourorg/lg-looking-glass/internal/domain"
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
