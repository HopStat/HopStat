package sitecache_test

import (
	"context"
	"testing"

	"github.com/HopStat/HopStat/internal/domain"
	"github.com/HopStat/HopStat/internal/sitecache"
	"github.com/HopStat/HopStat/internal/store"
	"github.com/HopStat/HopStat/internal/store/repo"
)

func TestNodeByIDFromCache(t *testing.T) {
	db, err := store.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := store.Migrate(db); err != nil {
		t.Fatal(err)
	}

	nodeRepo := repo.NewNodeRepo(db, "")
	created, err := nodeRepo.Create(context.Background(), &domain.Node{
		Name: "edge", Type: domain.NodeTypeStandalone, Active: true,
		AgentToken:  "secret-token",
		EnabledCmds: domain.DefaultEnabledCmds(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := sitecache.RefreshNodes(db, ""); err != nil {
		t.Fatal(err)
	}

	node, ok := sitecache.NodeByID(created.ID)
	if !ok {
		t.Fatal("expected cached node")
	}
	if node.AgentToken != "secret-token" {
		t.Fatal("engine cache should retain agent token")
	}

	public := sitecache.ActiveNodes()
	if len(public) != 1 || public[0].AgentToken != "" {
		t.Fatal("public node list must strip agent token")
	}
}

func TestActiveCommunityRulesForNode(t *testing.T) {
	db, err := store.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := store.Migrate(db); err != nil {
		t.Fatal(err)
	}
	if err := sitecache.Load(db, "", 0); err != nil {
		t.Fatal(err)
	}

	rules, ok := sitecache.ActiveCommunityRulesForNode(1)
	if !ok {
		t.Fatal("cache should be loaded")
	}
	if rules != nil {
		t.Fatalf("expected nil rules slice, got %d", len(rules))
	}
}
