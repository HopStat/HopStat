package sitecache

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/HopStat/HopStat/internal/domain"
	"github.com/HopStat/HopStat/internal/store"
	"github.com/HopStat/HopStat/internal/store/repo"
)

func TestLoadSuccess(t *testing.T) {
	db, err := store.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := store.Migrate(db); err != nil {
		t.Fatal(err)
	}
	if err := Load(db, "", 65001); err != nil {
		t.Fatal(err)
	}
}

func TestLoadSettingsError(t *testing.T) {
	db, err := store.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := store.Migrate(db); err != nil {
		t.Fatal(err)
	}
	db.Close()
	if err := Load(db, "", 0); err == nil {
		t.Fatal("expected settings refresh error")
	}
}

func TestEmptySettingsAndPublicSettings(t *testing.T) {
	settingsMu.Lock()
	allSettings = nil
	publicSettings = nil
	settingsMu.Unlock()
	if got := AllSettings(); len(got) != 0 {
		t.Fatalf("AllSettings = %#v", got)
	}
	if got := PublicSettings(); len(got) != 0 {
		t.Fatalf("PublicSettings = %#v", got)
	}
}

func TestEnrichLogoPathBranches(t *testing.T) {
	enrichLogoPath(nil)
	m := map[string]string{"logo_path": "  "}
	enrichLogoPath(m)
	if m["logo_path"] != "  " {
		t.Fatal("expected unchanged empty logo")
	}
	m = map[string]string{"logo_path": "/other.png"}
	enrichLogoPath(m)
	if m["logo_path"] != "/other.png" {
		t.Fatal("expected non-logo path unchanged")
	}
}

func TestActiveCommunityRulesForNodeEmptyLoaded(t *testing.T) {
	communitiesMu.Lock()
	cachedCommunityRules = []*domain.CommunityRule{}
	communitiesMu.Unlock()
	rules, ok := ActiveCommunityRulesForNode(1)
	if !ok || rules != nil {
		t.Fatalf("rules=%v ok=%v", rules, ok)
	}
}

func TestCachedCommunityRuleRepoFallback(t *testing.T) {
	db, err := store.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := store.Migrate(db); err != nil {
		t.Fatal(err)
	}
	communitiesMu.Lock()
	cachedCommunityRules = nil
	communitiesMu.Unlock()

	inner := repo.NewCommunityRuleRepo(db)
	node, err := repo.NewNodeRepo(db, "").Create(context.Background(), &domain.Node{
		Name: "n", Type: domain.NodeTypeStandalone, Active: true,
		EnabledCmds: domain.DefaultEnabledCmds(),
	})
	if err != nil {
		t.Fatal(err)
	}
	nodeID := node.ID
	if _, err := inner.Create(context.Background(), &domain.CommunityRule{
		Community: "65000:1", Severity: domain.SeverityInfo,
		MessageI18n: "{}", Scope: "node", NodeID: &nodeID, Active: true,
	}); err != nil {
		t.Fatal(err)
	}
	cached := NewCachedCommunityRuleRepo(db)
	rules, err := cached.GetActiveRulesForNode(context.Background(), nodeID)
	if err != nil || len(rules) != 1 {
		t.Fatalf("rules=%d err=%v", len(rules), err)
	}
}

func TestRefreshNodesClosedDB(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	db.Close()
	if err := RefreshNodes(db, ""); err == nil {
		t.Fatal("expected refresh error")
	}
}

func TestRefreshCommunitiesClosedDB(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	db.Close()
	if err := RefreshCommunities(db); err == nil {
		t.Fatal("expected refresh error")
	}
}

func TestLoadStepErrors(t *testing.T) {
	db, err := store.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := store.Migrate(db); err != nil {
		t.Fatal(err)
	}

	t.Run("settings", func(t *testing.T) {
		if _, err := db.Exec(`DROP TABLE settings`); err != nil {
			t.Fatal(err)
		}
		if err := Load(db, "", 0); err == nil {
			t.Fatal("expected settings error")
		}
	})

	db2, err := store.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db2.Close()
	if err := store.Migrate(db2); err != nil {
		t.Fatal(err)
	}
	if _, err := db2.Exec(`DROP TABLE community_rules`); err != nil {
		t.Fatal(err)
	}
	if err := Load(db2, "", 0); err == nil {
		t.Fatal("expected communities error")
	}

	db3, err := store.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db3.Close()
	if err := store.Migrate(db3); err != nil {
		t.Fatal(err)
	}
	if _, err := db3.Exec(`DROP TABLE nodes`); err != nil {
		t.Fatal(err)
	}
	if err := Load(db3, "", 0); err == nil {
		t.Fatal("expected nodes error")
	}
}

func TestActiveCommunityRulesForNodeFilters(t *testing.T) {
	nodeID := int64(1)
	otherID := int64(2)
	inactive := false
	communitiesMu.Lock()
	cachedCommunityRules = []*domain.CommunityRule{
		nil,
		{ID: 1, Scope: "global", Active: true},
		{ID: 2, Scope: "node", NodeID: &nodeID, Active: true},
		{ID: 3, Scope: "node", NodeID: &otherID, Active: true},
		{ID: 4, Scope: "global", Active: false},
		{ID: 5, Scope: "node", NodeID: &nodeID, Active: inactive},
	}
	communitiesMu.Unlock()
	rules, ok := ActiveCommunityRulesForNode(nodeID)
	if !ok || len(rules) != 2 {
		t.Fatalf("rules=%d ok=%v", len(rules), ok)
	}
}

func TestActiveNodesSkipsNilEntries(t *testing.T) {
	nodesMu.Lock()
	cachedNodes = []*domain.Node{nil, {ID: 1, Name: "n", Active: true}}
	nodesMu.Unlock()
	nodes := ActiveNodes()
	if len(nodes) != 2 || nodes[0] != nil || nodes[1] == nil {
		t.Fatalf("nodes=%v", nodes)
	}
}

func TestEnrichLogoPathStatSuccess(t *testing.T) {
	dir := t.TempDir()
	SetLogoUploadsDir(dir)
	logoFile := filepath.Join(dir, "logo.png")
	if err := os.WriteFile(logoFile, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	m := map[string]string{"logo_path": "/logo.png"}
	enrichLogoPath(m)
	if !strings.Contains(m["logo_path"], "?v=") {
		t.Fatalf("logo_path=%q", m["logo_path"])
	}
	m = map[string]string{"logo_path": "/logo.png?v=1"}
	enrichLogoPath(m)
	if !strings.Contains(m["logo_path"], "?v=") {
		t.Fatalf("logo_path=%q", m["logo_path"])
	}
	missingDir := t.TempDir()
	SetLogoUploadsDir(missingDir)
	m = map[string]string{"logo_path": "/logo.png"}
	enrichLogoPath(m)
	if m["logo_path"] != "/logo.png" {
		t.Fatalf("logo_path=%q", m["logo_path"])
	}
}

func TestRefreshCommunitiesSkipsNilRule(t *testing.T) {
	old := loadActiveCommunityRules
	loadActiveCommunityRules = func(*sql.DB) ([]*domain.CommunityRule, error) {
		return []*domain.CommunityRule{
			nil,
			{ID: 1, Community: "65000:1", Severity: domain.SeverityInfo, MessageI18n: "{}", Scope: "global", Active: true},
		}, nil
	}
	t.Cleanup(func() { loadActiveCommunityRules = old })
	if err := RefreshCommunities(nil); err != nil {
		t.Fatal(err)
	}
	if len(ActiveCommunities()) != 1 {
		t.Fatalf("communities=%d", len(ActiveCommunities()))
	}
}

func TestRefreshNodesSkipsNilNode(t *testing.T) {
	old := loadActiveNodes
	loadActiveNodes = func(*sql.DB, string) ([]*domain.Node, error) {
		return []*domain.Node{
			nil,
			{ID: 1, Name: "n", Active: true, AgentToken: "tok"},
		}, nil
	}
	t.Cleanup(func() { loadActiveNodes = old })
	if err := RefreshNodes(nil, ""); err != nil {
		t.Fatal(err)
	}
	if len(ActiveNodes()) != 1 {
		t.Fatalf("nodes=%d", len(ActiveNodes()))
	}
}
