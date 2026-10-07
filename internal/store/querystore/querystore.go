package querystore

import (
	"sync"
	"time"

	"github.com/HopStat/HopStat/internal/domain"
)

type entry struct {
	result         *domain.QueryResult
	lines          []string
	outputComplete bool
	linesMu        sync.Mutex
	notify         chan struct{}
	createdAt      time.Time
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
	// Interval read here rather than on the background goroutine: it is a package-level
	// test seam, and reading it there races with any test that swaps it.
	go s.cleanup(cleanupTickInterval)
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
		notify:    make(chan struct{}, 1),
		createdAt: time.Now(),
	}
}

func (s *Store) NotifyCh(id string) <-chan struct{} {
	s.mu.RLock()
	e, ok := s.results[id]
	s.mu.RUnlock()
	if !ok || e.notify == nil {
		ch := make(chan struct{})
		close(ch)
		return ch
	}
	return e.notify
}

func (s *Store) signal(id string) {
	s.mu.RLock()
	e, ok := s.results[id]
	s.mu.RUnlock()
	if !ok || e.notify == nil {
		return
	}
	select {
	case e.notify <- struct{}{}:
	default:
	}
}

func (s *Store) AppendLine(id, line string) {
	s.mu.RLock()
	e, ok := s.results[id]
	if ok {
		e.linesMu.Lock()
		e.lines = append(e.lines, line)
		e.linesMu.Unlock()
	}
	s.mu.RUnlock()
	if ok {
		s.signal(id)
	}
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

func (s *Store) MergePartial(id string, partial *domain.QueryResult) {
	if partial == nil {
		return
	}
	s.mu.Lock()
	e, ok := s.results[id]
	if !ok || e.result == nil {
		s.mu.Unlock()
		return
	}

	mergeASPathPartialFields(e.result, partial)

	if e.result.Status != domain.StatusRunning {
		s.mu.Unlock()
		return
	}
	if partial.Parsed != nil {
		e.result.Parsed = partial.Parsed
	}
	if partial.Raw != "" {
		e.result.Raw = partial.Raw
	}
	if len(partial.MatchedRules) > 0 {
		e.result.MatchedRules = partial.MatchedRules
	}
	s.mu.Unlock()
	s.signal(id)
}

func mergeASPathPartialFields(dest, partial *domain.QueryResult) {
	if len(partial.ASPath) > 0 {
		dest.ASPath = partial.ASPath
	}
	if partial.ASPathPrefix != "" {
		dest.ASPathPrefix = partial.ASPathPrefix
	}
	if len(partial.ASPathEnriched) > 0 {
		dest.ASPathEnriched = partial.ASPathEnriched
	}
	if len(partial.ASPathNodes) > 0 {
		dest.ASPathNodes = partial.ASPathNodes
	}
}

func (s *Store) MarkOutputComplete(id string) {
	s.mu.Lock()
	if e, ok := s.results[id]; ok {
		e.outputComplete = true
	}
	s.mu.Unlock()
	s.signal(id)
}

func (s *Store) IsOutputComplete(id string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if e, ok := s.results[id]; ok {
		return e.outputComplete
	}
	return false
}

func (s *Store) Set(id string, result *domain.QueryResult) {
	// Copy on the way in, for the same reason Get copies on the way out. MergePartial
	// mutates e.result in place through mergeASPathPartialFields, and that runs before
	// the status guard, so storing the caller's pointer would let those writes land on
	// memory the caller still owns and still reads without holding a lock — the same
	// unsynchronised read/write pair, entered from the other direction. Observed under
	// -race at querystore.go:137 against a caller reading the result it had just handed
	// over, as SubmitQuery does when it builds the audit entry.
	var stored *domain.QueryResult
	if result != nil {
		cp := *result
		stored = &cp
	}
	s.mu.Lock()
	if e, ok := s.results[id]; ok {
		e.result = stored
	} else {
		s.results[id] = &entry{result: stored, notify: make(chan struct{}, 1), createdAt: time.Now()}
	}
	s.mu.Unlock()
	s.signal(id)
}

// Get returns a copy of the stored result.
//
// It must not hand back the live pointer. MergePartial writes fields into
// e.result in place while holding the store lock, but every caller of Get reads
// the result after this function has already released the lock — GetResult
// serialises it straight into the response. Returning e.result therefore races
// the query goroutine's OnPartial writes: concurrent read and write of the same
// memory, observed under -race at querystore.go:126 against a Get caller.
func (s *Store) Get(id string) (*domain.QueryResult, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	e, ok := s.results[id]
	if !ok {
		return nil, false
	}
	if e.result == nil {
		return nil, true
	}
	cp := *e.result
	return &cp, true
}

func (s *Store) Delete(id string) {
	s.mu.Lock()
	delete(s.results, id)
	s.mu.Unlock()
}

func (s *Store) Stop() {
	close(s.stopCh)
}

var cleanupTickInterval = time.Minute

func (s *Store) cleanup(interval time.Duration) {
	ticker := time.NewTicker(interval)
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
