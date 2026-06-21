package quickqueries

import (
	"database/sql"
	"testing"

	"github.com/HopStat/HopStat/internal/store/queries"
	_ "modernc.org/sqlite"
)

func TestMapQueryNil(t *testing.T) {
	got := mapQuery(nil)
	if got.ID != 0 || got.Command != "" {
		t.Fatalf("mapQuery(nil) = %+v, want zero value", got)
	}
}

func TestMapQueryNodeID(t *testing.T) {
	if mapQueryNodeID(nil) != nil {
		t.Fatal("nil item should return nil node ID")
	}
	if mapQueryNodeID(&queries.QuickQuery{}) != nil {
		t.Fatal("invalid node ID should return nil")
	}
	if mapQueryNodeID(&queries.QuickQuery{NodeID: sql.NullInt64{Int64: 0, Valid: true}}) != nil {
		t.Fatal("zero node ID should return nil")
	}
	neg := int64(-1)
	if mapQueryNodeID(&queries.QuickQuery{NodeID: sql.NullInt64{Int64: neg, Valid: true}}) != nil {
		t.Fatal("negative node ID should return nil")
	}
	nodeID := int64(42)
	got := mapQueryNodeID(&queries.QuickQuery{NodeID: sql.NullInt64{Int64: nodeID, Valid: true}})
	if got == nil || *got != nodeID {
		t.Fatalf("valid node ID = %v, want %d", got, nodeID)
	}
}

func TestMapQueryActiveFlag(t *testing.T) {
	active := mapQuery(&queries.QuickQuery{ID: 1, Command: "ping", Active: 1})
	if !active.Active {
		t.Fatal("active=1 should map to true")
	}
	inactive := mapQuery(&queries.QuickQuery{ID: 2, Command: "ping", Active: 0})
	if inactive.Active {
		t.Fatal("active=0 should map to false")
	}
}

func TestAllEmptyCache(t *testing.T) {
	mu.Lock()
	prev := cached
	cached = nil
	mu.Unlock()
	t.Cleanup(func() {
		mu.Lock()
		cached = prev
		mu.Unlock()
	})

	got := All()
	if got == nil || len(got) != 0 {
		t.Fatalf("All() with empty cache = %v, want empty slice", got)
	}
}

func TestLoadClosedDB(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	db.Close()
	if err := Load(db); err == nil {
		t.Fatal("expected error loading from closed db")
	}
}
