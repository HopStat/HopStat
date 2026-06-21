package queries

import (
	"context"
	"database/sql"
	"testing"

	"github.com/HopStat/HopStat/internal/store"
	_ "modernc.org/sqlite"
)

func setupMigratedDB(t *testing.T) (*sql.DB, *Queries) {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err := store.Migrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db, New(db)
}

func TestQueries_Settings(t *testing.T) {
	_, q := setupMigratedDB(t)

	if err := q.SetSetting("site_name", "Test LG"); err != nil {
		t.Fatal(err)
	}
	val, err := q.GetSetting("site_name")
	if err != nil || val != "Test LG" {
		t.Fatalf("GetSetting = %q err %v", val, err)
	}
	missing, err := q.GetSetting("missing_key")
	if err != nil || missing != "" {
		t.Fatalf("missing key = %q err %v", missing, err)
	}

	if err := q.SetSettings(map[string]string{"url_website": "https://example.com", "header_color": "#000"}); err != nil {
		t.Fatal(err)
	}
	all, err := q.GetSettings()
	if err != nil {
		t.Fatal(err)
	}
	if all["url_website"] != "https://example.com" {
		t.Fatalf("settings = %#v", all)
	}
}

func TestQueries_UserByIDAndUpdates(t *testing.T) {
	db, q := setupMigratedDB(t)
	ctx := context.Background()

	var userID int64
	if err := db.QueryRow(`SELECT id FROM users WHERE email = ?`, store.DefaultAdminEmail).Scan(&userID); err != nil {
		t.Fatal(err)
	}

	user, err := q.GetUserByID(ctx, userID)
	if err != nil || user == nil {
		t.Fatalf("GetUserByID: %+v err %v", user, err)
	}
	if user, err = q.GetUserByID(ctx, 99999); err != nil || user != nil {
		t.Fatalf("missing user = %+v err %v", user, err)
	}

	if err := q.UpdateLastLogin(ctx, userID); err != nil {
		t.Fatal(err)
	}
	if err := q.UpdateUser(ctx, userID, store.DefaultAdminEmail, "$2a$12$newhashhashhashhashhashhashhashhashhashhash"); err != nil {
		t.Fatal(err)
	}
}

func TestQueries_SetDefaultNode(t *testing.T) {
	_, q := setupMigratedDB(t)
	ctx := context.Background()

	n1, err := q.CreateNode(ctx, &Node{Name: "a", Type: "standalone", Active: 1})
	if err != nil {
		t.Fatal(err)
	}
	n2, err := q.CreateNode(ctx, &Node{Name: "b", Type: "standalone", Active: 1})
	if err != nil {
		t.Fatal(err)
	}
	if err := q.SetDefaultNode(ctx, n2.ID); err != nil {
		t.Fatal(err)
	}
	got, _ := q.GetNodeByID(ctx, n2.ID)
	if got.IsDefault != 1 {
		t.Fatal("n2 should be default")
	}
	got1, _ := q.GetNodeByID(ctx, n1.ID)
	if got1.IsDefault != 0 {
		t.Fatal("n1 should not be default")
	}
	if err := q.SetDefaultNode(ctx, 99999); err != sql.ErrNoRows {
		t.Fatalf("SetDefaultNode missing = %v", err)
	}
}

func TestQueries_CommunityRuleFullCRUD(t *testing.T) {
	_, q := setupMigratedDB(t)
	ctx := context.Background()

	created, err := q.CreateCommunityRule(ctx, &CommunityRule{
		Community: "65001:1", Severity: "info", MessageI18n: "{}", Scope: "global", Active: 1,
	})
	if err != nil {
		t.Fatal(err)
	}

	active, err := q.GetActiveCommunityRules(ctx)
	if err != nil || len(active) != 1 {
		t.Fatalf("active = %d err %v", len(active), err)
	}

	created.Community = "65001:2"
	updated, err := q.UpdateCommunityRule(ctx, created)
	if err != nil || updated.Community != "65001:2" {
		t.Fatalf("UpdateCommunityRule = %+v err %v", updated, err)
	}

	all, err := q.GetAllCommunityRules(ctx)
	if err != nil || len(all) != 1 {
		t.Fatalf("GetAll = %d err %v", len(all), err)
	}

	nodeID := int64(1)
	forNode, err := q.GetActiveCommunityRulesForNode(ctx, nodeID)
	if err != nil {
		t.Fatal(err)
	}
	if len(forNode) != 1 {
		t.Fatalf("forNode len = %d", len(forNode))
	}

	if err := q.DeleteCommunityRule(ctx, created.ID); err != nil {
		t.Fatal(err)
	}
}

func TestQueries_BGPNeighborCRUD(t *testing.T) {
	db, q := setupMigratedDB(t)
	ctx := context.Background()

	nodeRes, err := db.ExecContext(ctx, `INSERT INTO nodes (name, type) VALUES ('n', 'standalone')`)
	if err != nil {
		t.Fatal(err)
	}
	nodeID, _ := nodeRes.LastInsertId()

	created, err := q.CreateBGPNeighbor(ctx, &BGPNeighbor{
		NodeID: nodeID, LocalAS: 65001, RemoteAS: 65002,
		PeeringIP: "10.0.0.1", NeighborIP: "10.0.0.2", PeerType: "external",
	})
	if err != nil {
		t.Fatal(err)
	}

	all, err := q.GetAllBGPNeighbors(ctx)
	if err != nil || len(all) != 1 {
		t.Fatalf("GetAll = %d err %v", len(all), err)
	}
	byNode, err := q.GetBGPNeighborsByNodeID(ctx, nodeID)
	if err != nil || len(byNode) != 1 {
		t.Fatalf("GetByNodeID = %d err %v", len(byNode), err)
	}
	got, err := q.GetBGPNeighborByID(ctx, created.ID)
	if err != nil || got == nil {
		t.Fatalf("GetByID = %+v err %v", got, err)
	}
	if got, err = q.GetBGPNeighborByID(ctx, 99999); err != nil || got != nil {
		t.Fatalf("missing neighbor = %+v err %v", got, err)
	}

	created.NeighborIP = "10.0.0.3"
	updated, err := q.UpdateBGPNeighbor(ctx, created)
	if err != nil || updated.NeighborIP != "10.0.0.3" {
		t.Fatalf("Update = %+v err %v", updated, err)
	}
	if err := q.DeleteBGPNeighbor(ctx, created.ID); err != nil {
		t.Fatal(err)
	}
}

func TestQueries_QuickQueryCRUD(t *testing.T) {
	_, q := setupMigratedDB(t)
	ctx := context.Background()

	next, err := q.NextQuickQuerySortOrder(ctx)
	if err != nil || next < 1 {
		t.Fatalf("NextQuickQuerySortOrder = %d err %v", next, err)
	}

	created, err := q.CreateQuickQuery(ctx, &QuickQuery{
		Command: "ping", Name: "Google", Target: "8.8.8.8", SortOrder: next, Active: 1,
	})
	if err != nil {
		t.Fatal(err)
	}

	all, err := q.GetAllQuickQueries(ctx)
	if err != nil || len(all) == 0 {
		t.Fatalf("GetAllQuickQueries len=%d err=%v", len(all), err)
	}
	active, err := q.GetActiveQuickQueries(ctx)
	if err != nil || len(active) == 0 {
		t.Fatalf("GetActiveQuickQueries len=%d err=%v", len(active), err)
	}
	got, err := q.GetQuickQueryByID(ctx, created.ID)
	if err != nil || got == nil {
		t.Fatalf("GetQuickQueryByID = %+v err %v", got, err)
	}
	if got, err = q.GetQuickQueryByID(ctx, 99999); err != nil || got != nil {
		t.Fatalf("missing qq = %+v err %v", got, err)
	}

	created.Name = "Cloudflare"
	updated, err := q.UpdateQuickQuery(ctx, created)
	if err != nil || updated.Name != "Cloudflare" {
		t.Fatalf("UpdateQuickQuery = %+v err %v", updated, err)
	}
	if err := q.ToggleQuickQuery(ctx, created.ID); err != nil {
		t.Fatal(err)
	}
	if err := q.DeleteQuickQuery(ctx, created.ID); err != nil {
		t.Fatal(err)
	}
}
