package middleware

import (
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

type RateLimiter struct {
	mu       sync.Mutex
	requests map[string][]time.Time
	limit    int
	window   time.Duration
	stopCh   chan struct{}
}

func NewRateLimiter(limit int, window time.Duration) *RateLimiter {
	rl := &RateLimiter{
		requests: make(map[string][]time.Time),
		limit:    limit,
		window:   window,
		stopCh:   make(chan struct{}),
	}
	go rl.cleanup(rateLimitCleanupInterval)
	return rl
}

const defaultMaxTrackedIPs = 100000

var rateLimitMaxTrackedIPs = defaultMaxTrackedIPs

func (rl *RateLimiter) Allow(ip string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	windowStart := now.Add(-rl.window)

	var valid []time.Time
	for _, t := range rl.requests[ip] {
		if t.After(windowStart) {
			valid = append(valid, t)
		}
	}

	if len(valid) >= rl.limit {
		rl.requests[ip] = valid
		return false
	}

	// Cap total tracked IPs to prevent OOM under IP-rotation attacks.
	// Reject unseen IPs once the map is full instead of bypassing rate limits.
	if _, exists := rl.requests[ip]; !exists && len(rl.requests) >= rateLimitMaxTrackedIPs {
		return false
	}

	valid = append(valid, now)
	rl.requests[ip] = valid
	return true
}

func (rl *RateLimiter) Stop() {
	close(rl.stopCh)
}

var rateLimitCleanupInterval = time.Minute

func (rl *RateLimiter) cleanup(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-rl.stopCh:
			return
		case <-ticker.C:
			rl.mu.Lock()
			now := time.Now()
			windowStart := now.Add(-rl.window)
			for ip, times := range rl.requests {
				var valid []time.Time
				for _, t := range times {
					if t.After(windowStart) {
						valid = append(valid, t)
					}
				}
				if len(valid) == 0 {
					delete(rl.requests, ip)
				} else {
					rl.requests[ip] = valid
				}
			}
			rl.mu.Unlock()
		}
	}
}

func RateLimit(limit int) gin.HandlerFunc {
	rl := NewRateLimiter(limit, time.Minute)

	return func(c *gin.Context) {
		ip := c.ClientIP()
		if !rl.Allow(ip) {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"error": gin.H{
					"code":    "RATE_LIMITED",
					"message": "too many requests",
				},
			})
			return
		}
		c.Next()
	}
}

// OptionalRateLimit applies per-IP HTTP rate limiting when enabled and limit > 0.
func OptionalRateLimit(enabled bool, limit int) gin.HandlerFunc {
	if !enabled || limit <= 0 {
		return func(c *gin.Context) { c.Next() }
	}
	return RateLimit(limit)
}
