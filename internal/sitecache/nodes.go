package sitecache

import (
	"context"
	"database/sql"
	"sync"

	"github.com/HopStat/HopStat/internal/domain"
	"github.com/HopStat/HopStat/internal/store/repo"
)

var (
	nodesMu         sync.RWMutex
	cachedNodes     []*domain.Node
	cachedNodesByID map[int64]*domain.Node
	loadActiveNodes = func(db *sql.DB, credKey string) ([]*domain.Node, error) {
		r := repo.NewNodeRepo(db, credKey)
		return r.GetActive(context.Background())
	}
)

func RefreshNodes(db *sql.DB, credKey string) error {
	nodes, err := loadActiveNodes(db, credKey)
	if err != nil {
		return err
	}
	out := make([]*domain.Node, 0, len(nodes))
	byID := make(map[int64]*domain.Node, len(nodes))
	for _, n := range nodes {
		if n == nil {
			continue
		}
		copyNode := *n
		byID[copyNode.ID] = &copyNode
		publicNode := copyNode
		publicNode.AgentToken = ""
		out = append(out, &publicNode)
	}
	nodesMu.Lock()
	cachedNodes = out
	cachedNodesByID = byID
	nodesMu.Unlock()
	return nil
}

// NodeByID returns a cached active node for query execution (includes agent token).
func NodeByID(id int64) (*domain.Node, bool) {
	nodesMu.RLock()
	defer nodesMu.RUnlock()
	n, ok := cachedNodesByID[id]
	if !ok || n == nil {
		return nil, false
	}
	copyNode := *n
	return &copyNode, true
}

// ActiveNodes returns cached active public nodes (without agent tokens).
func ActiveNodes() []*domain.Node {
	nodesMu.RLock()
	defer nodesMu.RUnlock()
	if len(cachedNodes) == 0 {
		return nil
	}
	out := make([]*domain.Node, len(cachedNodes))
	for i, n := range cachedNodes {
		if n == nil {
			continue
		}
		copyNode := *n
		copyNode.AgentToken = ""
		out[i] = &copyNode
	}
	return out
}
