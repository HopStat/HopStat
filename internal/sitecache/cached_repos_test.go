package sitecache_test

import (
	"context"
	"testing"

	"github.com/HopStat/HopStat/internal/domain"
	"github.com/HopStat/HopStat/internal/sitecache"
	"github.com/HopStat/HopStat/internal/store"
	"github.com/HopStat/HopStat/internal/store/repo"
)

func TestCachedNodeRepo_GetByIDUsesSiteCache(t *testing.T) {
	db, err := store.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := store.Migrate(db); err != nil {
		t.Fatal(err)
	}

	inner := repo.NewNodeRepo(db, "")
	created, err := inner.Create(context.Background(), &domain.Node{
		Name: "cached", Type: domain.NodeTypeStandalone, Active: true,
		EnabledCmds: domain.DefaultEnabledCmds(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := sitecache.RefreshNodes(db, ""); err != nil {
		t.Fatal(err)
	}

	cached := sitecache.NewCachedNodeRepo(db, "")
	got, err := cached.GetByID(context.Background(), created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "cached" {
		t.Fatalf("name = %q", got.Name)
	}
}

func TestCachedCommunityRuleRepo_GetActiveRulesForNodeUsesCache(t *testing.T) {
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

	inner := repo.NewCommunityRuleRepo(db)
	createdNode, err := repo.NewNodeRepo(db, "").Create(context.Background(), &domain.Node{
		Name: "n", Type: domain.NodeTypeStandalone, Active: true,
		EnabledCmds: domain.DefaultEnabledCmds(),
	})
	if err != nil {
		t.Fatal(err)
	}
	nodeID := createdNode.ID
	_, err = inner.Create(context.Background(), &domain.CommunityRule{
		Community:   "65000:100",
		Severity:    domain.SeverityInfo,
		MessageI18n: `{"en":"test"}`,
		Scope:       "global",
		Active:      true,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = inner.Create(context.Background(), &domain.CommunityRule{
		Community:   "65000:200",
		Severity:    domain.SeverityWarning,
		MessageI18n: `{"en":"node"}`,
		Scope:       "node",
		NodeID:      &nodeID,
		Active:      true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := sitecache.RefreshCommunities(db); err != nil {
		t.Fatal(err)
	}

	cached := sitecache.NewCachedCommunityRuleRepo(db)
	rules, err := cached.GetActiveRulesForNode(context.Background(), nodeID)
	if err != nil {
		t.Fatal(err)
	}
	if len(rules) != 2 {
		t.Fatalf("expected 2 rules, got %d", len(rules))
	}
}
