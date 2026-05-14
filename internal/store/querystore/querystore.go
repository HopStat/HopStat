package querystore

import (
	"sync"
	"time"

	"github.com/HopStat/HopStat/internal/domain"
)

type entry struct {
	result    *domain.QueryResult
	lines     []string
	linesMu   sync.Mutex
	createdAt time.Time
}

type Store struct {
	mu      sync.RWMutex
	results map[string]*entry
	expiry  time.Duration
	stopCh  chan struct{}
}

func New() *Store {
	s := &Store{
		results: make(map[string]*entry),
		expiry:  5 * time.Minute,
		stopCh:  make(chan struct{}),
	}
	go s.cleanup()
	return s
}

func (s *Store) SetRunning(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.results[id] = &entry{
		result: &domain.QueryResult{
			ID:     id,
			Status: domain.StatusRunning,
		},
		lines:     []string{},
		createdAt: time.Now(),
	}
}

func (s *Store) AppendLine(id, line string) {
	s.mu.RLock()
	e, ok := s.results[id]
	s.mu.RUnlock()
	if !ok {
		return
	}
	e.linesMu.Lock()
	e.lines = append(e.lines, line)
	e.linesMu.Unlock()
}

func (s *Store) GetLines(id string) ([]string, bool) {
	s.mu.RLock()
	e, ok := s.results[id]
	s.mu.RUnlock()
	if !ok {
		return nil, false
	}
	e.linesMu.Lock()
	defer e.linesMu.Unlock()
	cp := make([]string, len(e.lines))
	copy(cp, e.lines)
	return cp, true
}

func (s *Store) Set(id string, result *domain.QueryResult) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if e, ok := s.results[id]; ok {
		e.result = result
	} else {
		s.results[id] = &entry{result: result, createdAt: time.Now()}
	}
}

func (s *Store) Get(id string) (*domain.QueryResult, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	e, ok := s.results[id]
	if !ok {
		return nil, false
	}
	return e.result, true
}

func (s *Store) Delete(id string) {
	s.mu.Lock()
	delete(s.results, id)
	s.mu.Unlock()
}

func (s *Store) Stop() {
	close(s.stopCh)
}

func (s *Store) cleanup() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-s.stopCh:
			return
		case <-ticker.C:
			s.mu.Lock()
			now := time.Now()
			for id, e := range s.results {
				if now.Sub(e.createdAt) > s.expiry {
					delete(s.results, id)
				}
			}
			s.mu.Unlock()
		}
	}
}
