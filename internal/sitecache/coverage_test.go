package sitecache_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/HopStat/HopStat/internal/domain"
	"github.com/HopStat/HopStat/internal/sitecache"
	"github.com/HopStat/HopStat/internal/store"
	"github.com/HopStat/HopStat/internal/store/repo"
)

func TestCachedNodeRepo_AllMethods(t *testing.T) {
	db, err := store.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := store.Migrate(db); err != nil {
		t.Fatal(err)
	}

	cached := sitecache.NewCachedNodeRepo(db, "")
	ctx := context.Background()

	created, err := cached.Create(ctx, &domain.Node{
		Name: "n1", Type: domain.NodeTypeStandalone, Active: true,
		EnabledCmds: domain.DefaultEnabledCmds(),
	})
	if err != nil {
		t.Fatal(err)
	}
	all, err := cached.GetAll(ctx)
	if err != nil || len(all) != 1 {
		t.Fatalf("GetAll = %d err %v", len(all), err)
	}
	active, err := cached.GetActive(ctx)
	if err != nil || len(active) != 1 {
		t.Fatalf("GetActive = %d err %v", len(active), err)
	}
	created.Name = "renamed"
	if _, err := cached.Update(ctx, created); err != nil {
		t.Fatal(err)
	}
	if err := cached.SetDefault(ctx, created.ID); err != nil {
		t.Fatal(err)
	}
	if err := cached.UpdateEnabledCmds(ctx, created.ID, []domain.CommandType{domain.CmdPing}); err != nil {
		t.Fatal(err)
	}
	if err := cached.Delete(ctx, created.ID); err != nil {
		t.Fatal(err)
	}
}

func TestCachedCommunityRuleRepo_AllMethods(t *testing.T) {
	db, err := store.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := store.Migrate(db); err != nil {
		t.Fatal(err)
	}

	cached := sitecache.NewCachedCommunityRuleRepo(db)
	ctx := context.Background()

	created, err := cached.Create(ctx, &domain.CommunityRule{
		Community: "65001:1", Severity: domain.SeverityInfo,
		MessageI18n: "{}", Scope: "global", Active: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	all, err := cached.GetAll(ctx)
	if err != nil || len(all) != 1 {
		t.Fatalf("GetAll = %d err %v", len(all), err)
	}
	active, err := cached.GetActive(ctx)
	if err != nil || len(active) != 1 {
		t.Fatalf("GetActive = %d err %v", len(active), err)
	}
	created.Community = "65001:2"
	if _, err := cached.Update(ctx, created); err != nil {
		t.Fatal(err)
	}
	if err := cached.Toggle(ctx, created.ID); err != nil {
		t.Fatal(err)
	}
	if err := cached.Delete(ctx, created.ID); err != nil {
		t.Fatal(err)
	}
}

func TestSettingsAndCommunitiesCache(t *testing.T) {
	db, err := store.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := store.Migrate(db); err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	sitecache.SetLogoUploadsDir(dir)
	logoPath := filepath.Join(dir, "logo.png")
	if err := os.WriteFile(logoPath, []byte("png"), 0o644); err != nil {
		t.Fatal(err)
	}

	q := repo.NewCommunityRuleRepo(db)
	node, err := repo.NewNodeRepo(db, "").Create(context.Background(), &domain.Node{
		Name: "n", Type: domain.NodeTypeStandalone, Active: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	nodeID := node.ID
	_, err = q.Create(context.Background(), &domain.CommunityRule{
		Community: "65000:1", Severity: domain.SeverityInfo,
		MessageI18n: "{}", Scope: "global", Active: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = q.Create(context.Background(), &domain.CommunityRule{
		Community: "65000:2", Severity: domain.SeverityWarning,
		MessageI18n: "{}", Scope: "node", NodeID: &nodeID, Active: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := sitecache.RefreshCommunities(db); err != nil {
		t.Fatal(err)
	}

	communities := sitecache.ActiveCommunities()
	if len(communities) != 2 {
		t.Fatalf("communities = %d", len(communities))
	}
	rules, ok := sitecache.ActiveCommunityRulesForNode(nodeID)
	if !ok || len(rules) != 2 {
		t.Fatalf("rules for node = %d ok=%v", len(rules), ok)
	}
	if _, ok := sitecache.ActiveCommunityRulesForNode(999); !ok {
		t.Fatal("expected cache loaded")
	}

	if err := sitecache.RefreshSettings(db, 65001); err != nil {
		t.Fatal(err)
	}
	all := sitecache.AllSettings()
	if all["site_name"] == "" {
		t.Fatalf("AllSettings = %#v", all)
	}
	if err := sitecache.RefreshSettings(db, 0); err != nil {
		t.Fatal(err)
	}
}

func TestLoad_ErrorPaths(t *testing.T) {
	db, err := store.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := sitecache.Load(db, "", 0); err == nil {
		t.Fatal("expected Load to fail without migrated schema")
	}
}
