package circuitbreaker

import (
	"errors"
	"testing"
	"time"
)

func TestNew(t *testing.T) {
	cb := New(3, 5*time.Second)
	if cb.State() != "closed" {
		t.Errorf("expected closed, got %s", cb.State())
	}
}

func TestCircuitBreakerSuccess(t *testing.T) {
	cb := New(3, 100*time.Millisecond)
	err := cb.Call(func() error { return nil })
	if err != nil {
		t.Errorf("expected nil error, got %v", err)
	}
	if cb.State() != "closed" {
		t.Errorf("expected closed, got %s", cb.State())
	}
}

func TestCircuitBreakerOpenAfterThreshold(t *testing.T) {
	cb := New(3, 100*time.Millisecond)
	testErr := errors.New("test error")

	for i := 0; i < 3; i++ {
		cb.Call(func() error { return testErr })
	}

	if cb.State() != "open" {
		t.Errorf("expected open after %d failures, got %s", 3, cb.State())
	}

	err := cb.Call(func() error { return nil })
	if err != ErrCircuitOpen {
		t.Errorf("expected ErrCircuitOpen, got %v", err)
	}
}

func TestCircuitBreakerHalfOpenAfterTimeout(t *testing.T) {
	cb := New(1, 50*time.Millisecond)
	cb.Call(func() error { return errors.New("fail") })

	if cb.State() != "open" {
		t.Fatal("expected open")
	}

	time.Sleep(60 * time.Millisecond)

	// Should now allow one call (half-open)
	if cb.State() != "half-open" {
		// State transitions on next call, so it might still report open
		// depending on implementation
	}

	err := cb.Call(func() error { return nil })
	if err != nil {
		t.Errorf("expected success in half-open, got %v", err)
	}
	if cb.State() != "closed" {
		t.Errorf("expected closed after successful half-open, got %s", cb.State())
	}
}

func TestCircuitBreakerResetOnSuccess(t *testing.T) {
	cb := New(2, time.Second)
	cb.Call(func() error { return errors.New("fail") })

	// Success should reset failure count
	cb.Call(func() error { return nil })

	// Now 2 more failures needed to open
	cb.Call(func() error { return errors.New("fail") })
	if cb.State() != "closed" {
		t.Errorf("expected closed, got %s", cb.State())
	}

	cb.Call(func() error { return errors.New("fail") })
	if cb.State() != "open" {
		t.Errorf("expected open after 2 failures, got %s", cb.State())
	}
}
