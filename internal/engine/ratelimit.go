package engine

import (
	"time"

	"github.com/HopStat/HopStat/internal/server/middleware"
)

// RateLimiter wraps the middleware rate limiter for use in the engine.
type RateLimiter = middleware.RateLimiter

func NewRateLimiter(limit int, window time.Duration) *RateLimiter {
	return middleware.NewRateLimiter(limit, window)
}
