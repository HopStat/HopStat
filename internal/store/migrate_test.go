package store

import (
	"database/sql"
	"testing"

	"golang.org/x/crypto/bcrypt"
	_ "modernc.org/sqlite"
)

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err := Migrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func adminHash(t *testing.T, db *sql.DB) string {
	t.Helper()
	var hash string
	if err := db.QueryRow(`SELECT password_hash FROM users WHERE email = ?`, DefaultAdminEmail).Scan(&hash); err != nil {
		t.Fatalf("query admin hash: %v", err)
	}
	return hash
}

func TestApplyAdminPassword_SeedsPlaceholder(t *testing.T) {
	db := openTestDB(t)

	applied, err := ApplyAdminPassword(db, "first-run-pass", false)
	if err != nil {
		t.Fatalf("ApplyAdminPassword: %v", err)
	}
	if !applied {
		t.Fatal("expected password to be applied on placeholder hash")
	}
	if err := bcrypt.CompareHashAndPassword([]byte(adminHash(t, db)), []byte("first-run-pass")); err != nil {
		t.Fatalf("password mismatch: %v", err)
	}
}

func TestApplyAdminPassword_SkipsExistingWithoutForce(t *testing.T) {
	db := openTestDB(t)
	if err := SetAdminPassword(db, "existing-pass"); err != nil {
		t.Fatalf("SetAdminPassword: %v", err)
	}

	applied, err := ApplyAdminPassword(db, "new-pass", false)
	if err != nil {
		t.Fatalf("ApplyAdminPassword: %v", err)
	}
	if applied {
		t.Fatal("expected existing password to be left unchanged")
	}
	if err := bcrypt.CompareHashAndPassword([]byte(adminHash(t, db)), []byte("existing-pass")); err != nil {
		t.Fatalf("existing password should remain: %v", err)
	}
}

func TestApplyAdminPassword_ForceResetsExisting(t *testing.T) {
	db := openTestDB(t)
	if err := SetAdminPassword(db, "old-pass"); err != nil {
		t.Fatalf("SetAdminPassword: %v", err)
	}

	applied, err := ApplyAdminPassword(db, "recovery-pass", true)
	if err != nil {
		t.Fatalf("ApplyAdminPassword: %v", err)
	}
	if !applied {
		t.Fatal("expected forced password reset to apply")
	}
	if err := bcrypt.CompareHashAndPassword([]byte(adminHash(t, db)), []byte("recovery-pass")); err != nil {
		t.Fatalf("forced password mismatch: %v", err)
	}
}

func TestMigrateLegacyNodesSchema(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	if _, err := db.Exec(`CREATE TABLE schema_migrations (version INTEGER PRIMARY KEY, applied_at DATETIME DEFAULT CURRENT_TIMESTAMP)`); err != nil {
		t.Fatal(err)
	}
	for v := 1; v <= 15; v++ {
		if _, err := db.Exec(`INSERT INTO schema_migrations (version) VALUES (?)`, v); err != nil {
			t.Fatal(err)
		}
	}

	legacy := `
CREATE TABLE pops (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	name TEXT NOT NULL,
	city TEXT NOT NULL,
	country TEXT NOT NULL,
	lat REAL,
	lon REAL,
	created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
	updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
INSERT INTO pops (id, name, city, country, lat, lon) VALUES (9, 'Home', 'Bursa', 'TR', 44, 44);
CREATE TABLE nodes (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	name TEXT NOT NULL,
	description TEXT,
	node_type TEXT NOT NULL,
	pop_id INTEGER,
	credential_id INTEGER,
	active INTEGER NOT NULL DEFAULT 1,
	bgp_router_id TEXT,
	bgp_local_as INTEGER,
	bgp_peer_as INTEGER,
	bgp_peer_addr TEXT,
	bgp_peer_port INTEGER DEFAULT 179,
	bgp_auth_pwd TEXT,
	bgp_passive INTEGER DEFAULT 0,
	bgp_tools_source_ip TEXT,
	agent_url TEXT,
	enabled_cmds TEXT NOT NULL DEFAULT '[]',
	created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
	updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
	agent_token TEXT,
	is_default INTEGER NOT NULL DEFAULT 0
);
INSERT INTO nodes (id, name, node_type, pop_id, active, enabled_cmds, is_default)
VALUES (5, 'Local-Router', 'standalone', 9, 1, '["ping","traceroute"]', 1);
`
	if _, err := db.Exec(legacy); err != nil {
		t.Fatalf("seed legacy schema: %v", err)
	}
	if err := Migrate(db); err != nil {
		t.Fatalf("migrate legacy schema: %v", err)
	}

	var nodeType, city, country string
	if err := db.QueryRow(`SELECT type, city, country FROM nodes WHERE id = 5`).Scan(&nodeType, &city, &country); err != nil {
		t.Fatalf("query migrated node: %v", err)
	}
	if nodeType != "standalone" || city != "Bursa" || country != "TR" {
		t.Fatalf("unexpected migrated node: type=%q city=%q country=%q", nodeType, city, country)
	}
}

func TestEnsureFirstAdmin_GeneratesPassword(t *testing.T) {
	db := openTestDB(t)

	email, password, generated, err := EnsureFirstAdmin(db)
	if err != nil {
		t.Fatalf("EnsureFirstAdmin: %v", err)
	}
	if !generated || password == "" || email != DefaultAdminEmail {
		t.Fatalf("generated=%v email=%q password=%q", generated, email, password)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(adminHash(t, db)), []byte(password)); err != nil {
		t.Fatalf("password mismatch: %v", err)
	}

	_, _, generated2, err := EnsureFirstAdmin(db)
	if err != nil {
		t.Fatal(err)
	}
	if generated2 {
		t.Fatal("expected second call to skip generation")
	}
}

func TestSeedAdminPassword_SkipsWhenAlreadySet(t *testing.T) {
	db := openTestDB(t)
	if err := SetAdminPassword(db, "already-set"); err != nil {
		t.Fatal(err)
	}
	applied, err := SeedAdminPassword(db, "new-pass")
	if err != nil {
		t.Fatal(err)
	}
	if applied {
		t.Fatal("expected SeedAdminPassword to skip configured admin")
	}
}

func TestSetAdminPassword_NoAdminUser(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`CREATE TABLE users (id INTEGER PRIMARY KEY, email TEXT, password_hash TEXT)`); err != nil {
		t.Fatal(err)
	}
	if err := SetAdminPassword(db, "pass"); err == nil {
		t.Fatal("expected error when admin user missing")
	}
}

func TestMigrateLegacyNodesSchema_WithBGPFields(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	if _, err := db.Exec(`CREATE TABLE schema_migrations (version INTEGER PRIMARY KEY, applied_at DATETIME DEFAULT CURRENT_TIMESTAMP)`); err != nil {
		t.Fatal(err)
	}
	for v := 1; v <= 15; v++ {
		if _, err := db.Exec(`INSERT INTO schema_migrations (version) VALUES (?)`, v); err != nil {
			t.Fatal(err)
		}
	}

	legacy := `
CREATE TABLE pops (id INTEGER PRIMARY KEY, name TEXT, city TEXT, country TEXT, lat REAL, lon REAL, created_at TEXT, updated_at TEXT);
INSERT INTO pops (id, name, city, country) VALUES (1, 'POP', 'City', 'US');
CREATE TABLE nodes (
	id INTEGER PRIMARY KEY, name TEXT, description TEXT, node_type TEXT, pop_id INTEGER,
	credential_id INTEGER, active INTEGER DEFAULT 1, bgp_router_id TEXT, bgp_local_as INTEGER,
	bgp_peer_as INTEGER, bgp_peer_addr TEXT, bgp_peer_port INTEGER, bgp_auth_pwd TEXT,
	bgp_passive INTEGER, bgp_tools_source_ip TEXT, agent_url TEXT, enabled_cmds TEXT,
	created_at TEXT, updated_at TEXT, agent_token TEXT, is_default INTEGER DEFAULT 0
);
INSERT INTO nodes (id, name, node_type, pop_id, active, bgp_router_id, bgp_local_as, bgp_peer_as, bgp_peer_addr, bgp_peer_port, bgp_auth_pwd, bgp_passive, enabled_cmds, is_default, created_at, updated_at)
VALUES (1, 'R1', 'standalone', 1, 1, '1.1.1.1', 65001, 65002, '10.0.0.1', 179, 'secret', 1, '["ping"]', 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP);
`
	if _, err := db.Exec(legacy); err != nil {
		t.Fatal(err)
	}
	if err := Migrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	var bgpConfig sql.NullString
	if err := db.QueryRow(`SELECT bgp_config FROM nodes WHERE id = 1`).Scan(&bgpConfig); err != nil {
		t.Fatal(err)
	}
	if !bgpConfig.Valid || bgpConfig.String == "" {
		t.Fatal("expected bgp_config to be populated")
	}
}

func TestMigrateBGPNeighborPeerType(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	if _, err := db.Exec(`CREATE TABLE schema_migrations (version INTEGER PRIMARY KEY)`); err != nil {
		t.Fatal(err)
	}
	for v := 1; v <= 17; v++ {
		if _, err := db.Exec(`INSERT INTO schema_migrations (version) VALUES (?)`, v); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec(`
CREATE TABLE bgp_neighbors (
	id INTEGER PRIMARY KEY, node_id INTEGER, local_as INTEGER, remote_as INTEGER,
	peering_ip TEXT, neighbor_ip TEXT, ipv6_peering_ip TEXT, ipv6_neighbor_ip TEXT,
	multihop INTEGER, default_route_as INTEGER, created_at TEXT, updated_at TEXT
);
INSERT INTO bgp_neighbors (node_id, local_as, remote_as, peering_ip, neighbor_ip, multihop, default_route_as, created_at, updated_at)
VALUES (1, 65001, 65001, '10.0.0.1', '10.0.0.2', 0, 0, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP);
`); err != nil {
		t.Fatal(err)
	}
	if err := Migrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	var peerType string
	if err := db.QueryRow(`SELECT peer_type FROM bgp_neighbors WHERE id = 1`).Scan(&peerType); err != nil {
		t.Fatal(err)
	}
	if peerType != "internal" {
		t.Fatalf("peer_type = %q, want internal", peerType)
	}
}
