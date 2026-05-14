package middleware

import (
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

type BruteForceGuard struct {
	mu       sync.Mutex
	attempts map[string]*attemptInfo
	max      int
	banDur   time.Duration
	stopCh   chan struct{}
}

type attemptInfo struct {
	count    int
	bannedAt time.Time
	lastTry  time.Time
}

func NewBruteForceGuard(max int, banMinutes int) *BruteForceGuard {
	g := &BruteForceGuard{
		attempts: make(map[string]*attemptInfo),
		max:      max,
		banDur:   time.Duration(banMinutes) * time.Minute,
		stopCh:   make(chan struct{}),
	}
	go g.cleanup()
	return g
}

// Stop terminates the background cleanup goroutine.
func (g *BruteForceGuard) Stop() {
	close(g.stopCh)
}

func (g *BruteForceGuard) cleanup() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-g.stopCh:
			return
		case <-ticker.C:
			g.mu.Lock()
			now := time.Now()
			for ip, info := range g.attempts {
				// Remove entries where the ban has expired
				if !info.bannedAt.IsZero() && now.Sub(info.bannedAt) >= g.banDur {
					delete(g.attempts, ip)
					continue
				}
				// Remove entries with no ban that haven't been attempted in the last 30 minutes
				if info.bannedAt.IsZero() && now.Sub(info.lastTry) > 30*time.Minute {
					delete(g.attempts, ip)
				}
			}
			g.mu.Unlock()
		}
	}
}

func (g *BruteForceGuard) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		ip := c.ClientIP()

		g.mu.Lock()
		info, exists := g.attempts[ip]
		if exists && !info.bannedAt.IsZero() && time.Since(info.bannedAt) < g.banDur {
			g.mu.Unlock()
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"error": "too many failed login attempts, please try again later",
			})
			return
		}
		if exists && !info.bannedAt.IsZero() && time.Since(info.bannedAt) >= g.banDur {
			delete(g.attempts, ip)
		}
		g.mu.Unlock()

		c.Next()

		if c.Writer.Status() == http.StatusUnauthorized {
			g.mu.Lock()
			info := g.attempts[ip]
			if info == nil {
				info = &attemptInfo{}
				g.attempts[ip] = info
			}
			info.count++
			info.lastTry = time.Now()
			if info.count >= g.max {
				info.bannedAt = time.Now()
			}
			g.mu.Unlock()
		} else if c.Writer.Status() == http.StatusOK {
			g.mu.Lock()
			delete(g.attempts, ip)
			g.mu.Unlock()
		}
	}
}
