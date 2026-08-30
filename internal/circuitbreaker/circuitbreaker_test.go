package circuitbreaker

import (
	"errors"
	"sync"
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

	// Not asserted: the breaker only moves to half-open on the next call, so right after
	// the cool-off it may legitimately still report "open".

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

func TestCircuitBreakerStateOpen(t *testing.T) {
	cb := New(1, time.Second)
	cb.Call(func() error { return errors.New("fail") })
	if cb.State() != "open" {
		t.Fatalf("expected open, got %s", cb.State())
	}
}

func TestCircuitBreakerStateHalfOpen(t *testing.T) {
	cb := New(1, 10*time.Millisecond)
	cb.Call(func() error { return errors.New("fail") })
	time.Sleep(15 * time.Millisecond)

	block := make(chan struct{})
	done := make(chan struct{})
	go func() {
		_ = cb.Call(func() error {
			close(done)
			<-block
			return nil
		})
	}()
	<-done
	if cb.State() != "half-open" {
		t.Fatalf("expected half-open, got %s", cb.State())
	}
	close(block)
}

func TestCircuitBreakerHalfOpenBlocksConcurrentProbe(t *testing.T) {
	cb := New(1, 10*time.Millisecond)
	cb.Call(func() error { return errors.New("fail") })
	time.Sleep(15 * time.Millisecond)

	block := make(chan struct{})
	release := make(chan struct{})
	started := make(chan struct{}, 1)
	go func() {
		_ = cb.Call(func() error {
			started <- struct{}{}
			<-block
			<-release
			return nil
		})
	}()
	<-started

	err := cb.Call(func() error { return nil })
	if err != ErrCircuitOpen {
		t.Fatalf("expected ErrCircuitOpen for concurrent half-open probe, got %v", err)
	}
	close(block)
	close(release)
}

func TestCircuitBreakerHalfOpenSeqMismatch(t *testing.T) {
	cb := New(1, 10*time.Millisecond)
	cb.Call(func() error { return errors.New("fail") })
	time.Sleep(15 * time.Millisecond)

	block := make(chan struct{})
	var probeErr error
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		probeErr = cb.Call(func() error {
			cb.mu.Lock()
			cb.halfOpenSeq++
			cb.mu.Unlock()
			<-block
			return errors.New("still failing")
		})
	}()

	time.Sleep(20 * time.Millisecond)
	close(block)
	wg.Wait()

	if probeErr == nil {
		t.Fatal("expected probe error from half-open failure")
	}
}

func TestCircuitBreakerStateUnknown(t *testing.T) {
	cb := New(1, time.Second)
	cb.state = cbState(99)
	if cb.State() != "unknown" {
		t.Fatalf("expected unknown, got %s", cb.State())
	}
}
