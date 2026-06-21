package bgp

import "time"

const prefixCountCacheTTL = 30 * time.Second

type prefixCountEntry struct {
	total     int
	updatedAt time.Time
}

func (m *SessionManager) invalidatePrefixCount(neighborID int64) {
	if m == nil {
		return
	}
	m.prefixCountMu.Lock()
	delete(m.prefixCounts, neighborID)
	m.prefixCountMu.Unlock()
}

func (m *SessionManager) cachedPrefixCount(neighborID int64) (int, bool) {
	m.prefixCountMu.RLock()
	defer m.prefixCountMu.RUnlock()
	entry, ok := m.prefixCounts[neighborID]
	if !ok || time.Since(entry.updatedAt) >= prefixCountCacheTTL {
		return 0, false
	}
	return entry.total, true
}

func (m *SessionManager) storePrefixCount(neighborID int64, total int) {
	m.prefixCountMu.Lock()
	if m.prefixCounts == nil {
		m.prefixCounts = make(map[int64]prefixCountEntry)
	}
	m.prefixCounts[neighborID] = prefixCountEntry{
		total:     total,
		updatedAt: time.Now(),
	}
	m.prefixCountMu.Unlock()
}
