package queries

import (
	"context"
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
)

func TestQueries_ErrorPathsClosedDB(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	db.Close()
	q := New(db)
	ctx := context.Background()

	if _, err := q.GetAllNodes(ctx); err == nil {
		t.Fatal("expected GetAllNodes error")
	}
	if _, err := q.GetActiveNodes(ctx); err == nil {
		t.Fatal("expected GetActiveNodes error")
	}
	if _, err := q.GetNodeByID(ctx, 1); err == nil {
		t.Fatal("expected GetNodeByID error")
	}
	if _, err := q.CreateNode(ctx, &Node{Name: "n", Type: "standalone"}); err == nil {
		t.Fatal("expected CreateNode error")
	}
	if _, err := q.UpdateNode(ctx, &Node{ID: 1, Name: "n", Type: "standalone"}); err == nil {
		t.Fatal("expected UpdateNode error")
	}
	if err := q.DeleteNode(ctx, 1); err == nil {
		t.Fatal("expected DeleteNode error")
	}
	if err := q.SetDefaultNode(ctx, 1); err == nil {
		t.Fatal("expected SetDefaultNode error")
	}
	if _, err := q.GetUserByEmail(ctx, "a@b.c"); err == nil {
		t.Fatal("expected GetUserByEmail error")
	}
	if _, err := q.GetUserByID(ctx, 1); err == nil {
		t.Fatal("expected GetUserByID error")
	}
	if err := q.UpdateLastLogin(ctx, 1); err == nil {
		t.Fatal("expected UpdateLastLogin error")
	}
	if err := q.UpdateUser(ctx, 1, "a@b.c", "hash"); err == nil {
		t.Fatal("expected UpdateUser error")
	}
	if err := q.CreateAuditLog(ctx, &AuditLog{SourceIP: "1.1.1.1", Command: "ping"}); err == nil {
		t.Fatal("expected CreateAuditLog error")
	}
	if _, _, err := q.ListAuditLogs(ctx, &AuditFilter{Limit: 1}); err == nil {
		t.Fatal("expected ListAuditLogs error")
	}
	if _, err := q.CleanupAuditLogs(ctx, "2020-01-01"); err == nil {
		t.Fatal("expected CleanupAuditLogs error")
	}
	if _, err := q.CountAuditLogsToday(ctx); err == nil {
		t.Fatal("expected CountAuditLogsToday error")
	}
	if _, err := q.GetAllCommunityRules(ctx); err == nil {
		t.Fatal("expected GetAllCommunityRules error")
	}
	if _, err := q.GetActiveCommunityRules(ctx); err == nil {
		t.Fatal("expected GetActiveCommunityRules error")
	}
	if _, err := q.GetActiveCommunityRulesForNode(ctx, 1); err == nil {
		t.Fatal("expected GetActiveCommunityRulesForNode error")
	}
	if _, err := q.GetCommunityRuleByID(ctx, 1); err == nil {
		t.Fatal("expected GetCommunityRuleByID error")
	}
	if _, err := q.CreateCommunityRule(ctx, &CommunityRule{Community: "1:1", Severity: "info", MessageI18n: "{}", Scope: "global", Active: 1}); err == nil {
		t.Fatal("expected CreateCommunityRule error")
	}
	if _, err := q.UpdateCommunityRule(ctx, &CommunityRule{ID: 1, Community: "1:1", Severity: "info", MessageI18n: "{}", Scope: "global", Active: 1}); err == nil {
		t.Fatal("expected UpdateCommunityRule error")
	}
	if err := q.DeleteCommunityRule(ctx, 1); err == nil {
		t.Fatal("expected DeleteCommunityRule error")
	}
	if err := q.ToggleCommunityRule(ctx, 1); err == nil {
		t.Fatal("expected ToggleCommunityRule error")
	}
	if _, err := q.GetAllBGPNeighbors(ctx); err == nil {
		t.Fatal("expected GetAllBGPNeighbors error")
	}
	if _, err := q.GetBGPNeighborsByNodeID(ctx, 1); err == nil {
		t.Fatal("expected GetBGPNeighborsByNodeID error")
	}
	if _, err := q.GetBGPNeighborByID(ctx, 1); err == nil {
		t.Fatal("expected GetBGPNeighborByID error")
	}
	if _, err := q.CreateBGPNeighbor(ctx, &BGPNeighbor{NodeID: 1, PeerType: "external"}); err == nil {
		t.Fatal("expected CreateBGPNeighbor error")
	}
	if _, err := q.UpdateBGPNeighbor(ctx, &BGPNeighbor{ID: 1, NodeID: 1, PeerType: "external"}); err == nil {
		t.Fatal("expected UpdateBGPNeighbor error")
	}
	if err := q.DeleteBGPNeighbor(ctx, 1); err == nil {
		t.Fatal("expected DeleteBGPNeighbor error")
	}
	if _, err := q.GetAllQuickQueries(ctx); err == nil {
		t.Fatal("expected GetAllQuickQueries error")
	}
	if _, err := q.GetActiveQuickQueries(ctx); err == nil {
		t.Fatal("expected GetActiveQuickQueries error")
	}
	if _, err := q.GetQuickQueryByID(ctx, 1); err == nil {
		t.Fatal("expected GetQuickQueryByID error")
	}
	if _, err := q.NextQuickQuerySortOrder(ctx); err == nil {
		t.Fatal("expected NextQuickQuerySortOrder error")
	}
	if _, err := q.CreateQuickQuery(ctx, &QuickQuery{Command: "ping", Name: "n", Target: "1.1.1.1", SortOrder: 1, Active: 1}); err == nil {
		t.Fatal("expected CreateQuickQuery error")
	}
	if _, err := q.UpdateQuickQuery(ctx, &QuickQuery{ID: 1, Command: "ping", Name: "n", Target: "1.1.1.1", SortOrder: 1, Active: 1}); err == nil {
		t.Fatal("expected UpdateQuickQuery error")
	}
	if err := q.DeleteQuickQuery(ctx, 1); err == nil {
		t.Fatal("expected DeleteQuickQuery error")
	}
	if err := q.ToggleQuickQuery(ctx, 1); err == nil {
		t.Fatal("expected ToggleQuickQuery error")
	}
	if _, err := q.GetSettings(); err == nil {
		t.Fatal("expected GetSettings error")
	}
	if _, err := q.GetSetting("k"); err == nil {
		t.Fatal("expected GetSetting error")
	}
	if err := q.SetSetting("k", "v"); err == nil {
		t.Fatal("expected SetSetting error")
	}
	if err := q.SetSettings(map[string]string{"k": "v"}); err == nil {
		t.Fatal("expected SetSettings error")
	}
}
