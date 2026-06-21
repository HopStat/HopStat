package bgp

import (
	"sync"
	"time"
)

const maxLogEntriesPerNeighbor = 400

type LogEntry struct {
	Timestamp time.Time `json:"timestamp"`
	Level     string    `json:"level"`
	Message   string    `json:"message"`
	Address   string    `json:"address,omitempty"`
}

type eventLog struct {
	mu      sync.RWMutex
	entries map[int64][]LogEntry
}

func newEventLog() *eventLog {
	return &eventLog{entries: make(map[int64][]LogEntry)}
}

func (l *eventLog) add(neighborID int64, level, message, address string) {
	l.mu.Lock()
	defer l.mu.Unlock()

	list := l.entries[neighborID]
	entry := LogEntry{
		Timestamp: time.Now().UTC(),
		Level:     level,
		Message:   message,
		Address:   address,
	}
	list = append(list, entry)
	if len(list) > maxLogEntriesPerNeighbor {
		list = list[len(list)-maxLogEntriesPerNeighbor:]
	}
	l.entries[neighborID] = list
}

func (l *eventLog) list(neighborID int64, limit int) []LogEntry {
	l.mu.RLock()
	defer l.mu.RUnlock()

	list := l.entries[neighborID]
	if len(list) == 0 {
		return []LogEntry{}
	}
	if limit <= 0 || limit > len(list) {
		limit = len(list)
	}
	start := len(list) - limit
	out := make([]LogEntry, limit)
	copy(out, list[start:])
	return out
}

func (l *eventLog) remove(neighborID int64) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.entries, neighborID)
}
