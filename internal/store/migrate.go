package store

import (
	"database/sql"
	"fmt"
	"log/slog"

	"golang.org/x/crypto/bcrypt"
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
		if _, err := tx.Exec(m); err != nil {
			tx.Rollback()
			return fmt.Errorf("apply migration %d: %w", version, err)
		}
		if _, err := tx.Exec("INSERT INTO schema_migrations (version) VALUES (?)", version); err != nil {
			tx.Rollback()
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
	}
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
	'admin@lookingglass.local',
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

func SeedAdminPassword(db *sql.DB, password string) error {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), 12)
	if err != nil {
		return fmt.Errorf("hash admin password: %w", err)
	}
	res, err := db.Exec(`UPDATE users SET password_hash = ? WHERE email = 'admin@lookingglass.local' AND password_hash LIKE '$2a$12$DISABLED%'`, string(hash))
	if err != nil {
		return fmt.Errorf("seed admin password: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		slog.Info("admin password already set, skipping seed")
	}
	return nil
}
