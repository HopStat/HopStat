package queries

import (
	"context"
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
)

func setupTestDB(t *testing.T) (*sql.DB, *Queries) {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	_, _ = db.Exec(`CREATE TABLE IF NOT EXISTS users (id INTEGER PRIMARY KEY AUTOINCREMENT, email TEXT NOT NULL UNIQUE, password_hash TEXT NOT NULL, role TEXT NOT NULL DEFAULT 'admin', created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP, last_login DATETIME)`)
	_, _ = db.Exec(`CREATE TABLE IF NOT EXISTS nodes (id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT NOT NULL, description TEXT NOT NULL DEFAULT '', type TEXT NOT NULL DEFAULT 'standalone', city TEXT NOT NULL DEFAULT '', country TEXT NOT NULL DEFAULT '', lat REAL, lon REAL, credential_id INTEGER, active INTEGER NOT NULL DEFAULT 1, enabled_cmds TEXT NOT NULL DEFAULT '[]', bgp_config TEXT, agent_url TEXT NOT NULL DEFAULT '', agent_token TEXT NOT NULL DEFAULT '', created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP, updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP)`)
	_, _ = db.Exec(`CREATE TABLE IF NOT EXISTS audit_log (id INTEGER PRIMARY KEY AUTOINCREMENT, created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP, source_ip TEXT NOT NULL, user_id INTEGER, node_id INTEGER, command TEXT NOT NULL, params TEXT NOT NULL DEFAULT '', duration_ms INTEGER NOT NULL DEFAULT 0, success INTEGER NOT NULL DEFAULT 1, error_msg TEXT NOT NULL DEFAULT '', FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE SET NULL, FOREIGN KEY (node_id) REFERENCES nodes(id) ON DELETE SET NULL)`)
	_, _ = db.Exec(`CREATE TABLE IF NOT EXISTS community_rules (id INTEGER PRIMARY KEY AUTOINCREMENT, community TEXT NOT NULL, severity TEXT NOT NULL DEFAULT 'info', message_i18n TEXT NOT NULL DEFAULT '', scope TEXT NOT NULL DEFAULT 'global', node_id INTEGER, active INTEGER NOT NULL DEFAULT 1, created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP, updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP, FOREIGN KEY (node_id) REFERENCES nodes(id) ON DELETE CASCADE)`)

	return db, New(db)
}

func TestQueries_CreateAndGetNode(t *testing.T) {
	_, q := setupTestDB(t)
	ctx := context.Background()

	node := &Node{
		Name:        "router1",
		Description: "main router",
		Type:        "standalone",
		Active:      1,
		EnabledCmds: `["ping","traceroute"]`,
		BGPConfig:   sql.NullString{String: `{"asn":65001}`, Valid: true},
		AgentURL:    "http://agent1:8080",
		AgentToken:  "secret-token",
	}

	created, err := q.CreateNode(ctx, node)
	if err != nil {
		t.Fatalf("CreateNode: %v", err)
	}
	if created.ID == 0 {
		t.Error("expected non-zero ID after insert")
	}
	if created.Name != "router1" {
		t.Errorf("Name: got %q, want %q", created.Name, "router1")
	}
	if created.Description != "main router" {
		t.Errorf("Description: got %q, want %q", created.Description, "main router")
	}
	if created.Type != "standalone" {
		t.Errorf("Type: got %q, want %q", created.Type, "standalone")
	}
	if created.Active != 1 {
		t.Errorf("Active: got %d, want 1", created.Active)
	}
	if created.EnabledCmds != `["ping","traceroute"]` {
		t.Errorf("EnabledCmds: got %q, want %q", created.EnabledCmds, `["ping","traceroute"]`)
	}
	if !created.BGPConfig.Valid || created.BGPConfig.String != `{"asn":65001}` {
		t.Errorf("BGPConfig: got %v", created.BGPConfig)
	}
	if created.AgentURL != "http://agent1:8080" {
		t.Errorf("AgentURL: got %q, want %q", created.AgentURL, "http://agent1:8080")
	}
	if created.AgentToken != "secret-token" {
		t.Errorf("AgentToken: got %q, want %q", created.AgentToken, "secret-token")
	}
	if created.CreatedAt == "" {
		t.Error("expected non-empty CreatedAt")
	}
	if created.UpdatedAt == "" {
		t.Error("expected non-empty UpdatedAt")
	}

	// Fetch by ID
	fetched, err := q.GetNodeByID(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetNodeByID: %v", err)
	}
	if fetched == nil {
		t.Fatal("GetNodeByID returned nil")
	}
	if fetched.Name != created.Name {
		t.Errorf("fetched Name: got %q, want %q", fetched.Name, created.Name)
	}
	if fetched.ID != created.ID {
		t.Errorf("fetched ID: got %d, want %d", fetched.ID, created.ID)
	}
}

func TestQueries_GetNodeByID_NotFound(t *testing.T) {
	_, q := setupTestDB(t)
	ctx := context.Background()

	node, err := q.GetNodeByID(ctx, 9999)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if node != nil {
		t.Error("expected nil node for non-existent ID")
	}
}

func TestQueries_UpdateNode(t *testing.T) {
	_, q := setupTestDB(t)
	ctx := context.Background()

	node := &Node{
		Name:        "router-old",
		Description: "old description",
		Type:        "standalone",
		Active:      1,
		EnabledCmds: `["ping"]`,
	}
	created, err := q.CreateNode(ctx, node)
	if err != nil {
		t.Fatal(err)
	}

	created.Name = "router-new"
	created.Description = "new description"
	created.Active = 0
	created.EnabledCmds = `["ping","traceroute","bgp"]`

	updated, err := q.UpdateNode(ctx, created)
	if err != nil {
		t.Fatalf("UpdateNode: %v", err)
	}
	if updated.Name != "router-new" {
		t.Errorf("Name: got %q, want %q", updated.Name, "router-new")
	}
	if updated.Description != "new description" {
		t.Errorf("Description: got %q, want %q", updated.Description, "new description")
	}
	if updated.Active != 0 {
		t.Errorf("Active: got %d, want 0", updated.Active)
	}
	if updated.EnabledCmds != `["ping","traceroute","bgp"]` {
		t.Errorf("EnabledCmds: got %q", updated.EnabledCmds)
	}

	// Verify via separate fetch
	fetched, err := q.GetNodeByID(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if fetched.Name != "router-new" {
		t.Errorf("fetched Name after update: got %q, want %q", fetched.Name, "router-new")
	}
}

func TestQueries_DeleteNode(t *testing.T) {
	_, q := setupTestDB(t)
	ctx := context.Background()

	node := &Node{
		Name: "to-delete",
		Type: "standalone",
	}
	created, err := q.CreateNode(ctx, node)
	if err != nil {
		t.Fatal(err)
	}

	if err := q.DeleteNode(ctx, created.ID); err != nil {
		t.Fatalf("DeleteNode: %v", err)
	}

	fetched, err := q.GetNodeByID(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetNodeByID after delete: %v", err)
	}
	if fetched != nil {
		t.Error("expected nil after deletion")
	}
}

func TestQueries_GetAllNodes(t *testing.T) {
	_, q := setupTestDB(t)
	ctx := context.Background()

	node1 := &Node{Name: "alpha", Type: "standalone", Active: 1}
	node2 := &Node{Name: "bravo", Type: "lgnode", Active: 0}

	if _, err := q.CreateNode(ctx, node1); err != nil {
		t.Fatal(err)
	}
	if _, err := q.CreateNode(ctx, node2); err != nil {
		t.Fatal(err)
	}

	nodes, err := q.GetAllNodes(ctx)
	if err != nil {
		t.Fatalf("GetAllNodes: %v", err)
	}
	if len(nodes) != 2 {
		t.Fatalf("expected 2 nodes, got %d", len(nodes))
	}
	// Ordered by name
	if nodes[0].Name != "alpha" {
		t.Errorf("nodes[0].Name: got %q, want %q", nodes[0].Name, "alpha")
	}
	if nodes[1].Name != "bravo" {
		t.Errorf("nodes[1].Name: got %q, want %q", nodes[1].Name, "bravo")
	}

	// Test GetActiveNodes returns only active
	active, err := q.GetActiveNodes(ctx)
	if err != nil {
		t.Fatalf("GetActiveNodes: %v", err)
	}
	if len(active) != 1 {
		t.Fatalf("expected 1 active node, got %d", len(active))
	}
	if active[0].Name != "alpha" {
		t.Errorf("active[0].Name: got %q, want %q", active[0].Name, "alpha")
	}
}


func TestQueries_CreateAndGetUser(t *testing.T) {
	_, q := setupTestDB(t)
	ctx := context.Background()

	user := &User{
		Email:        "admin@example.com",
		PasswordHash: "$2a$10$hashhash",
		Role:         "admin",
	}

	created, err := q.CreateUser(ctx, user)
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if created.ID == 0 {
		t.Error("expected non-zero ID")
	}
	if created.Email != "admin@example.com" {
		t.Errorf("Email: got %q, want %q", created.Email, "admin@example.com")
	}
	if created.PasswordHash != "$2a$10$hashhash" {
		t.Errorf("PasswordHash: got %q", created.PasswordHash)
	}
	if created.Role != "admin" {
		t.Errorf("Role: got %q, want %q", created.Role, "admin")
	}
	if created.CreatedAt == "" {
		t.Error("expected non-empty CreatedAt")
	}

	// Fetch by email
	fetched, err := q.GetUserByEmail(ctx, "admin@example.com")
	if err != nil {
		t.Fatalf("GetUserByEmail: %v", err)
	}
	if fetched == nil {
		t.Fatal("GetUserByEmail returned nil")
	}
	if fetched.ID != created.ID {
		t.Errorf("ID: got %d, want %d", fetched.ID, created.ID)
	}

	// Fetch non-existent
	notFound, err := q.GetUserByEmail(ctx, "nobody@example.com")
	if err != nil {
		t.Fatalf("GetUserByEmail non-existent: %v", err)
	}
	if notFound != nil {
		t.Error("expected nil for non-existent email")
	}
}

func TestQueries_ListUsers(t *testing.T) {
	_, q := setupTestDB(t)
	ctx := context.Background()

	user1 := &User{Email: "alice@example.com", PasswordHash: "hash1", Role: "admin"}
	user2 := &User{Email: "bob@example.com", PasswordHash: "hash2", Role: "viewer"}

	if _, err := q.CreateUser(ctx, user1); err != nil {
		t.Fatal(err)
	}
	if _, err := q.CreateUser(ctx, user2); err != nil {
		t.Fatal(err)
	}

	users, err := q.ListUsers(ctx)
	if err != nil {
		t.Fatalf("ListUsers: %v", err)
	}
	if len(users) != 2 {
		t.Fatalf("expected 2 users, got %d", len(users))
	}
	// Ordered by email
	if users[0].Email != "alice@example.com" {
		t.Errorf("users[0].Email: got %q, want %q", users[0].Email, "alice@example.com")
	}
	if users[1].Email != "bob@example.com" {
		t.Errorf("users[1].Email: got %q, want %q", users[1].Email, "bob@example.com")
	}
}

func TestQueries_DeleteUser(t *testing.T) {
	_, q := setupTestDB(t)
	ctx := context.Background()

	user := &User{Email: "deleteme@example.com", PasswordHash: "hash", Role: "admin"}
	created, err := q.CreateUser(ctx, user)
	if err != nil {
		t.Fatal(err)
	}

	if err := q.DeleteUser(ctx, created.ID); err != nil {
		t.Fatalf("DeleteUser: %v", err)
	}

	fetched, err := q.GetUserByEmail(ctx, "deleteme@example.com")
	if err != nil {
		t.Fatalf("GetUserByEmail after delete: %v", err)
	}
	if fetched != nil {
		t.Error("expected nil after user deletion")
	}
}

func TestQueries_AuditLog_CRUD(t *testing.T) {
	db, q := setupTestDB(t)
	ctx := context.Background()

	// Create a user and node to reference
	userRes, _ := db.ExecContext(ctx, `INSERT INTO users (email, password_hash, role) VALUES (?, ?, ?)`,
		"audituser@example.com", "hash", "admin")
	userID, _ := userRes.LastInsertId()

	nodeRes, _ := db.ExecContext(ctx, `INSERT INTO nodes (name, type) VALUES (?, ?)`,
		"audit-node", "standalone")
	nodeID, _ := nodeRes.LastInsertId()

	// Create audit log
	logEntry := &AuditLog{
		SourceIP:   "192.168.1.1",
		UserID:     sql.NullInt64{Int64: userID, Valid: true},
		NodeID:     sql.NullInt64{Int64: nodeID, Valid: true},
		Command:    "ping",
		Params:     "8.8.8.8",
		DurationMS: 150,
		Success:    1,
		ErrorMsg:   "",
	}
	if err := q.CreateAuditLog(ctx, logEntry); err != nil {
		t.Fatalf("CreateAuditLog: %v", err)
	}

	// Create a second log with different command
	logEntry2 := &AuditLog{
		SourceIP:   "10.0.0.1",
		UserID:     sql.NullInt64{Int64: userID, Valid: true},
		NodeID:     sql.NullInt64{Int64: nodeID, Valid: true},
		Command:    "traceroute",
		Params:     "1.1.1.1",
		DurationMS: 300,
		Success:    0,
		ErrorMsg:   "timeout",
	}
	if err := q.CreateAuditLog(ctx, logEntry2); err != nil {
		t.Fatalf("CreateAuditLog second: %v", err)
	}

	// List all logs
	logs, total, err := q.ListAuditLogs(ctx, &AuditFilter{Limit: 10})
	if err != nil {
		t.Fatalf("ListAuditLogs: %v", err)
	}
	if total != 2 {
		t.Errorf("total: got %d, want 2", total)
	}
	if len(logs) != 2 {
		t.Fatalf("expected 2 logs, got %d", len(logs))
	}

	// Filter by command
	pingLogs, pingTotal, err := q.ListAuditLogs(ctx, &AuditFilter{Command: "ping", Limit: 10})
	if err != nil {
		t.Fatalf("ListAuditLogs filtered: %v", err)
	}
	if pingTotal != 1 {
		t.Errorf("ping total: got %d, want 1", pingTotal)
	}
	if len(pingLogs) != 1 {
		t.Fatalf("expected 1 ping log, got %d", len(pingLogs))
	}
	if pingLogs[0].Command != "ping" {
		t.Errorf("command: got %q, want %q", pingLogs[0].Command, "ping")
	}
	if pingLogs[0].SourceIP != "192.168.1.1" {
		t.Errorf("source_ip: got %q, want %q", pingLogs[0].SourceIP, "192.168.1.1")
	}
	if pingLogs[0].DurationMS != 150 {
		t.Errorf("duration_ms: got %d, want 150", pingLogs[0].DurationMS)
	}
	if pingLogs[0].Success != 1 {
		t.Errorf("success: got %d, want 1", pingLogs[0].Success)
	}

	// Filter by node_id
	nodeFilterLogs, _, err := q.ListAuditLogs(ctx, &AuditFilter{NodeID: &nodeID, Limit: 10})
	if err != nil {
		t.Fatalf("ListAuditLogs node filter: %v", err)
	}
	if len(nodeFilterLogs) != 2 {
		t.Errorf("node filter: expected 2 logs, got %d", len(nodeFilterLogs))
	}

	// Filter by source_ip
	ipLogs, _, err := q.ListAuditLogs(ctx, &AuditFilter{SourceIP: "10.0.0.1", Limit: 10})
	if err != nil {
		t.Fatalf("ListAuditLogs ip filter: %v", err)
	}
	if len(ipLogs) != 1 {
		t.Errorf("ip filter: expected 1 log, got %d", len(ipLogs))
	}
	if ipLogs[0].ErrorMsg != "timeout" {
		t.Errorf("error_msg: got %q, want %q", ipLogs[0].ErrorMsg, "timeout")
	}

	// Pagination: page 0, limit 1
	page1, total, err := q.ListAuditLogs(ctx, &AuditFilter{Limit: 1, Page: 0})
	if err != nil {
		t.Fatalf("ListAuditLogs pagination: %v", err)
	}
	if total != 2 {
		t.Errorf("pagination total: got %d, want 2", total)
	}
	if len(page1) != 1 {
		t.Fatalf("pagination: expected 1 log, got %d", len(page1))
	}
}

func TestQueries_CommunityRule_CRUD(t *testing.T) {
	_, q := setupTestDB(t)
	ctx := context.Background()

	rule := &CommunityRule{
		Community:   "65001:100",
		Severity:    "warning",
		MessageI18n: `{"en":"Leaked route","de":"Leck route"}`,
		Scope:       "global",
		Active:      1,
	}

	created, err := q.CreateCommunityRule(ctx, rule)
	if err != nil {
		t.Fatalf("CreateCommunityRule: %v", err)
	}
	if created.ID == 0 {
		t.Error("expected non-zero ID")
	}
	if created.Community != "65001:100" {
		t.Errorf("Community: got %q, want %q", created.Community, "65001:100")
	}
	if created.Severity != "warning" {
		t.Errorf("Severity: got %q, want %q", created.Severity, "warning")
	}
	if created.Scope != "global" {
		t.Errorf("Scope: got %q, want %q", created.Scope, "global")
	}
	if created.Active != 1 {
		t.Errorf("Active: got %d, want 1", created.Active)
	}

	// List all rules
	rules, err := q.GetAllCommunityRules(ctx)
	if err != nil {
		t.Fatalf("GetAllCommunityRules: %v", err)
	}
	if len(rules) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(rules))
	}
	if rules[0].Community != "65001:100" {
		t.Errorf("rules[0].Community: got %q, want %q", rules[0].Community, "65001:100")
	}

	// Toggle active (1 -> 0)
	if err := q.ToggleCommunityRule(ctx, created.ID); err != nil {
		t.Fatalf("ToggleCommunityRule: %v", err)
	}
	toggled, err := q.GetCommunityRuleByID(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetCommunityRuleByID: %v", err)
	}
	if toggled.Active != 0 {
		t.Errorf("after toggle: Active got %d, want 0", toggled.Active)
	}

	// Toggle again (0 -> 1)
	if err := q.ToggleCommunityRule(ctx, created.ID); err != nil {
		t.Fatalf("ToggleCommunityRule second: %v", err)
	}
	toggled2, err := q.GetCommunityRuleByID(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if toggled2.Active != 1 {
		t.Errorf("after second toggle: Active got %d, want 1", toggled2.Active)
	}
}

func TestQueries_CleanupAuditLogs(t *testing.T) {
	db, q := setupTestDB(t)
	ctx := context.Background()

	// Insert audit logs with explicit created_at dates
	_, _ = db.ExecContext(ctx, `INSERT INTO audit_log (created_at, source_ip, command, params, duration_ms, success, error_msg) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		"2024-01-01 00:00:00", "10.0.0.1", "ping", "8.8.8.8", 100, 1, "")
	_, _ = db.ExecContext(ctx, `INSERT INTO audit_log (created_at, source_ip, command, params, duration_ms, success, error_msg) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		"2024-06-15 12:00:00", "10.0.0.2", "traceroute", "1.1.1.1", 200, 1, "")
	_, _ = db.ExecContext(ctx, `INSERT INTO audit_log (created_at, source_ip, command, params, duration_ms, success, error_msg) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		"2025-03-01 08:30:00", "10.0.0.3", "bgp_lookup", "8.8.4.4", 50, 0, "not found")

	// Verify 3 logs exist
	_, total, err := q.ListAuditLogs(ctx, &AuditFilter{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if total != 3 {
		t.Fatalf("initial count: got %d, want 3", total)
	}

	// Cleanup logs older than 2025-01-01
	deleted, err := q.CleanupAuditLogs(ctx, "2025-01-01 00:00:00")
	if err != nil {
		t.Fatalf("CleanupAuditLogs: %v", err)
	}
	if deleted != 2 {
		t.Errorf("deleted count: got %d, want 2", deleted)
	}

	// Verify only 1 log remains
	remaining, totalAfter, err := q.ListAuditLogs(ctx, &AuditFilter{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if totalAfter != 1 {
		t.Errorf("remaining total: got %d, want 1", totalAfter)
	}
	if len(remaining) != 1 {
		t.Fatalf("remaining logs: got %d, want 1", len(remaining))
	}
	if remaining[0].Command != "bgp_lookup" {
		t.Errorf("remaining command: got %q, want %q", remaining[0].Command, "bgp_lookup")
	}
}
