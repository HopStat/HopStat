package sitecache_test

import (
	"testing"

	"github.com/HopStat/HopStat/internal/quickqueries"
	"github.com/HopStat/HopStat/internal/sitecache"
	"github.com/HopStat/HopStat/internal/store"
)

func TestLoadServesPublicDataFromMemory(t *testing.T) {
	db, err := store.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := store.Migrate(db); err != nil {
		t.Fatal(err)
	}
	if err := sitecache.Load(db, "", 65000); err != nil {
		t.Fatal(err)
	}

	settings := sitecache.PublicSettings()
	if settings["site_name"] != "Looking Glass" {
		t.Fatalf("site_name = %q", settings["site_name"])
	}
	if settings["local_as"] != "65000" {
		t.Fatalf("local_as = %q", settings["local_as"])
	}

	active := quickqueries.Active()
	if len(active) == 0 {
		t.Fatal("expected seeded quick queries in cache")
	}
	if quickqueries.All()[0].ID != active[0].ID {
		t.Fatal("expected All() to include active quick query")
	}

	if nodes := sitecache.ActiveNodes(); len(nodes) != 0 {
		t.Fatalf("expected no active nodes, got %d", len(nodes))
	}
	if communities := sitecache.ActiveCommunities(); len(communities) != 0 {
		t.Fatalf("expected no communities, got %d", len(communities))
	}
}
