package quickqueries_test

import (
	"context"
	"database/sql"
	"testing"

	"github.com/HopStat/HopStat/internal/quickqueries"
	"github.com/HopStat/HopStat/internal/store"
	"github.com/HopStat/HopStat/internal/store/queries"
)

func TestQuickQueriesCache(t *testing.T) {
	db, err := store.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := store.Migrate(db); err != nil {
		t.Fatal(err)
	}
	if err := quickqueries.Load(db); err != nil {
		t.Fatal(err)
	}

	active := quickqueries.Active()
	if len(active) == 0 {
		t.Fatal("expected seeded quick queries")
	}
	all := quickqueries.All()
	if len(all) < len(active) {
		t.Fatalf("All()=%d Active()=%d", len(all), len(active))
	}
	for _, item := range active {
		if !item.Active {
			t.Fatalf("active list contains inactive item %d", item.ID)
		}
	}
}

func TestQuickQueriesRefresh(t *testing.T) {
	db, err := store.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := store.Migrate(db); err != nil {
		t.Fatal(err)
	}
	if err := quickqueries.Refresh(db); err != nil {
		t.Fatal(err)
	}
	if quickqueries.All() == nil {
		t.Fatal("expected non-nil slice from All()")
	}
}

func TestQuickQueriesWithNodeID(t *testing.T) {
	db, err := store.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := store.Migrate(db); err != nil {
		t.Fatal(err)
	}

	q := queries.New(db)
	nodeID := sql.NullInt64{Int64: 7, Valid: true}
	created, err := q.CreateQuickQuery(context.Background(), &queries.QuickQuery{
		Command:   "ping",
		Name:      "node ping",
		Target:    "8.8.8.8",
		NodeID:    nodeID,
		SortOrder: 99,
		Active:    1,
	})
	if err != nil {
		t.Fatal(err)
	}

	if err := quickqueries.Load(db); err != nil {
		t.Fatal(err)
	}

	var found bool
	for _, item := range quickqueries.All() {
		if item.ID == created.ID {
			found = true
			if item.NodeID == nil || *item.NodeID != 7 {
				t.Fatalf("node ID = %v, want 7", item.NodeID)
			}
		}
	}
	if !found {
		t.Fatal("created quick query not found in cache")
	}
}
