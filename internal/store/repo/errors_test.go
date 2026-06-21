package repo

import (
	"context"
	"database/sql"
	"testing"

	"github.com/HopStat/HopStat/internal/domain"
	_ "modernc.org/sqlite"
)

func TestRepo_ErrorPathsClosedDB(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	db.Close()
	ctx := context.Background()

	nodeRepo := NewNodeRepo(db, "")
	if _, err := nodeRepo.GetAll(ctx); err == nil {
		t.Fatal("expected error")
	}
	if _, err := nodeRepo.GetActive(ctx); err == nil {
		t.Fatal("expected error")
	}
	if _, err := nodeRepo.GetByID(ctx, 1); err == nil {
		t.Fatal("expected error")
	}
	if _, err := nodeRepo.Create(ctx, &domain.Node{Name: "n", Type: domain.NodeTypeStandalone}); err == nil {
		t.Fatal("expected error")
	}
	if _, err := nodeRepo.Update(ctx, &domain.Node{ID: 1, Name: "n", Type: domain.NodeTypeStandalone}); err == nil {
		t.Fatal("expected error")
	}
	if err := nodeRepo.SetDefault(ctx, 1); err == nil {
		t.Fatal("expected error")
	}
	if err := nodeRepo.Delete(ctx, 1); err == nil {
		t.Fatal("expected error")
	}
	if err := nodeRepo.UpdateEnabledCmds(ctx, 1, nil); err == nil {
		t.Fatal("expected error")
	}

	userRepo := NewUserRepo(db)
	if _, err := userRepo.GetByEmail(ctx, "a@b.c"); err == nil {
		t.Fatal("expected error")
	}
	if _, err := userRepo.GetByID(ctx, 1); err == nil {
		t.Fatal("expected error")
	}
	if _, err := userRepo.UpdateCredentials(ctx, 1, "a@b.c", "hash"); err == nil {
		t.Fatal("expected error")
	}
	if err := userRepo.UpdateLastLogin(ctx, 1); err == nil {
		t.Fatal("expected error")
	}

	auditRepo := NewAuditRepo(db)
	if err := auditRepo.Log(ctx, &domain.AuditEntry{Command: "ping"}); err == nil {
		t.Fatal("expected error")
	}
	if _, _, err := auditRepo.List(ctx, domain.AuditFilter{}); err == nil {
		t.Fatal("expected error")
	}
	if _, err := auditRepo.Cleanup(ctx, "2020-01-01"); err == nil {
		t.Fatal("expected error")
	}
	if _, err := auditRepo.CountToday(ctx); err == nil {
		t.Fatal("expected error")
	}

	bgpRepo := NewBGPNeighborRepo(db)
	if _, err := bgpRepo.GetAll(ctx); err == nil {
		t.Fatal("expected error")
	}
	if _, err := bgpRepo.GetByNodeID(ctx, 1); err == nil {
		t.Fatal("expected error")
	}
	if _, err := bgpRepo.GetByID(ctx, 1); err == nil {
		t.Fatal("expected error")
	}
	if _, err := bgpRepo.Create(ctx, &domain.BGPNeighbor{NodeID: 1}); err == nil {
		t.Fatal("expected error")
	}
	if _, err := bgpRepo.Update(ctx, &domain.BGPNeighbor{ID: 1, NodeID: 1}); err == nil {
		t.Fatal("expected error")
	}
	if err := bgpRepo.Delete(ctx, 1); err == nil {
		t.Fatal("expected error")
	}

	ruleRepo := NewCommunityRuleRepo(db)
	if _, err := ruleRepo.GetAll(ctx); err == nil {
		t.Fatal("expected error")
	}
	if _, err := ruleRepo.GetActive(ctx); err == nil {
		t.Fatal("expected error")
	}
	if _, err := ruleRepo.GetActiveRulesForNode(ctx, 1); err == nil {
		t.Fatal("expected error")
	}
	if _, err := ruleRepo.Create(ctx, &domain.CommunityRule{Community: "1:1", Severity: domain.SeverityInfo, MessageI18n: "{}", Scope: "global", Active: true}); err == nil {
		t.Fatal("expected error")
	}
	if _, err := ruleRepo.Update(ctx, &domain.CommunityRule{ID: 1, Community: "1:1", Severity: domain.SeverityInfo, MessageI18n: "{}", Scope: "global", Active: true}); err == nil {
		t.Fatal("expected error")
	}
	if err := ruleRepo.Delete(ctx, 1); err == nil {
		t.Fatal("expected error")
	}
	if err := ruleRepo.Toggle(ctx, 1); err == nil {
		t.Fatal("expected error")
	}
}

func TestBGPNeighborRepo_GetByIDMissing(t *testing.T) {
	db := setupRepoDB(t)
	repo := NewBGPNeighborRepo(db)
	got, err := repo.GetByID(context.Background(), 99999)
	if err != nil || got != nil {
		t.Fatalf("got = %+v err %v", got, err)
	}
}

func TestNodeRepo_EncryptFailureUsesPlaintext(t *testing.T) {
	db := setupRepoDB(t)
	repo := NewNodeRepo(db, "bad-key")
	created, err := repo.Create(context.Background(), &domain.Node{
		Name: "n", Type: domain.NodeTypeStandalone, Active: true, AgentToken: "plain",
	})
	if err != nil {
		t.Fatal(err)
	}
	if created.AgentToken != "plain" {
		t.Fatalf("token = %q", created.AgentToken)
	}
}

func TestNodeRepo_DecryptPlaintextLegacy(t *testing.T) {
	db := setupRepoDB(t)
	repo := NewNodeRepo(db, testCredKey)
	created, err := repo.Create(context.Background(), &domain.Node{
		Name: "legacy", Type: domain.NodeTypeStandalone, Active: true, AgentToken: "legacy-plain",
	})
	if err != nil {
		t.Fatal(err)
	}
	// Simulate legacy plaintext in DB
	if _, err := db.Exec(`UPDATE nodes SET agent_token = ? WHERE id = ?`, "legacy-plain", created.ID); err != nil {
		t.Fatal(err)
	}
	got, err := repo.GetByID(context.Background(), created.ID)
	if err != nil || got.AgentToken != "legacy-plain" {
		t.Fatalf("token = %q err %v", got.AgentToken, err)
	}
}

func TestDecryptInvalidBase64(t *testing.T) {
	key := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	_, err := Decrypt("!!!", key)
	if err == nil {
		t.Fatal("expected base64 error")
	}
}

func TestNodeRepo_CreateDefault(t *testing.T) {
	db := setupRepoDB(t)
	repo := NewNodeRepo(db, testCredKey)
	created, err := repo.Create(context.Background(), &domain.Node{
		Name: "default-node", Type: domain.NodeTypeStandalone, Active: true, IsDefault: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	got, _ := repo.GetByID(context.Background(), created.ID)
	if !got.IsDefault {
		t.Fatal("expected default node")
	}
}

func TestUserRepo_GetByIDMissing(t *testing.T) {
	db := setupRepoDB(t)
	repo := NewUserRepo(db)
	got, err := repo.GetByID(context.Background(), 99999)
	if err != nil || got != nil {
		t.Fatalf("got = %+v err %v", got, err)
	}
}

func TestMapNode_InvalidEnabledCmdsJSON(t *testing.T) {
	db := setupRepoDB(t)
	if _, err := db.Exec(`INSERT INTO nodes (name, type, enabled_cmds) VALUES ('bad', 'standalone', 'not-json')`); err != nil {
		t.Fatal(err)
	}
	repo := NewNodeRepo(db, "")
	got, err := repo.GetByID(context.Background(), 1)
	if err != nil || got.Name != "bad" {
		t.Fatalf("got = %+v err %v", got, err)
	}
}
