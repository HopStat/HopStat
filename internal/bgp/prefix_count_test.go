package bgp

import (
	"context"
	"testing"
	"time"

	"github.com/HopStat/HopStat/internal/config"
	"github.com/HopStat/HopStat/internal/domain"
)

func TestPrefixCountCacheHitAndExpiry(t *testing.T) {
	mgr := NewSessionManager(config.BGPConfig{})
	mgr.storePrefixCount(7, 100_000)

	if total, ok := mgr.cachedPrefixCount(7); !ok || total != 100_000 {
		t.Fatalf("cache hit = (%d, %v), want (100000, true)", total, ok)
	}

	mgr.prefixCountMu.Lock()
	mgr.prefixCounts[7] = prefixCountEntry{total: 100_000, updatedAt: time.Now().Add(-prefixCountCacheTTL - time.Second)}
	mgr.prefixCountMu.Unlock()

	if _, ok := mgr.cachedPrefixCount(7); ok {
		t.Fatal("expected expired cache entry to miss")
	}
}

func TestInvalidatePrefixCount(t *testing.T) {
	mgr := NewSessionManager(config.BGPConfig{})
	mgr.storePrefixCount(3, 42)
	mgr.invalidatePrefixCount(3)
	if _, ok := mgr.cachedPrefixCount(3); ok {
		t.Fatal("expected cache entry to be removed")
	}
}

func TestGetPrefixesReceivedUsesCache(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	mgr := NewSessionManager(config.BGPConfig{LocalAS: 65001, RouterID: "127.0.0.1"})
	if err := mgr.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer mgr.Stop()

	mgr.mu.Lock()
	mgr.neighbors[1] = &neighborEntry{neighbor: &domain.BGPNeighbor{ID: 1}, neighborIP: "10.0.0.2"}
	mgr.states[1] = domain.BGPSessionEstablished
	mgr.mu.Unlock()
	mgr.storePrefixCount(1, 123_456)

	total := mgr.GetPrefixesReceived(ctx, 1)
	if total != 123_456 {
		t.Fatalf("GetPrefixesReceived = %d, want cached 123456", total)
	}
}

func TestInvalidatePrefixCountNilReceiver(t *testing.T) {
	var mgr *SessionManager
	mgr.invalidatePrefixCount(1)
}

func TestStorePrefixCountNilMap(t *testing.T) {
	mgr := &SessionManager{}
	mgr.storePrefixCount(9, 77)
	if total, ok := mgr.cachedPrefixCount(9); !ok || total != 77 {
		t.Fatalf("stored count = (%d, %v)", total, ok)
	}
}
