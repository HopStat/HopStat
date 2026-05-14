package engine

import (
	"testing"
	"time"
)

func TestRateLimiterAllow(t *testing.T) {
	rl := NewRateLimiter(3, time.Minute)
	defer rl.Stop()

	for i := 0; i < 3; i++ {
		if !rl.Allow("1.2.3.4") {
			t.Errorf("request %d should be allowed", i+1)
		}
	}

	if rl.Allow("1.2.3.4") {
		t.Error("4th request should be denied")
	}
}

func TestRateLimiterDifferentIPs(t *testing.T) {
	rl := NewRateLimiter(2, time.Minute)
	defer rl.Stop()

	rl.Allow("1.1.1.1")
	rl.Allow("1.1.1.1")

	if !rl.Allow("2.2.2.2") {
		t.Error("different IP should be allowed")
	}
}

func TestRateLimiterWindowExpiry(t *testing.T) {
	rl := NewRateLimiter(1, 100*time.Millisecond)
	defer rl.Stop()

	if !rl.Allow("1.1.1.1") {
		t.Error("first request should be allowed")
	}
	if rl.Allow("1.1.1.1") {
		t.Error("second request should be denied")
	}

	time.Sleep(150 * time.Millisecond)

	if !rl.Allow("1.1.1.1") {
		t.Error("request after window expiry should be allowed")
	}
}
