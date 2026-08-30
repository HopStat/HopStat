package sitecache_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/HopStat/HopStat/internal/domain"
	"github.com/HopStat/HopStat/internal/sitecache"
	"github.com/HopStat/HopStat/internal/store"
	"github.com/HopStat/HopStat/internal/store/queries"
)

func TestCachedNodeRepo_GetByIDFallback(t *testing.T) {
	db, err := store.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := store.Migrate(db); err != nil {
		t.Fatal(err)
	}

	// This test needs a cache miss, but the node cache is process-wide and other tests
	// populate it. Refresh from this empty DB so the miss is deterministic.
	if err := sitecache.RefreshNodes(db, ""); err != nil {
		t.Fatal(err)
	}

	inner := sitecache.NewCachedNodeRepo(db, "")
	created, err := inner.Create(context.Background(), &domain.Node{
		Name: "fallback", Type: domain.NodeTypeStandalone, Active: true,
		EnabledCmds: domain.DefaultEnabledCmds(), AgentToken: "secret",
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := inner.GetByID(context.Background(), created.ID)
	if err != nil || got.Name != "fallback" {
		t.Fatalf("GetByID fallback = %+v err %v", got, err)
	}
}

func TestEnrichLogoPath(t *testing.T) {
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
	if err := os.WriteFile(filepath.Join(dir, "logo.png"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	q := queries.New(db)
	if err := q.SetSetting("logo_path", "/logo.png"); err != nil {
		t.Fatal(err)
	}
	if err := sitecache.RefreshSettings(db, 65001); err != nil {
		t.Fatal(err)
	}
	settings := sitecache.AllSettings()
	if settings["logo_path"] == "/logo.png" {
		t.Fatal("expected cache-busted logo path")
	}
}

func TestRefreshNodesAndActiveNodes(t *testing.T) {
	db, err := store.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := store.Migrate(db); err != nil {
		t.Fatal(err)
	}
	_, err = sitecache.NewCachedNodeRepo(db, "").Create(context.Background(), &domain.Node{
		Name: "active", Type: domain.NodeTypeStandalone, Active: true,
		EnabledCmds: domain.DefaultEnabledCmds(), AgentToken: "tok",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := sitecache.RefreshNodes(db, ""); err != nil {
		t.Fatal(err)
	}
	nodes := sitecache.ActiveNodes()
	if len(nodes) != 1 || nodes[0].AgentToken != "" {
		t.Fatalf("ActiveNodes = %+v", nodes)
	}
	if node, ok := sitecache.NodeByID(nodes[0].ID); !ok || node.AgentToken != "tok" {
		t.Fatalf("NodeByID = %+v ok=%v", node, ok)
	}
}
