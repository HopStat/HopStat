package quickqueries

import (
	"context"
	"database/sql"
	"sync"

	"github.com/HopStat/HopStat/internal/domain"
	"github.com/HopStat/HopStat/internal/store/queries"
)

var (
	mu     sync.RWMutex
	cached []domain.QuickQuery
)

func mapQuery(item *queries.QuickQuery) domain.QuickQuery {
	if item == nil {
		return domain.QuickQuery{}
	}
	return domain.QuickQuery{
		ID:        item.ID,
		Command:   item.Command,
		Name:      item.Name,
		Target:    item.Target,
		NodeID:    mapQueryNodeID(item),
		SortOrder: item.SortOrder,
		Active:    item.Active == 1,
	}
}

func mapQueryNodeID(item *queries.QuickQuery) *int64 {
	if item == nil || !item.NodeID.Valid || item.NodeID.Int64 <= 0 {
		return nil
	}
	nodeID := item.NodeID.Int64
	return &nodeID
}

func Load(db *sql.DB) error {
	q := queries.New(db)
	items, err := q.GetAllQuickQueries(context.Background())
	if err != nil {
		return err
	}
	out := make([]domain.QuickQuery, 0, len(items))
	for i := range items {
		out = append(out, mapQuery(&items[i]))
	}
	mu.Lock()
	cached = out
	mu.Unlock()
	return nil
}

func Refresh(db *sql.DB) error {
	return Load(db)
}

func Active() []domain.QuickQuery {
	mu.RLock()
	defer mu.RUnlock()
	out := make([]domain.QuickQuery, 0, len(cached))
	for _, item := range cached {
		if item.Active {
			out = append(out, item)
		}
	}
	return out
}

func All() []domain.QuickQuery {
	mu.RLock()
	defer mu.RUnlock()
	if len(cached) == 0 {
		return []domain.QuickQuery{}
	}
	out := make([]domain.QuickQuery, len(cached))
	copy(out, cached)
	return out
}
