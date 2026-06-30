package middleware

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestBruteForceGuard_ExpiredBanAllowsRetry(t *testing.T) {
	guard := NewBruteForceGuard(1, 1)
	defer guard.Stop()
	guard.mu.Lock()
	guard.attempts["1.2.3.4"] = &attemptInfo{bannedAt: time.Now().Add(-2 * time.Minute), count: 2}
	guard.mu.Unlock()

	handler := guard.Middleware()
	req := httptest.NewRequest(http.MethodPost, "/login", nil)
	req.RemoteAddr = "1.2.3.4:1"
	c, w := newTestContext(req)
	handler(c)
	if w.Code == http.StatusTooManyRequests {
		t.Fatal("expected expired ban to allow request")
	}
}

func TestJTIDenyList_RevokeAtCapacity(t *testing.T) {
	dl := NewJTIDenyList()
	future := time.Now().Add(time.Hour)
	for i := 0; i < maxDenyListEntries; i++ {
		dl.Revoke(fmt.Sprintf("jti-cap-%d", i), future)
	}
	dl.Revoke("should-not-add", future)
	if dl.IsRevoked("should-not-add") {
		t.Fatal("expected revocation to be dropped when full and no expired entries")
	}
}

func TestDenylistRevokeEvictsExpired(t *testing.T) {
	// Do not use NewJTIDenyList(): timing_test.go sets denyListPurgeInterval to
	// 5ms so the purge goroutine fires between setup and Revoke under -race,
	// deleting the expired entries before the eviction branch can be reached.
	dl := &JTIDenyList{entries: make(map[string]time.Time)}
	past := time.Now().Add(-time.Hour)
	future := time.Now().Add(time.Hour)
	for i := 0; i < maxDenyListEntries; i++ {
		dl.entries[fmt.Sprintf("old-%d", i)] = past
	}
	dl.Revoke("new-jti", future)
	if !dl.IsRevoked("new-jti") {
		t.Fatal("expected room after evicting expired entry")
	}
}

func TestBruteForceCleanupRemovesStaleAttempts(t *testing.T) {
	guard := NewBruteForceGuard(5, 1)
	defer guard.Stop()
	guard.mu.Lock()
	guard.attempts["9.9.9.9"] = &attemptInfo{count: 1, lastTry: time.Now().Add(-31 * time.Minute)}
	guard.mu.Unlock()
	time.Sleep(20 * time.Millisecond)
	guard.mu.Lock()
	_, exists := guard.attempts["9.9.9.9"]
	guard.mu.Unlock()
	if exists {
		t.Fatal("expected stale attempt to be cleaned up")
	}
}

func TestPurgeLoopDeletesExpiredEntry(t *testing.T) {
	dl := NewJTIDenyList()
	// Add an already-expired entry; the 5ms purge goroutine (set by timing_test.go) will delete it.
	dl.Revoke("old-jti", time.Now().Add(-time.Hour))
	time.Sleep(50 * time.Millisecond)
	dl.mu.Lock()
	_, still := dl.entries["old-jti"]
	dl.mu.Unlock()
	if still {
		t.Fatal("expected purge loop to remove expired entry")
	}
}

func TestNodeAgentAuth_InvalidFormat(t *testing.T) {
	db := setupAgentDB(t)
	req := httptest.NewRequest(http.MethodGet, "/agent/v1/health", nil)
	req.Header.Set("Authorization", "Token not-bearer")
	c, w := newTestContext(req)
	NodeAgentAuth(db, "")(c)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d", w.Code)
	}
}
