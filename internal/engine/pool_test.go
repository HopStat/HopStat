package engine

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func TestQueryPoolExecute(t *testing.T) {
	pool := NewQueryPool(2)
	var ran int32

	err := pool.Execute(context.Background(), func() error {
		atomic.AddInt32(&ran, 1)
		return nil
	})

	if err != nil {
		t.Errorf("expected nil error, got %v", err)
	}
	if atomic.LoadInt32(&ran) != 1 {
		t.Errorf("expected 1 run, got %d", ran)
	}
}

func TestQueryPoolConcurrency(t *testing.T) {
	pool := NewQueryPool(2)
	var running int32
	var maxRunning int32

	for i := 0; i < 5; i++ {
		pool.Execute(context.Background(), func() error {
			cur := atomic.AddInt32(&running, 1)
			for {
				old := atomic.LoadInt32(&maxRunning)
				if cur <= old || atomic.CompareAndSwapInt32(&maxRunning, old, cur) {
					break
				}
			}
			time.Sleep(50 * time.Millisecond)
			atomic.AddInt32(&running, -1)
			return nil
		})
	}

	time.Sleep(200 * time.Millisecond)
	if atomic.LoadInt32(&maxRunning) > 2 {
		t.Errorf("expected max 2 concurrent, got %d", maxRunning)
	}
}

func TestQueryPoolContextCancel(t *testing.T) {
	pool := NewQueryPool(1)

	// Fill the pool so the next request must wait on the semaphore
	pool.sem <- struct{}{} // occupy the only slot

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := pool.Execute(ctx, func() error { return nil })
	if err == nil {
		t.Errorf("expected error for cancelled context with full pool, got nil")
	}

	// Release the slot
	<-pool.sem
}
