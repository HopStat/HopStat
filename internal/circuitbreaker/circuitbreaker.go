package circuitbreaker

import (
	"errors"
	"sync"
	"time"
)

var ErrCircuitOpen = errors.New("circuit breaker open")

type CircuitBreaker struct {
	mu          sync.Mutex
	failures    int
	lastFailure time.Time
	state       cbState
	threshold   int
	resetTimeout time.Duration
	halfOpenSeq uint64
}

type cbState int

const (
	cbClosed cbState = iota
	cbOpen
	cbHalfOpen
)

func New(threshold int, resetTimeout time.Duration) *CircuitBreaker {
	return &CircuitBreaker{
		threshold:    threshold,
		resetTimeout: resetTimeout,
		state:        cbClosed,
	}
}

func (cb *CircuitBreaker) Call(fn func() error) error {
	cb.mu.Lock()
	if cb.state == cbOpen && time.Since(cb.lastFailure) < cb.resetTimeout {
		cb.mu.Unlock()
		return ErrCircuitOpen
	}
	if cb.state == cbOpen {
		cb.state = cbHalfOpen
		cb.halfOpenSeq++
	}
	seq := cb.halfOpenSeq
	cb.mu.Unlock()

	err := fn()

	cb.mu.Lock()
	// Only update state if we're still in the same half-open sequence
	if cb.state == cbHalfOpen && cb.halfOpenSeq != seq {
		// Another goroutine transitioned to a new half-open; treat as closed reset
		cb.mu.Unlock()
		return err
	}
	if err != nil {
		cb.failures++
		cb.lastFailure = time.Now()
		if cb.failures >= cb.threshold {
			cb.state = cbOpen
		}
		cb.mu.Unlock()
		return err
	}

	cb.failures = 0
	cb.state = cbClosed
	cb.mu.Unlock()

	return nil
}

func (cb *CircuitBreaker) State() string {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	switch cb.state {
	case cbClosed:
		return "closed"
	case cbOpen:
		return "open"
	case cbHalfOpen:
		return "half-open"
	}
	return "unknown"
}
