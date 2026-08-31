package store

import (
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"strings"

	"golang.org/x/crypto/bcrypt"
)

var (
	migrateRandRead            = rand.Read
	bcryptGenerateFromPassword = bcrypt.GenerateFromPassword
	migrateJSONMarshal         = json.Marshal
)

func Migrate(db *sql.DB) error {
	migrations := []string{
		`CREATE TABLE IF NOT EXISTS schema_migrations (
			version INTEGER PRIMARY KEY,
			applied_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
	}

	for _, m := range migrations {
		if _, err := db.Exec(m); err != nil {
			return fmt.Errorf("bootstrap migrations table: %w", err)
		}
	}

	rows, err := db.Query("SELECT version FROM schema_migrations ORDER BY version")
	if err != nil {
		return fmt.Errorf("read applied migrations: %w", err)
	}
	defer rows.Close()

	applied := map[int]bool{}
	for rows.Next() {
		var v int
		if err := rows.Scan(&v); err != nil {
			return fmt.Errorf("scan migration version: %w", err)
		}
		applied[v] = true
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate migrations: %w", err)
	}

	for i, m := range migrationsList() {
		version := i + 1
		if applied[version] {
			continue
		}
		tx, err := db.Begin()
		if err != nil {
			return fmt.Errorf("begin migration %d: %w", version, err)
		}
		if err := applyMigration(tx, version, m); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("apply migration %d: %w", version, err)
		}
		if _, err := tx.Exec("INSERT INTO schema_migrations (version) VALUES (?)", version); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("record migration %d: %w", version, err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit migration %d: %w", version, err)
		}
	}

	return nil
}

func migrationsList() []string {
	return []string{
		migrationV1,
		migrationV2,
		migrationV3,
		migrationV4,
		migrationV5,
		migrationV6,
		migrationV7,
		migrationV8,
		migrationV9,
		migrationV10,
		migrationV11,
		migrationV12,
		migrationV13,
		migrationV14,
		migrationV15,
		migrationV16,
		migrationV17,
		migrationV18,
		migrationV19,
		migrationV20,
	}
}

func applyMigration(tx *sql.Tx, version int, sql string) error {
	if version == 16 {
		return migrateLegacyNodesSchema(tx)
	}
	if version == 18 {
		return migrateBGPNeighborPeerType(tx)
	}
	if version == 19 {
		return migrateQuickQueryNodeID(tx)
	}
	if version == 20 {
		return migrateBGPNeighborPassiveMode(tx)
	}
	_, err := tx.Exec(sql)
	return err
}

func migrateBGPNeighborPeerType(tx *sql.Tx) error {
	exists, err := tableExistsTx(tx, "bgp_neighbors")
	if err != nil {
		return err
	}
	if !exists {
		return nil
	}
	hasCol, err := tableHasColumnTx(tx, "bgp_neighbors", "peer_type")
	if err != nil {
		return err
	}
	if hasCol {
		return nil
	}
	if _, err := tx.Exec(`ALTER TABLE bgp_neighbors ADD COLUMN peer_type TEXT NOT NULL DEFAULT 'external'`); err != nil {
		return err
	}
	_, err = tx.Exec(`UPDATE bgp_neighbors SET peer_type = 'internal' WHERE remote_as = local_as AND local_as > 0`)
	return err
}

func tableExistsTx(tx *sql.Tx, table string) (bool, error) {
	var name string
	err := tx.QueryRow(`SELECT name FROM sqlite_master WHERE type = 'table' AND name = ?`, table).Scan(&name)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

const migrationV1 = `
CREATE TABLE IF NOT EXISTS users (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	email TEXT NOT NULL UNIQUE,
	password_hash TEXT NOT NULL,
	role TEXT NOT NULL DEFAULT 'admin',
	created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
	last_login DATETIME
);

CREATE TABLE IF NOT EXISTS pops (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	name TEXT NOT NULL,
	city TEXT NOT NULL,
	country TEXT NOT NULL,
	lat REAL,
	lon REAL,
	created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
	updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS nodes (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	name TEXT NOT NULL,
	description TEXT NOT NULL DEFAULT '',
	type TEXT NOT NULL DEFAULT 'standalone',
	pop_id INTEGER,
	credential_id INTEGER,
	active INTEGER NOT NULL DEFAULT 1,
	enabled_cmds TEXT NOT NULL DEFAULT '[]',
	bgp_config TEXT,
	agent_url TEXT NOT NULL DEFAULT '',
	agent_token TEXT NOT NULL DEFAULT '',
	created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
	updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (pop_id) REFERENCES pops(id) ON DELETE SET NULL
);

CREATE TABLE IF NOT EXISTS audit_log (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
	source_ip TEXT NOT NULL,
	user_id INTEGER,
	node_id INTEGER,
	command TEXT NOT NULL,
	params TEXT NOT NULL DEFAULT '',
	duration_ms INTEGER NOT NULL DEFAULT 0,
	success INTEGER NOT NULL DEFAULT 1,
	error_msg TEXT NOT NULL DEFAULT '',
	FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE SET NULL,
	FOREIGN KEY (node_id) REFERENCES nodes(id) ON DELETE SET NULL
);

CREATE TABLE IF NOT EXISTS community_rules (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	community TEXT NOT NULL,
	severity TEXT NOT NULL DEFAULT 'info',
	message_i18n TEXT NOT NULL DEFAULT '',
	scope TEXT NOT NULL DEFAULT 'global',
	node_id INTEGER,
	active INTEGER NOT NULL DEFAULT 1,
	created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
	updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (node_id) REFERENCES nodes(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_audit_log_created_at ON audit_log(created_at);
CREATE INDEX IF NOT EXISTS idx_audit_log_command ON audit_log(command);
CREATE INDEX IF NOT EXISTS idx_nodes_active ON nodes(active);
CREATE INDEX IF NOT EXISTS idx_community_rules_active ON community_rules(active);

-- Seed: default admin user — password set via LG_ADMIN_PASSWORD env or must be created manually
INSERT OR IGNORE INTO users (email, password_hash, role) VALUES (
	'admin@hopstat.local',
	'$2a$12$DISABLED.USE.LG_ADMIN_PASSWORD.ENV.TO.SET.INITIAL.PASSWORD',
	'admin'
);
`

const migrationV2 = `
CREATE TABLE IF NOT EXISTS settings (
	key TEXT PRIMARY KEY,
	value TEXT NOT NULL DEFAULT ''
);

INSERT OR IGNORE INTO settings (key, value) VALUES
	('site_name', 'Looking Glass'),
	('site_description', 'Network Diagnostic Platform'),
	('logo_path', ''),
	('header_color', '#1e293b'),
	('url_website', ''),
	('url_peeringdb', ''),
	('url_contact', ''),
	('url_terms', ''),
	('url_privacy', '');
`

const migrationV3 = `
ALTER TABLE nodes ADD COLUMN city TEXT NOT NULL DEFAULT '';
ALTER TABLE nodes ADD COLUMN country TEXT NOT NULL DEFAULT '';
ALTER TABLE nodes ADD COLUMN lat REAL;
ALTER TABLE nodes ADD COLUMN lon REAL;
`

const migrationV4 = `
INSERT OR IGNORE INTO settings (key, value) VALUES
	('ping_count', '5'),
	('max_hops', '30'),
	('mtr_cycles', '10');
`

const migrationV5 = `
CREATE TABLE IF NOT EXISTS bgp_neighbors (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    node_id INTEGER NOT NULL,
    local_as INTEGER NOT NULL,
    remote_as INTEGER NOT NULL,
    peering_ip TEXT NOT NULL,
    neighbor_ip TEXT NOT NULL,
    multihop INTEGER NOT NULL DEFAULT 0,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (node_id) REFERENCES nodes(id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_bgp_neighbors_node_id ON bgp_neighbors(node_id);
`

const migrationV6 = `
ALTER TABLE bgp_neighbors ADD COLUMN ipv6_peering_ip TEXT NOT NULL DEFAULT '';
ALTER TABLE bgp_neighbors ADD COLUMN ipv6_neighbor_ip TEXT NOT NULL DEFAULT '';
`

const migrationV7 = `
ALTER TABLE nodes ADD COLUMN is_default INTEGER NOT NULL DEFAULT 0;
UPDATE nodes SET is_default = 1
WHERE id = (SELECT id FROM nodes ORDER BY id LIMIT 1)
  AND (SELECT COUNT(*) FROM nodes) = 1;
`

const migrationV8 = `
PRAGMA foreign_keys=OFF;
CREATE TABLE users_new (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	email TEXT NOT NULL UNIQUE,
	password_hash TEXT NOT NULL,
	created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
	last_login DATETIME
);
INSERT INTO users_new (id, email, password_hash, created_at, last_login)
SELECT id, email, password_hash, created_at, last_login
FROM users
ORDER BY CASE WHEN email = 'admin@hopstat.local' THEN 0 ELSE 1 END, id
LIMIT 1;
DROP TABLE users;
ALTER TABLE users_new RENAME TO users;
PRAGMA foreign_keys=ON;
`

const migrationV9 = `
UPDATE nodes SET is_default = 1
WHERE id = (SELECT id FROM nodes ORDER BY id LIMIT 1)
  AND NOT EXISTS (SELECT 1 FROM nodes WHERE is_default = 1);
`

const migrationV10 = `
DELETE FROM settings WHERE key = 'mtr_cycles';
UPDATE nodes SET enabled_cmds = REPLACE(REPLACE(enabled_cmds, ',"mtr"', ''), '"mtr",', '');
`

const migrationV11 = `
INSERT OR IGNORE INTO settings (key, value) VALUES
	('geoip_license_key', ''),
	('geoip_account_id', ''),
	('geoip_update_interval', '72h'),
	('geoip_asn_last_download', ''),
	('geoip_city_last_download', '');
`

const migrationV12 = `
UPDATE nodes SET enabled_cmds = REPLACE(REPLACE(enabled_cmds, ',"as_path"', ''), '"as_path",', '');
`

const migrationV13 = `
INSERT OR IGNORE INTO settings (key, value) VALUES
	('default_route_as', '');
`

const migrationV14 = `
ALTER TABLE bgp_neighbors ADD COLUMN default_route_as INTEGER NOT NULL DEFAULT 0;
UPDATE bgp_neighbors
SET default_route_as = (
	SELECT CAST(TRIM(value) AS INTEGER)
	FROM settings
	WHERE key = 'default_route_as'
	  AND TRIM(value) != ''
	  AND TRIM(value) GLOB '[0-9]*'
)
WHERE default_route_as = 0
  AND EXISTS (
	SELECT 1 FROM settings
	WHERE key = 'default_route_as'
	  AND TRIM(value) != ''
	  AND TRIM(value) GLOB '[0-9]*'
  );
DELETE FROM settings WHERE key = 'default_route_as';
`

const migrationV15 = `
INSERT OR IGNORE INTO settings (key, value) VALUES
	('traceroute_max_timeouts', '5');
`

// migrationV16 upgrades pre-v3 nodes tables that still use node_type + inline BGP columns.
const migrationV16 = `SELECT 1`

const migrationV17 = `
CREATE TABLE IF NOT EXISTS quick_queries (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	command TEXT NOT NULL,
	name TEXT NOT NULL DEFAULT '',
	target TEXT NOT NULL,
	sort_order INTEGER NOT NULL DEFAULT 0,
	active INTEGER NOT NULL DEFAULT 1,
	created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
	updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

INSERT INTO quick_queries (command, name, target, sort_order, active)
SELECT 'ping', 'Cloudflare', '1.1.1.1', 1, 1
WHERE NOT EXISTS (SELECT 1 FROM quick_queries LIMIT 1);

INSERT INTO quick_queries (command, name, target, sort_order, active)
SELECT 'traceroute', 'VALVE', '146.66.155.1', 2, 1
WHERE (SELECT COUNT(*) FROM quick_queries) = 1;

INSERT INTO quick_queries (command, name, target, sort_order, active)
SELECT 'bgp_route', 'Google', '8.8.8.8', 3, 1
WHERE (SELECT COUNT(*) FROM quick_queries) = 2;
`

const migrationV18 = `
ALTER TABLE bgp_neighbors ADD COLUMN peer_type TEXT NOT NULL DEFAULT 'external';
UPDATE bgp_neighbors SET peer_type = 'internal' WHERE remote_as = local_as AND local_as > 0;
`

const migrationV19 = `SELECT 1`

const migrationV20 = `SELECT 1`

func migrateBGPNeighborPassiveMode(tx *sql.Tx) error {
	exists, err := tableExistsTx(tx, "bgp_neighbors")
	if err != nil {
		return err
	}
	if !exists {
		return nil
	}
	hasCol, err := tableHasColumnTx(tx, "bgp_neighbors", "passive_mode")
	if err != nil {
		return err
	}
	if hasCol {
		return nil
	}
	if _, err := tx.Exec(`ALTER TABLE bgp_neighbors ADD COLUMN passive_mode INTEGER NOT NULL DEFAULT 0`); err != nil {
		return err
	}
	_, err = tx.Exec(`UPDATE bgp_neighbors SET passive_mode = 1 WHERE remote_as = local_as AND local_as > 0`)
	return err
}

func migrateQuickQueryNodeID(tx *sql.Tx) error {
	exists, err := tableExistsTx(tx, "quick_queries")
	if err != nil {
		return err
	}
	if !exists {
		return nil
	}
	hasCol, err := tableHasColumnTx(tx, "quick_queries", "node_id")
	if err != nil {
		return err
	}
	if hasCol {
		return nil
	}
	_, err = tx.Exec(`ALTER TABLE quick_queries ADD COLUMN node_id INTEGER`)
	return err
}

const DefaultAdminEmail = "admin@hopstat.local"

func migrateLegacyNodesSchema(tx *sql.Tx) error {
	hasNodeType, err := tableHasColumnTx(tx, "nodes", "node_type")
	if err != nil {
		return err
	}
	hasType, err := tableHasColumnTx(tx, "nodes", "type")
	if err != nil {
		return err
	}
	if !hasNodeType || hasType {
		return nil
	}

	if _, err := tx.Exec(`PRAGMA foreign_keys=OFF`); err != nil {
		return err
	}

	if _, err := tx.Exec(`
CREATE TABLE nodes_v16 (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	name TEXT NOT NULL,
	description TEXT NOT NULL DEFAULT '',
	type TEXT NOT NULL DEFAULT 'standalone',
	city TEXT NOT NULL DEFAULT '',
	country TEXT NOT NULL DEFAULT '',
	lat REAL,
	lon REAL,
	credential_id INTEGER,
	active INTEGER NOT NULL DEFAULT 1,
	is_default INTEGER NOT NULL DEFAULT 0,
	enabled_cmds TEXT NOT NULL DEFAULT '[]',
	bgp_config TEXT,
	agent_url TEXT NOT NULL DEFAULT '',
	agent_token TEXT NOT NULL DEFAULT '',
	created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
	updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
)`); err != nil {
		return err
	}

	rows, err := tx.Query(`
SELECT n.id, n.name, COALESCE(n.description, ''), n.node_type, n.pop_id, n.credential_id, n.active,
       COALESCE(n.is_default, 0), n.enabled_cmds,
       n.bgp_router_id, n.bgp_local_as, n.bgp_peer_as, n.bgp_peer_addr, n.bgp_peer_port,
       COALESCE(n.bgp_auth_pwd, ''), COALESCE(n.bgp_passive, 0), COALESCE(n.bgp_tools_source_ip, ''),
       COALESCE(n.agent_url, ''), COALESCE(n.agent_token, ''), n.created_at, n.updated_at,
       COALESCE(p.city, ''), COALESCE(p.country, ''), p.lat, p.lon
FROM nodes n
LEFT JOIN pops p ON p.id = n.pop_id
ORDER BY n.id`)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var (
			id, popID, credentialID, active, isDefault sql.NullInt64
			name, description, nodeType, enabledCmds   string
			bgpRouterID, bgpPeerAddr, bgpAuthPwd       sql.NullString
			bgpToolsSourceIP, agentURL, agentToken     string
			bgpLocalAS, bgpPeerAS, bgpPeerPort         sql.NullInt64
			bgpPassive                                 int
			createdAt, updatedAt                       string
			city, country                              string
			lat, lon                                   sql.NullFloat64
		)
		if err := rows.Scan(
			&id, &name, &description, &nodeType, &popID, &credentialID, &active,
			&isDefault, &enabledCmds,
			&bgpRouterID, &bgpLocalAS, &bgpPeerAS, &bgpPeerAddr, &bgpPeerPort,
			&bgpAuthPwd, &bgpPassive, &bgpToolsSourceIP,
			&agentURL, &agentToken, &createdAt, &updatedAt,
			&city, &country, &lat, &lon,
		); err != nil {
			return err
		}

		nodeType = strings.TrimSpace(nodeType)
		if nodeType == "" {
			nodeType = "standalone"
		}

		var bgpConfig sql.NullString
		if bgpRouterID.Valid || bgpLocalAS.Valid || bgpPeerAS.Valid || bgpPeerAddr.Valid {
			peerPort := uint16(179)
			if bgpPeerPort.Valid && bgpPeerPort.Int64 > 0 && bgpPeerPort.Int64 <= math.MaxUint16 {
				peerPort = uint16(bgpPeerPort.Int64)
			}
			cfg := map[string]any{
				"router_id":       strings.TrimSpace(bgpRouterID.String),
				"local_as":        asNumber(bgpLocalAS),
				"peer_as":         asNumber(bgpPeerAS),
				"peer_addr":       strings.TrimSpace(bgpPeerAddr.String),
				"peer_port":       peerPort,
				"passive_mode":    bgpPassive != 0,
				"tools_source_ip": strings.TrimSpace(bgpToolsSourceIP),
			}
			if bgpAuthPwd.Valid && strings.TrimSpace(bgpAuthPwd.String) != "" {
				cfg["auth_pwd"] = strings.TrimSpace(bgpAuthPwd.String)
			}
			raw, err := migrateJSONMarshal(cfg)
			if err != nil {
				return fmt.Errorf("encode bgp_config for node %d: %w", id.Int64, err)
			}
			bgpConfig = sql.NullString{String: string(raw), Valid: true}
		}

		if _, err := tx.Exec(`
INSERT INTO nodes_v16 (
	id, name, description, type, city, country, lat, lon, credential_id, active, is_default,
	enabled_cmds, bgp_config, agent_url, agent_token, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			id.Int64, name, description, nodeType, city, country, nullFloatArg(lat), nullFloatArg(lon),
			nullIntArg(credentialID), active.Int64, isDefault.Int64,
			enabledCmds, nullStringArg(bgpConfig), agentURL, agentToken, createdAt, updatedAt,
		); err != nil {
			return err
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}

	if _, err := tx.Exec(`DROP TABLE nodes`); err != nil {
		return err
	}
	if _, err := tx.Exec(`ALTER TABLE nodes_v16 RENAME TO nodes`); err != nil {
		return err
	}
	if _, err := tx.Exec(`CREATE INDEX IF NOT EXISTS idx_nodes_active ON nodes(active)`); err != nil {
		return err
	}
	_, err = tx.Exec(`PRAGMA foreign_keys=ON`)
	return err
}

func tableHasColumnTx(tx *sql.Tx, table, column string) (bool, error) {
	rows, err := tx.Query(`PRAGMA table_info(` + table + `)`)
	if err != nil {
		return false, err
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, colType string
		var notNull, pk int
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &colType, &notNull, &dflt, &pk); err != nil {
			return false, err
		}
		if name == column {
			return true, nil
		}
	}
	return false, rows.Err()
}

func nullFloatArg(v sql.NullFloat64) any {
	if !v.Valid {
		return nil
	}
	return v.Float64
}

func nullIntArg(v sql.NullInt64) any {
	if !v.Valid {
		return nil
	}
	return v.Int64
}

func nullStringArg(v sql.NullString) any {
	if !v.Valid {
		return nil
	}
	return v.String
}

// asNumber reads a legacy AS number column. Values outside the range a real AS number
// can occupy would wrap silently on conversion, so they are treated as unset.
func asNumber(v sql.NullInt64) uint32 {
	if !v.Valid || v.Int64 < 0 || v.Int64 > math.MaxUint32 {
		return 0
	}
	return uint32(v.Int64)
}

func SeedAdminPassword(db *sql.DB, password string) (bool, error) {
	hash, err := bcryptGenerateFromPassword([]byte(password), 12)
	if err != nil {
		return false, fmt.Errorf("hash admin password: %w", err)
	}
	res, err := db.Exec(`UPDATE users SET password_hash = ? WHERE password_hash LIKE '$2a$12$DISABLED%'`, string(hash))
	if err != nil {
		return false, fmt.Errorf("seed admin password: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		slog.Info("admin password already set, skipping seed")
		return false, nil
	}
	return true, nil
}

// SetAdminPassword unconditionally updates the admin user's password.
// Used in tests and manual recovery flows.
func SetAdminPassword(db *sql.DB, password string) error {
	hash, err := bcryptGenerateFromPassword([]byte(password), 12)
	if err != nil {
		return fmt.Errorf("hash admin password: %w", err)
	}
	res, err := db.Exec(`UPDATE users SET password_hash = ? WHERE email = ?`, string(hash), DefaultAdminEmail)
	if err != nil {
		return fmt.Errorf("set admin password: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("no admin user found")
	}
	return nil
}

// ApplyAdminPassword sets the admin password. When force is true (e.g. config was
// regenerated while the database was kept), the password is always replaced.
// Otherwise only the DISABLED placeholder from first migration is seeded.
func ApplyAdminPassword(db *sql.DB, password string, force bool) (applied bool, err error) {
	if force {
		if err := SetAdminPassword(db, password); err != nil {
			return false, err
		}
		return true, nil
	}
	return SeedAdminPassword(db, password)
}

// EnsureFirstAdmin sets a random password on the admin account if it has never
// been configured (i.e. still carries the DISABLED placeholder hash).
// Returns the email and generated password when generated=true.
// The final UPDATE is conditional on the hash still being DISABLED, so concurrent
// calls are safe — only one will win and the others will return generated=false.
func EnsureFirstAdmin(db *sql.DB) (email, password string, generated bool, err error) {
	var hash string
	row := db.QueryRow(`SELECT email, password_hash FROM users ORDER BY id LIMIT 1`)
	if scanErr := row.Scan(&email, &hash); scanErr == sql.ErrNoRows {
		return "", "", false, nil
	} else if scanErr != nil {
		return "", "", false, fmt.Errorf("query admin user: %w", scanErr)
	}

	if !strings.HasPrefix(hash, "$2a$12$DISABLED") {
		return email, "", false, nil // already configured
	}

	b := make([]byte, 18)
	if _, err = migrateRandRead(b); err != nil {
		return "", "", false, fmt.Errorf("generate password: %w", err)
	}
	password = base64.RawURLEncoding.EncodeToString(b) // 24 URL-safe chars

	hashed, err := bcryptGenerateFromPassword([]byte(password), 12)
	if err != nil {
		return "", "", false, fmt.Errorf("hash admin password: %w", err)
	}

	// Atomic conditional UPDATE: only succeeds if hash still has DISABLED prefix.
	// If another process already set the password, RowsAffected == 0 and we return
	// generated=false rather than claiming a password that was never stored.
	res, err := db.Exec(
		`UPDATE users SET password_hash = ? WHERE password_hash LIKE '$2a$12$DISABLED%'`,
		string(hashed),
	)
	if err != nil {
		return "", "", false, fmt.Errorf("update admin password: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return email, "", false, nil
	}
	return email, password, true, nil
}
