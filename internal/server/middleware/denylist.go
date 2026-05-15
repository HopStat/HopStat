package middleware

import (
	"sync"
	"time"
)

// JTIDenyList tracks revoked JWT IDs so that logged-out tokens cannot be reused.
// Entries expire automatically when the corresponding JWT would have expired anyway.
type JTIDenyList struct {
	mu      sync.Mutex
	entries map[string]time.Time // jti → token expiry
}

func NewJTIDenyList() *JTIDenyList {
	dl := &JTIDenyList{entries: make(map[string]time.Time)}
	go dl.purgeLoop()
	return dl
}

func (d *JTIDenyList) Revoke(jti string, exp time.Time) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.entries[jti] = exp
}

func (d *JTIDenyList) IsRevoked(jti string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	exp, ok := d.entries[jti]
	if !ok {
		return false
	}
	if time.Now().After(exp) {
		delete(d.entries, jti)
		return false
	}
	return true
}

func (d *JTIDenyList) purgeLoop() {
	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		now := time.Now()
		d.mu.Lock()
		for jti, exp := range d.entries {
			if now.After(exp) {
				delete(d.entries, jti)
			}
		}
		d.mu.Unlock()
	}
}
