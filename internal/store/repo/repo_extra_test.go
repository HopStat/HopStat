package repo

import (
	"context"
	"testing"

	"github.com/HopStat/HopStat/internal/domain"
	"github.com/HopStat/HopStat/internal/store"
)

func TestUserRepo_Full(t *testing.T) {
	db := setupRepoDB(t)
	if err := store.SetAdminPassword(db, "pass"); err != nil {
		t.Fatal(err)
	}
	repo := NewUserRepo(db)
	ctx := context.Background()

	user, err := repo.GetByEmail(ctx, store.DefaultAdminEmail)
	if err != nil || user == nil {
		t.Fatal(err)
	}
	got, err := repo.GetByID(ctx, user.ID)
	if err != nil || got == nil {
		t.Fatal(err)
	}
	if err := repo.UpdateLastLogin(ctx, user.ID); err != nil {
		t.Fatal(err)
	}
	updated, err := repo.UpdateCredentials(ctx, user.ID, store.DefaultAdminEmail, "$2a$12$updatedhashupdatedhashupdatedhashup")
	if err != nil || updated == nil {
		t.Fatalf("UpdateCredentials = %+v err %v", updated, err)
	}
}

func TestAuditRepo_CleanupAndCountToday(t *testing.T) {
	db := setupRepoDB(t)
	repo := NewAuditRepo(db)
	ctx := context.Background()

	_, err := db.ExecContext(ctx, `INSERT INTO audit_log (created_at, source_ip, command, params, duration_ms, success, error_msg) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		"2024-01-01 00:00:00", "10.0.0.1", "ping", "8.8.8.8", 100, 1, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.Log(ctx, &domain.AuditEntry{Command: "ping", Success: true}); err != nil {
		t.Fatal(err)
	}
	count, err := repo.CountToday(ctx)
	if err != nil || count < 1 {
		t.Fatalf("CountToday = %d err %v", count, err)
	}
	deleted, err := repo.Cleanup(ctx, "2025-01-01 00:00:00")
	if err != nil {
		t.Fatal(err)
	}
	if deleted != 1 {
		t.Fatalf("deleted = %d", deleted)
	}
}

func TestBGPNeighborRepo_CRUD(t *testing.T) {
	db := setupRepoDB(t)
	nodeRepo := NewNodeRepo(db, "")
	ctx := context.Background()
	node, err := nodeRepo.Create(ctx, &domain.Node{Name: "n", Type: domain.NodeTypeStandalone, Active: true})
	if err != nil {
		t.Fatal(err)
	}

	repo := NewBGPNeighborRepo(db)
	created, err := repo.Create(ctx, &domain.BGPNeighbor{
		NodeID: node.ID, LocalAS: 65001, RemoteAS: 65002,
		PeeringIP: "10.0.0.1", NeighborIP: "10.0.0.2", Multihop: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	all, err := repo.GetAll(ctx)
	if err != nil || len(all) != 1 {
		t.Fatalf("GetAll = %d err %v", len(all), err)
	}
	byNode, err := repo.GetByNodeID(ctx, node.ID)
	if err != nil || len(byNode) != 1 {
		t.Fatalf("GetByNodeID = %d err %v", len(byNode), err)
	}
	got, err := repo.GetByID(ctx, created.ID)
	if err != nil || got == nil {
		t.Fatalf("GetByID = %+v err %v", got, err)
	}
	created.NeighborIP = "10.0.0.3"
	updated, err := repo.Update(ctx, created)
	if err != nil || updated.NeighborIP != "10.0.0.3" {
		t.Fatalf("Update = %+v err %v", updated, err)
	}
	if err := repo.Delete(ctx, created.ID); err != nil {
		t.Fatal(err)
	}
}

func TestCommunityRuleRepo_GetAllUpdateDelete(t *testing.T) {
	db := setupRepoDB(t)
	repo := NewCommunityRuleRepo(db)
	ctx := context.Background()

	created, err := repo.Create(ctx, &domain.CommunityRule{
		Community: "65001:1", Severity: domain.SeverityInfo,
		MessageI18n: "{}", Scope: "global", Active: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	all, err := repo.GetAll(ctx)
	if err != nil || len(all) != 1 {
		t.Fatalf("GetAll = %d err %v", len(all), err)
	}
	created.Community = "65001:2"
	if _, err := repo.Update(ctx, created); err != nil {
		t.Fatal(err)
	}
	if err := repo.Delete(ctx, created.ID); err != nil {
		t.Fatal(err)
	}
}

func TestNodeRepo_WithCoordinatesAndEncryption(t *testing.T) {
	db := setupRepoDB(t)
	repo := NewNodeRepo(db, testCredKey)
	ctx := context.Background()

	lat, lon := 40.0, -74.0
	credID := int64(7)
	created, err := repo.Create(ctx, &domain.Node{
		Name: "geo-node", Type: domain.NodeTypeStandalone, Active: true,
		Lat: &lat, Lon: &lon, CredentialID: &credID,
		EnabledCmds: domain.DefaultEnabledCmds(), AgentToken: "tok",
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := repo.GetByID(ctx, created.ID)
	if err != nil || got.AgentToken != "tok" {
		t.Fatalf("GetByID token = %q err %v", got.AgentToken, err)
	}
	if err := repo.Delete(ctx, created.ID); err != nil {
		t.Fatal(err)
	}
}

func TestNodeRepo_EnsureDefaultOnDelete(t *testing.T) {
	db := setupRepoDB(t)
	repo := NewNodeRepo(db, "")
	ctx := context.Background()

	n1, _ := repo.Create(ctx, &domain.Node{Name: "only", Type: domain.NodeTypeStandalone, Active: true, IsDefault: true})
	n2, _ := repo.Create(ctx, &domain.Node{Name: "other", Type: domain.NodeTypeStandalone, Active: true})
	if err := repo.Delete(ctx, n1.ID); err != nil {
		t.Fatal(err)
	}
	active, err := repo.GetActive(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(active) != 1 || active[0].ID != n2.ID {
		t.Fatalf("active after delete = %+v", active)
	}
}

func TestMapUser_LastLogin(t *testing.T) {
	db := setupRepoDB(t)
	ctx := context.Background()
	_, err := db.ExecContext(ctx, `UPDATE users SET last_login = '2024-01-15 10:00:00' WHERE email = ?`, store.DefaultAdminEmail)
	if err != nil {
		t.Fatal(err)
	}
	repo := NewUserRepo(db)
	user, err := repo.GetByEmail(ctx, store.DefaultAdminEmail)
	if err != nil || user.LastLogin == nil {
		t.Fatalf("LastLogin = %v err %v", user.LastLogin, err)
	}
}

func TestNullInt64Helper(t *testing.T) {
	if v := nullInt64(nil); v.Valid {
		t.Fatal("expected invalid null int")
	}
	id := int64(5)
	if v := nullInt64(&id); !v.Valid || v.Int64 != 5 {
		t.Fatalf("nullInt64 = %+v", v)
	}
}

func TestParseTimePtr_Invalid(t *testing.T) {
	if parseTimePtr("not-a-date") != nil {
		t.Fatal("expected nil for invalid date")
	}
}

func TestAuditRepo_LogWithNodeAndUser(t *testing.T) {
	db := setupRepoDB(t)
	ctx := context.Background()
	nodeRepo := NewNodeRepo(db, "")
	node, _ := nodeRepo.Create(ctx, &domain.Node{Name: "audit-node", Type: domain.NodeTypeStandalone, Active: true})
	var userID int64
	if err := db.QueryRow(`SELECT id FROM users LIMIT 1`).Scan(&userID); err != nil {
		t.Fatal(err)
	}
	nodeID := node.ID
	repo := NewAuditRepo(db)
	if err := repo.Log(ctx, &domain.AuditEntry{
		UserID: &userID, NodeID: &nodeID, Command: "traceroute", Success: false, ErrorMsg: "fail",
	}); err != nil {
		t.Fatal(err)
	}
	entries, total, err := repo.List(ctx, domain.AuditFilter{Limit: 10})
	if err != nil || total != 1 || entries[0].NodeName == "" {
		t.Fatalf("List = %d entries=%+v err %v", total, entries, err)
	}
}
