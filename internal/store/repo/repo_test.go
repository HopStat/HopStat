package repo

import (
	"context"
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/HopStat/HopStat/internal/domain"
	"github.com/HopStat/HopStat/internal/store"
)

func setupRepoDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := store.Migrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

const testCredKey = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func TestNodeRepo_CRUD(t *testing.T) {
	db := setupRepoDB(t)
	repo := NewNodeRepo(db, testCredKey)
	ctx := context.Background()

	created, err := repo.Create(ctx, &domain.Node{
		Name:        "test-node",
		Type:        domain.NodeTypeStandalone,
		Active:      true,
		IsDefault:   true,
		EnabledCmds: []domain.CommandType{domain.CmdPing, domain.CmdTraceroute},
		AgentToken:  "secret-token",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.ID == 0 {
		t.Fatal("expected non-zero id")
	}
	if created.AgentToken != "secret-token" {
		t.Errorf("AgentToken = %q, want secret-token", created.AgentToken)
	}

	got, err := repo.GetByID(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.Name != "test-node" {
		t.Errorf("Name = %q", got.Name)
	}

	got.Name = "renamed"
	updated, err := repo.Update(ctx, got)
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated.Name != "renamed" {
		t.Errorf("Name = %q after update", updated.Name)
	}

	active, err := repo.GetActive(ctx)
	if err != nil {
		t.Fatalf("GetActive: %v", err)
	}
	if len(active) != 1 {
		t.Fatalf("GetActive len = %d, want 1", len(active))
	}

	all, err := repo.GetAll(ctx)
	if err != nil {
		t.Fatalf("GetAll: %v", err)
	}
	if len(all) != 1 {
		t.Fatalf("GetAll len = %d, want 1", len(all))
	}

	if err := repo.UpdateEnabledCmds(ctx, created.ID, []domain.CommandType{domain.CmdPing}); err != nil {
		t.Fatalf("UpdateEnabledCmds: %v", err)
	}
	refreshed, _ := repo.GetByID(ctx, created.ID)
	if len(refreshed.EnabledCmds) != 1 || refreshed.EnabledCmds[0] != domain.CmdPing {
		t.Errorf("EnabledCmds = %v", refreshed.EnabledCmds)
	}

	if err := repo.Delete(ctx, created.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	_, err = repo.GetByID(ctx, created.ID)
	if err != domain.ErrNodeNotFound {
		t.Errorf("GetByID after delete = %v, want ErrNodeNotFound", err)
	}
}

func TestNodeRepo_EnsureDefault(t *testing.T) {
	db := setupRepoDB(t)
	repo := NewNodeRepo(db, "")
	ctx := context.Background()

	n1, _ := repo.Create(ctx, &domain.Node{Name: "a", Type: domain.NodeTypeStandalone, Active: true})
	n2, _ := repo.Create(ctx, &domain.Node{Name: "b", Type: domain.NodeTypeStandalone, Active: true})

	if err := repo.SetDefault(ctx, n2.ID); err != nil {
		t.Fatalf("SetDefault: %v", err)
	}
	got, _ := repo.GetByID(ctx, n2.ID)
	if !got.IsDefault {
		t.Error("n2 should be default")
	}
	got1, _ := repo.GetByID(ctx, n1.ID)
	if got1.IsDefault {
		t.Error("n1 should not be default")
	}
}

func TestUserRepo_GetByEmail(t *testing.T) {
	db := setupRepoDB(t)
	if err := store.SetAdminPassword(db, "password123"); err != nil {
		t.Fatalf("SetAdminPassword: %v", err)
	}
	repo := NewUserRepo(db)
	ctx := context.Background()

	user, err := repo.GetByEmail(ctx, store.DefaultAdminEmail)
	if err != nil {
		t.Fatalf("GetByEmail: %v", err)
	}
	if user == nil {
		t.Fatal("expected admin user")
	}
	if user.Email != store.DefaultAdminEmail {
		t.Errorf("email = %q", user.Email)
	}

	missing, err := repo.GetByEmail(ctx, "missing@example.com")
	if err != nil {
		t.Fatalf("GetByEmail missing: %v", err)
	}
	if missing != nil {
		t.Error("expected nil for missing user")
	}
}

func TestAuditRepo_LogAndList(t *testing.T) {
	db := setupRepoDB(t)
	repo := NewAuditRepo(db)
	ctx := context.Background()
	uid := int64(1)

	if err := repo.Log(ctx, &domain.AuditEntry{
		UserID:  &uid,
		Command: "login",
		Params:  "ok",
		Success: true,
	}); err != nil {
		t.Fatalf("Log: %v", err)
	}

	entries, total, err := repo.List(ctx, domain.AuditFilter{Page: 0, Limit: 10})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if total != 1 || len(entries) != 1 {
		t.Fatalf("total=%d len=%d, want 1/1", total, len(entries))
	}
	if entries[0].Command != "login" {
		t.Errorf("command = %q", entries[0].Command)
	}
}

func TestCommunityRuleRepo_ActiveAndToggle(t *testing.T) {
	db := setupRepoDB(t)
	repo := NewCommunityRuleRepo(db)
	ctx := context.Background()

	created, err := repo.Create(ctx, &domain.CommunityRule{
		Community: "65001:99", Severity: domain.SeverityWarning,
		MessageI18n: "warn", Scope: "global", Active: true,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	active, err := repo.GetActive(ctx)
	if err != nil {
		t.Fatalf("GetActive: %v", err)
	}
	if len(active) != 1 {
		t.Fatalf("active len = %d", len(active))
	}

	if err := repo.Toggle(ctx, created.ID); err != nil {
		t.Fatalf("Toggle: %v", err)
	}
	active, _ = repo.GetActive(ctx)
	if len(active) != 0 {
		t.Fatalf("expected 0 active after toggle, got %d", len(active))
	}

	nodeID := int64(1)
	_, err = repo.Create(ctx, &domain.CommunityRule{
		Community: "65001:100", Severity: domain.SeverityInfo,
		MessageI18n: "node", Scope: "node", NodeID: &nodeID, Active: true,
	})
	if err != nil {
		t.Fatalf("Create node rule: %v", err)
	}
	forNode, err := repo.GetActiveRulesForNode(ctx, nodeID)
	if err != nil {
		t.Fatalf("GetActiveRulesForNode: %v", err)
	}
	if len(forNode) == 0 {
		t.Fatal("expected node-scoped rule")
	}
}
