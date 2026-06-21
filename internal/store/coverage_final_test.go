package store

import (
	"database/sql"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestOpenErrors(t *testing.T) {
	t.Run("open fails", func(t *testing.T) {
		old := openSQLite
		openSQLite = func(driverName, dataSourceName string) (*sql.DB, error) {
			return nil, errors.New("open failed")
		}
		t.Cleanup(func() { openSQLite = old })
		if _, err := Open(":memory:"); err == nil {
			t.Fatal("expected open error")
		}
	})

	t.Run("pragma fails", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { db.Close() })
		oldOpen := openSQLite
		oldPragma := execPragma
		openSQLite = func(driverName, dataSourceName string) (*sql.DB, error) { return db, nil }
		execPragma = func(*sql.DB, string) error { return errors.New("pragma failed") }
		t.Cleanup(func() {
			openSQLite = oldOpen
			execPragma = oldPragma
		})
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatal(err)
		}
		if _, err := Open(":memory:"); err == nil {
			t.Fatal("expected pragma error")
		}
	})

	t.Run("ping fails", func(t *testing.T) {
		db, _, err := sqlmock.New()
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { db.Close() })
		oldOpen := openSQLite
		oldPragma := execPragma
		oldPing := pingDatabase
		openSQLite = func(driverName, dataSourceName string) (*sql.DB, error) { return db, nil }
		execPragma = func(*sql.DB, string) error { return nil }
		pingDatabase = func(*sql.DB) error { return errors.New("ping failed") }
		t.Cleanup(func() {
			openSQLite = oldOpen
			execPragma = oldPragma
			pingDatabase = oldPing
		})
		if _, err := Open(":memory:"); err == nil {
			t.Fatal("expected ping error")
		}
	})
}

func TestMigrateErrorPaths(t *testing.T) {
	t.Run("bootstrap exec fails", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { db.Close() })
		mock.ExpectExec("CREATE TABLE IF NOT EXISTS schema_migrations").WillReturnError(errors.New("exec failed"))
		if err := Migrate(db); err == nil {
			t.Fatal("expected migrate error")
		}
	})

	t.Run("read migrations fails", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { db.Close() })
		mock.ExpectExec("CREATE TABLE IF NOT EXISTS schema_migrations").WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectQuery("SELECT version FROM schema_migrations").WillReturnError(errors.New("query failed"))
		if err := Migrate(db); err == nil {
			t.Fatal("expected query error")
		}
	})

	t.Run("scan migration version fails", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { db.Close() })
		mock.ExpectExec("CREATE TABLE IF NOT EXISTS schema_migrations").WillReturnResult(sqlmock.NewResult(0, 0))
		rows := sqlmock.NewRows([]string{"version"}).AddRow("bad")
		mock.ExpectQuery("SELECT version FROM schema_migrations").WillReturnRows(rows)
		if err := Migrate(db); err == nil {
			t.Fatal("expected scan error")
		}
	})
}

func TestPasswordHooks(t *testing.T) {
	db := openTestDB(t)

	t.Run("SeedAdminPassword hash error", func(t *testing.T) {
		old := bcryptGenerateFromPassword
		bcryptGenerateFromPassword = func([]byte, int) ([]byte, error) { return nil, errors.New("hash failed") }
		t.Cleanup(func() { bcryptGenerateFromPassword = old })
		if _, err := SeedAdminPassword(db, "pass"); err == nil {
			t.Fatal("expected hash error")
		}
	})

	t.Run("SeedAdminPassword exec error", func(t *testing.T) {
		db2, mock, err := sqlmock.New()
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { db2.Close() })
		mock.ExpectExec("UPDATE users SET password_hash").WillReturnError(errors.New("exec failed"))
		if _, err := SeedAdminPassword(db2, "pass"); err == nil {
			t.Fatal("expected exec error")
		}
	})

	t.Run("SetAdminPassword hash error", func(t *testing.T) {
		old := bcryptGenerateFromPassword
		bcryptGenerateFromPassword = func([]byte, int) ([]byte, error) { return nil, errors.New("hash failed") }
		t.Cleanup(func() { bcryptGenerateFromPassword = old })
		if err := SetAdminPassword(db, "pass"); err == nil {
			t.Fatal("expected hash error")
		}
	})

	t.Run("SetAdminPassword exec error", func(t *testing.T) {
		db2, mock, err := sqlmock.New()
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { db2.Close() })
		mock.ExpectExec("UPDATE users SET password_hash").WillReturnError(errors.New("exec failed"))
		if err := SetAdminPassword(db2, "pass"); err == nil {
			t.Fatal("expected exec error")
		}
	})

	t.Run("ApplyAdminPassword force error", func(t *testing.T) {
		db2, mock, err := sqlmock.New()
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { db2.Close() })
		mock.ExpectExec("UPDATE users SET password_hash").WillReturnError(errors.New("exec failed"))
		if _, err := ApplyAdminPassword(db2, "pass", true); err == nil {
			t.Fatal("expected force error")
		}
	})

	t.Run("EnsureFirstAdmin rand error", func(t *testing.T) {
		db2, err := sql.Open("sqlite", ":memory:")
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { db2.Close() })
		if err := Migrate(db2); err != nil {
			t.Fatal(err)
		}
		old := migrateRandRead
		migrateRandRead = func([]byte) (int, error) { return 0, errors.New("rand failed") }
		t.Cleanup(func() { migrateRandRead = old })
		if _, _, _, err := EnsureFirstAdmin(db2); err == nil {
			t.Fatal("expected rand error")
		}
	})

	t.Run("EnsureFirstAdmin query error", func(t *testing.T) {
		db2, mock, err := sqlmock.New()
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { db2.Close() })
		mock.ExpectQuery("SELECT email, password_hash FROM users").WillReturnError(errors.New("query failed"))
		if _, _, _, err := EnsureFirstAdmin(db2); err == nil {
			t.Fatal("expected query error")
		}
	})

	t.Run("EnsureFirstAdmin hash error", func(t *testing.T) {
		db2, err := sql.Open("sqlite", ":memory:")
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { db2.Close() })
		if err := Migrate(db2); err != nil {
			t.Fatal(err)
		}
		old := bcryptGenerateFromPassword
		bcryptGenerateFromPassword = func([]byte, int) ([]byte, error) { return nil, errors.New("hash failed") }
		t.Cleanup(func() { bcryptGenerateFromPassword = old })
		if _, _, _, err := EnsureFirstAdmin(db2); err == nil {
			t.Fatal("expected hash error")
		}
	})

	t.Run("EnsureFirstAdmin update error", func(t *testing.T) {
		db2, mock, err := sqlmock.New()
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { db2.Close() })
		rows := sqlmock.NewRows([]string{"email", "password_hash"}).
			AddRow(DefaultAdminEmail, "$2a$12$DISABLED.USE.LG_ADMIN_PASSWORD.ENV.TO.SET.INITIAL.PASSWORD")
		mock.ExpectQuery("SELECT email, password_hash FROM users").WillReturnRows(rows)
		mock.ExpectExec("UPDATE users SET password_hash").WillReturnError(errors.New("update failed"))
		if _, _, _, err := EnsureFirstAdmin(db2); err == nil {
			t.Fatal("expected update error")
		}
	})

	t.Run("EnsureFirstAdmin concurrent loss", func(t *testing.T) {
		db2, mock, err := sqlmock.New()
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { db2.Close() })
		rows := sqlmock.NewRows([]string{"email", "password_hash"}).
			AddRow(DefaultAdminEmail, "$2a$12$DISABLED.USE.LG_ADMIN_PASSWORD.ENV.TO.SET.INITIAL.PASSWORD")
		mock.ExpectQuery("SELECT email, password_hash FROM users").WillReturnRows(rows)
		mock.ExpectExec("UPDATE users SET password_hash").WillReturnResult(sqlmock.NewResult(0, 0))
		email, pass, generated, err := EnsureFirstAdmin(db2)
		if err != nil || generated || pass != "" || email != DefaultAdminEmail {
			t.Fatalf("email=%q pass=%q generated=%v err=%v", email, pass, generated, err)
		}
	})
}

func TestMigrateMoreErrorPaths(t *testing.T) {
	t.Run("iterate migrations fails", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { db.Close() })
		mock.ExpectExec("CREATE TABLE IF NOT EXISTS schema_migrations").WillReturnResult(sqlmock.NewResult(0, 0))
		rows := sqlmock.NewRows([]string{"version"}).AddRow(1).CloseError(errors.New("rows err"))
		mock.ExpectQuery("SELECT version FROM schema_migrations").WillReturnRows(rows)
		if err := Migrate(db); err == nil {
			t.Fatal("expected rows error")
		}
	})

	t.Run("begin migration fails", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { db.Close() })
		mock.ExpectExec("CREATE TABLE IF NOT EXISTS schema_migrations").WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectQuery("SELECT version FROM schema_migrations").WillReturnRows(sqlmock.NewRows([]string{"version"}))
		mock.ExpectBegin().WillReturnError(errors.New("begin failed"))
		if err := Migrate(db); err == nil {
			t.Fatal("expected begin error")
		}
	})

	t.Run("apply migration fails", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { db.Close() })
		mock.ExpectExec("CREATE TABLE IF NOT EXISTS schema_migrations").WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectQuery("SELECT version FROM schema_migrations").WillReturnRows(sqlmock.NewRows([]string{"version"}))
		mock.ExpectBegin()
		mock.ExpectExec("CREATE TABLE IF NOT EXISTS users").WillReturnError(errors.New("apply failed"))
		mock.ExpectRollback()
		if err := Migrate(db); err == nil {
			t.Fatal("expected apply error")
		}
	})

	t.Run("record migration fails", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { db.Close() })
		mock.ExpectExec("CREATE TABLE IF NOT EXISTS schema_migrations").WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectQuery("SELECT version FROM schema_migrations").WillReturnRows(sqlmock.NewRows([]string{"version"}))
		mock.ExpectBegin()
		mock.ExpectExec("CREATE TABLE IF NOT EXISTS users").WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("INSERT INTO schema_migrations").WillReturnError(errors.New("record failed"))
		mock.ExpectRollback()
		if err := Migrate(db); err == nil {
			t.Fatal("expected record error")
		}
	})

	t.Run("commit migration fails", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { db.Close() })
		mock.ExpectExec("CREATE TABLE IF NOT EXISTS schema_migrations").WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectQuery("SELECT version FROM schema_migrations").WillReturnRows(sqlmock.NewRows([]string{"version"}))
		mock.ExpectBegin()
		mock.ExpectExec("CREATE TABLE IF NOT EXISTS users").WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("INSERT INTO schema_migrations").WillReturnResult(sqlmock.NewResult(1, 1))
		mock.ExpectCommit().WillReturnError(errors.New("commit failed"))
		if err := Migrate(db); err == nil {
			t.Fatal("expected commit error")
		}
	})
}

func TestMigrationHelpersErrors(t *testing.T) {
	t.Run("tableExistsTx query error", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { db.Close() })
		mock.ExpectBegin()
		mock.ExpectQuery("SELECT name FROM sqlite_master").WillReturnError(errors.New("query failed"))
		tx, err := db.Begin()
		if err != nil {
			t.Fatal(err)
		}
		if _, err := tableExistsTx(tx, "nodes"); err == nil {
			t.Fatal("expected query error")
		}
	})

	t.Run("tableHasColumnTx scan error", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { db.Close() })
		mock.ExpectBegin()
		mock.ExpectQuery("PRAGMA table_info").WillReturnRows(
			sqlmock.NewRows([]string{"cid", "name", "type", "notnull", "dflt_value", "pk"}).
				AddRow("bad", nil, nil, nil, nil, nil),
		)
		tx, err := db.Begin()
		if err != nil {
			t.Fatal(err)
		}
		if _, err := tableHasColumnTx(tx, "nodes", "type"); err == nil {
			t.Fatal("expected scan error")
		}
	})

	t.Run("tableHasColumnTx rows err", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { db.Close() })
		mock.ExpectBegin()
		mock.ExpectQuery("PRAGMA table_info").WillReturnRows(
			sqlmock.NewRows([]string{"cid", "name", "type", "notnull", "dflt_value", "pk"}).
				AddRow(1, "type", "TEXT", 0, nil, 0).CloseError(errors.New("rows err")),
		)
		tx, err := db.Begin()
		if err != nil {
			t.Fatal(err)
		}
		if _, err := tableHasColumnTx(tx, "nodes", "missing"); err == nil {
			t.Fatal("expected rows error")
		}
	})

	t.Run("migrateBGPNeighborPeerType errors", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { db.Close() })
		mock.ExpectBegin()
		mock.ExpectQuery("SELECT name FROM sqlite_master").WillReturnError(errors.New("exists failed"))
		tx, err := db.Begin()
		if err != nil {
			t.Fatal(err)
		}
		if err := migrateBGPNeighborPeerType(tx); err == nil {
			t.Fatal("expected table exists error")
		}
	})

	t.Run("migrateBGPNeighborPeerType already has column", func(t *testing.T) {
		db, err := sql.Open("sqlite", ":memory:")
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { db.Close() })
		if _, err := db.Exec(`CREATE TABLE bgp_neighbors (id INTEGER PRIMARY KEY, peer_type TEXT NOT NULL DEFAULT 'external')`); err != nil {
			t.Fatal(err)
		}
		tx, err := db.Begin()
		if err != nil {
			t.Fatal(err)
		}
		if err := migrateBGPNeighborPeerType(tx); err != nil {
			t.Fatal(err)
		}
		tx.Rollback()
	})

	t.Run("migrateQuickQueryNodeID already has column", func(t *testing.T) {
		db := openTestDB(t)
		tx, err := db.Begin()
		if err != nil {
			t.Fatal(err)
		}
		if err := migrateQuickQueryNodeID(tx); err != nil {
			t.Fatal(err)
		}
		tx.Rollback()
	})

	t.Run("null helpers valid values", func(t *testing.T) {
		if v := nullIntArg(sql.NullInt64{Int64: 3, Valid: true}); v.(int64) != 3 {
			t.Fatalf("nullIntArg = %v", v)
		}
		if v := nullFloatArg(sql.NullFloat64{Float64: 1.5, Valid: true}); v.(float64) != 1.5 {
			t.Fatalf("nullFloatArg = %v", v)
		}
		if v := nullStringArg(sql.NullString{String: "x", Valid: true}); v.(string) != "x" {
			t.Fatalf("nullStringArg = %v", v)
		}
	})

	t.Run("migrateLegacyNodesSchema with full legacy row", func(t *testing.T) {
		db, err := sql.Open("sqlite", ":memory:")
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { db.Close() })
		legacy := `
CREATE TABLE pops (id INTEGER PRIMARY KEY, name TEXT, city TEXT, country TEXT, lat REAL, lon REAL, created_at TEXT, updated_at TEXT);
INSERT INTO pops (id, name, city, country, lat, lon) VALUES (1, 'POP', 'City', 'US', 10.5, 20.5);
CREATE TABLE nodes (
	id INTEGER PRIMARY KEY, name TEXT, description TEXT, node_type TEXT, pop_id INTEGER,
	credential_id INTEGER, active INTEGER DEFAULT 1, bgp_router_id TEXT, bgp_local_as INTEGER,
	bgp_peer_as INTEGER, bgp_peer_addr TEXT, bgp_peer_port INTEGER, bgp_auth_pwd TEXT,
	bgp_passive INTEGER, bgp_tools_source_ip TEXT, agent_url TEXT, enabled_cmds TEXT,
	created_at TEXT, updated_at TEXT, agent_token TEXT, is_default INTEGER DEFAULT 0
);
INSERT INTO nodes (id, name, description, node_type, pop_id, credential_id, active, bgp_router_id, bgp_local_as, bgp_peer_as, bgp_peer_addr, bgp_peer_port, bgp_auth_pwd, bgp_passive, bgp_tools_source_ip, agent_url, enabled_cmds, agent_token, is_default, created_at, updated_at)
VALUES (1, 'R1', 'desc', 'standalone', 1, 5, 1, '1.1.1.1', 65001, 65002, '10.0.0.1', 180, 'secret', 0, '10.0.0.2', 'http://agent', '["ping"]', 'tok', 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP);
`
		if _, err := db.Exec(legacy); err != nil {
			t.Fatal(err)
		}
		tx, err := db.Begin()
		if err != nil {
			t.Fatal(err)
		}
		if err := migrateLegacyNodesSchema(tx); err != nil {
			t.Fatal(err)
		}
		tx.Commit()
	})

	t.Run("migrateBGPNeighborPeerType alter error", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { db.Close() })
		mock.ExpectBegin()
		mock.ExpectQuery("SELECT name FROM sqlite_master").WillReturnRows(sqlmock.NewRows([]string{"name"}).AddRow("bgp_neighbors"))
		mock.ExpectQuery("PRAGMA table_info").WillReturnRows(sqlmock.NewRows([]string{"cid", "name", "type", "notnull", "dflt_value", "pk"}))
		mock.ExpectExec("ALTER TABLE bgp_neighbors ADD COLUMN peer_type").WillReturnError(errors.New("alter failed"))
		tx, err := db.Begin()
		if err != nil {
			t.Fatal(err)
		}
		if err := migrateBGPNeighborPeerType(tx); err == nil {
			t.Fatal("expected alter error")
		}
	})

	t.Run("migrateQuickQueryNodeID alter error", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { db.Close() })
		mock.ExpectBegin()
		mock.ExpectQuery("SELECT name FROM sqlite_master").WillReturnRows(sqlmock.NewRows([]string{"name"}).AddRow("quick_queries"))
		mock.ExpectQuery("PRAGMA table_info").WillReturnRows(sqlmock.NewRows([]string{"cid", "name", "type", "notnull", "dflt_value", "pk"}))
		mock.ExpectExec("ALTER TABLE quick_queries ADD COLUMN node_id").WillReturnError(errors.New("alter failed"))
		tx, err := db.Begin()
		if err != nil {
			t.Fatal(err)
		}
		if err := migrateQuickQueryNodeID(tx); err == nil {
			t.Fatal("expected alter error")
		}
	})

	t.Run("tableHasColumnTx query error", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { db.Close() })
		mock.ExpectBegin()
		mock.ExpectQuery("PRAGMA table_info").WillReturnError(errors.New("pragma failed"))
		tx, err := db.Begin()
		if err != nil {
			t.Fatal(err)
		}
		if _, err := tableHasColumnTx(tx, "nodes", "type"); err == nil {
			t.Fatal("expected query error")
		}
	})

	t.Run("migrateQuickQueryNodeID tableHasColumn error", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { db.Close() })
		mock.ExpectBegin()
		mock.ExpectQuery("SELECT name FROM sqlite_master").WillReturnRows(sqlmock.NewRows([]string{"name"}).AddRow("quick_queries"))
		mock.ExpectQuery("PRAGMA table_info").WillReturnError(errors.New("pragma failed"))
		tx, err := db.Begin()
		if err != nil {
			t.Fatal(err)
		}
		if err := migrateQuickQueryNodeID(tx); err == nil {
			t.Fatal("expected column check error")
		}
	})

	t.Run("migrateBGPNeighborPeerType update error", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { db.Close() })
		mock.ExpectBegin()
		mock.ExpectQuery("SELECT name FROM sqlite_master").WillReturnRows(sqlmock.NewRows([]string{"name"}).AddRow("bgp_neighbors"))
		mock.ExpectQuery("PRAGMA table_info").WillReturnRows(sqlmock.NewRows([]string{"cid", "name", "type", "notnull", "dflt_value", "pk"}))
		mock.ExpectExec("ALTER TABLE bgp_neighbors ADD COLUMN peer_type").WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("UPDATE bgp_neighbors SET peer_type = 'internal'").WillReturnError(errors.New("update failed"))
		tx, err := db.Begin()
		if err != nil {
			t.Fatal(err)
		}
		if err := migrateBGPNeighborPeerType(tx); err == nil {
			t.Fatal("expected update error")
		}
	})

	t.Run("migrateLegacyNodesSchema scan error", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { db.Close() })
		mock.ExpectBegin()
		mock.ExpectQuery("PRAGMA table_info").WillReturnRows(sqlmock.NewRows([]string{"cid", "name", "type", "notnull", "dflt_value", "pk"}).AddRow(1, "node_type", "TEXT", 0, nil, 0))
		mock.ExpectQuery("PRAGMA table_info").WillReturnRows(sqlmock.NewRows([]string{"cid", "name", "type", "notnull", "dflt_value", "pk"}))
		mock.ExpectExec("PRAGMA foreign_keys=OFF").WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE TABLE nodes_v16").WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectQuery("SELECT n.id").WillReturnRows(sqlmock.NewRows([]string{"bad"}).AddRow("bad"))
		tx, err := db.Begin()
		if err != nil {
			t.Fatal(err)
		}
		if err := migrateLegacyNodesSchema(tx); err == nil {
			t.Fatal("expected scan error")
		}
	})

	t.Run("migrateLegacyNodesSchema rows err", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { db.Close() })
		cols := []string{"id", "name", "description", "node_type", "pop_id", "credential_id", "active", "is_default", "enabled_cmds", "bgp_router_id", "bgp_local_as", "bgp_peer_as", "bgp_peer_addr", "bgp_peer_port", "bgp_auth_pwd", "bgp_passive", "bgp_tools_source_ip", "agent_url", "agent_token", "created_at", "updated_at", "city", "country", "lat", "lon"}
		mock.ExpectBegin()
		mock.ExpectQuery("PRAGMA table_info").WillReturnRows(sqlmock.NewRows([]string{"cid", "name", "type", "notnull", "dflt_value", "pk"}).AddRow(1, "node_type", "TEXT", 0, nil, 0))
		mock.ExpectQuery("PRAGMA table_info").WillReturnRows(sqlmock.NewRows([]string{"cid", "name", "type", "notnull", "dflt_value", "pk"}))
		mock.ExpectExec("PRAGMA foreign_keys=OFF").WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE TABLE nodes_v16").WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectQuery("SELECT n.id").WillReturnRows(sqlmock.NewRows(cols).AddRow(1, "n", "", "standalone", nil, nil, 1, 0, "[]", nil, nil, nil, nil, nil, nil, 0, "", "", "", "now", "now", "", "", nil, nil).CloseError(errors.New("rows err")))
		tx, err := db.Begin()
		if err != nil {
			t.Fatal(err)
		}
		if err := migrateLegacyNodesSchema(tx); err == nil {
			t.Fatal("expected rows error")
		}
	})

	t.Run("migrateLegacyNodesSchema drop error", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { db.Close() })
		cols := []string{"id", "name", "description", "node_type", "pop_id", "credential_id", "active", "is_default", "enabled_cmds", "bgp_router_id", "bgp_local_as", "bgp_peer_as", "bgp_peer_addr", "bgp_peer_port", "bgp_auth_pwd", "bgp_passive", "bgp_tools_source_ip", "agent_url", "agent_token", "created_at", "updated_at", "city", "country", "lat", "lon"}
		mock.ExpectBegin()
		mock.ExpectQuery("PRAGMA table_info").WillReturnRows(sqlmock.NewRows([]string{"cid", "name", "type", "notnull", "dflt_value", "pk"}).AddRow(1, "node_type", "TEXT", 0, nil, 0))
		mock.ExpectQuery("PRAGMA table_info").WillReturnRows(sqlmock.NewRows([]string{"cid", "name", "type", "notnull", "dflt_value", "pk"}))
		mock.ExpectExec("PRAGMA foreign_keys=OFF").WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE TABLE nodes_v16").WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectQuery("SELECT n.id").WillReturnRows(sqlmock.NewRows(cols).AddRow(1, "n", "", "standalone", nil, nil, 1, 0, "[]", nil, nil, nil, nil, nil, nil, 0, "", "", "", "now", "now", "", "", nil, nil))
		mock.ExpectExec("INSERT INTO nodes_v16").WillReturnResult(sqlmock.NewResult(1, 1))
		mock.ExpectExec("DROP TABLE nodes").WillReturnError(errors.New("drop failed"))
		tx, err := db.Begin()
		if err != nil {
			t.Fatal(err)
		}
		if err := migrateLegacyNodesSchema(tx); err == nil {
			t.Fatal("expected drop error")
		}
	})

	t.Run("migrateLegacyNodesSchema no node_type column", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { db.Close() })
		mock.ExpectBegin()
		mock.ExpectQuery("PRAGMA table_info").WillReturnRows(sqlmock.NewRows([]string{"cid", "name", "type", "notnull", "dflt_value", "pk"}))
		mock.ExpectQuery("PRAGMA table_info").WillReturnRows(sqlmock.NewRows([]string{"cid", "name", "type", "notnull", "dflt_value", "pk"}))
		tx, err := db.Begin()
		if err != nil {
			t.Fatal(err)
		}
		if err := migrateLegacyNodesSchema(tx); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("migrateLegacyNodesSchema query error", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { db.Close() })
		mock.ExpectBegin()
		mock.ExpectQuery("PRAGMA table_info").WillReturnRows(sqlmock.NewRows([]string{"cid", "name", "type", "notnull", "dflt_value", "pk"}).AddRow(1, "node_type", "TEXT", 0, nil, 0))
		mock.ExpectQuery("PRAGMA table_info").WillReturnRows(sqlmock.NewRows([]string{"cid", "name", "type", "notnull", "dflt_value", "pk"}))
		mock.ExpectExec("PRAGMA foreign_keys=OFF").WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE TABLE nodes_v16").WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectQuery("SELECT n.id").WillReturnError(errors.New("query failed"))
		tx, err := db.Begin()
		if err != nil {
			t.Fatal(err)
		}
		if err := migrateLegacyNodesSchema(tx); err == nil {
			t.Fatal("expected query error")
		}
	})

	t.Run("migrateLegacyNodesSchema insert error", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { db.Close() })
		cols := []string{"id", "name", "description", "node_type", "pop_id", "credential_id", "active", "is_default", "enabled_cmds", "bgp_router_id", "bgp_local_as", "bgp_peer_as", "bgp_peer_addr", "bgp_peer_port", "bgp_auth_pwd", "bgp_passive", "bgp_tools_source_ip", "agent_url", "agent_token", "created_at", "updated_at", "city", "country", "lat", "lon"}
		mock.ExpectBegin()
		mock.ExpectQuery("PRAGMA table_info").WillReturnRows(sqlmock.NewRows([]string{"cid", "name", "type", "notnull", "dflt_value", "pk"}).AddRow(1, "node_type", "TEXT", 0, nil, 0))
		mock.ExpectQuery("PRAGMA table_info").WillReturnRows(sqlmock.NewRows([]string{"cid", "name", "type", "notnull", "dflt_value", "pk"}))
		mock.ExpectExec("PRAGMA foreign_keys=OFF").WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE TABLE nodes_v16").WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectQuery("SELECT n.id").WillReturnRows(sqlmock.NewRows(cols).AddRow(1, "n", "", "standalone", nil, nil, 1, 0, "[]", nil, nil, nil, nil, nil, nil, 0, "", "", "", "now", "now", "", "", nil, nil))
		mock.ExpectExec("INSERT INTO nodes_v16").WillReturnError(errors.New("insert failed"))
		tx, err := db.Begin()
		if err != nil {
			t.Fatal(err)
		}
		if err := migrateLegacyNodesSchema(tx); err == nil {
			t.Fatal("expected insert error")
		}
	})

	t.Run("migrateLegacyNodesSchema already typed", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { db.Close() })
		mock.ExpectBegin()
		mock.ExpectQuery("PRAGMA table_info").WillReturnRows(sqlmock.NewRows([]string{"cid", "name", "type", "notnull", "dflt_value", "pk"}).AddRow(1, "node_type", "TEXT", 0, nil, 0))
		mock.ExpectQuery("PRAGMA table_info").WillReturnRows(sqlmock.NewRows([]string{"cid", "name", "type", "notnull", "dflt_value", "pk"}).AddRow(1, "type", "TEXT", 0, nil, 0))
		tx, err := db.Begin()
		if err != nil {
			t.Fatal(err)
		}
		if err := migrateLegacyNodesSchema(tx); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("migrateBGPNeighborPeerType column check error", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { db.Close() })
		mock.ExpectBegin()
		mock.ExpectQuery("SELECT name FROM sqlite_master").WillReturnRows(sqlmock.NewRows([]string{"name"}).AddRow("bgp_neighbors"))
		mock.ExpectQuery("PRAGMA table_info").WillReturnError(errors.New("pragma failed"))
		tx, err := db.Begin()
		if err != nil {
			t.Fatal(err)
		}
		if err := migrateBGPNeighborPeerType(tx); err == nil {
			t.Fatal("expected column check error")
		}
	})

	t.Run("migrateQuickQueryNodeID table exists error", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { db.Close() })
		mock.ExpectBegin()
		mock.ExpectQuery("SELECT name FROM sqlite_master").WillReturnError(errors.New("exists failed"))
		tx, err := db.Begin()
		if err != nil {
			t.Fatal(err)
		}
		if err := migrateQuickQueryNodeID(tx); err == nil {
			t.Fatal("expected exists error")
		}
	})

	t.Run("migrateLegacyNodesSchema node_type check error", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { db.Close() })
		mock.ExpectBegin()
		mock.ExpectQuery("PRAGMA table_info").WillReturnError(errors.New("pragma failed"))
		tx, err := db.Begin()
		if err != nil {
			t.Fatal(err)
		}
		if err := migrateLegacyNodesSchema(tx); err == nil {
			t.Fatal("expected node_type check error")
		}
	})

	t.Run("migrateLegacyNodesSchema type check error", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { db.Close() })
		mock.ExpectBegin()
		mock.ExpectQuery("PRAGMA table_info").WillReturnRows(sqlmock.NewRows([]string{"cid", "name", "type", "notnull", "dflt_value", "pk"}).AddRow(1, "node_type", "TEXT", 0, nil, 0))
		mock.ExpectQuery("PRAGMA table_info").WillReturnError(errors.New("pragma failed"))
		tx, err := db.Begin()
		if err != nil {
			t.Fatal(err)
		}
		if err := migrateLegacyNodesSchema(tx); err == nil {
			t.Fatal("expected type check error")
		}
	})

	t.Run("migrateLegacyNodesSchema foreign keys error", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { db.Close() })
		mock.ExpectBegin()
		mock.ExpectQuery("PRAGMA table_info").WillReturnRows(sqlmock.NewRows([]string{"cid", "name", "type", "notnull", "dflt_value", "pk"}).AddRow(1, "node_type", "TEXT", 0, nil, 0))
		mock.ExpectQuery("PRAGMA table_info").WillReturnRows(sqlmock.NewRows([]string{"cid", "name", "type", "notnull", "dflt_value", "pk"}))
		mock.ExpectExec("PRAGMA foreign_keys=OFF").WillReturnError(errors.New("pragma failed"))
		tx, err := db.Begin()
		if err != nil {
			t.Fatal(err)
		}
		if err := migrateLegacyNodesSchema(tx); err == nil {
			t.Fatal("expected foreign keys error")
		}
	})

	t.Run("migrateLegacyNodesSchema create table error", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { db.Close() })
		mock.ExpectBegin()
		mock.ExpectQuery("PRAGMA table_info").WillReturnRows(sqlmock.NewRows([]string{"cid", "name", "type", "notnull", "dflt_value", "pk"}).AddRow(1, "node_type", "TEXT", 0, nil, 0))
		mock.ExpectQuery("PRAGMA table_info").WillReturnRows(sqlmock.NewRows([]string{"cid", "name", "type", "notnull", "dflt_value", "pk"}))
		mock.ExpectExec("PRAGMA foreign_keys=OFF").WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE TABLE nodes_v16").WillReturnError(errors.New("create failed"))
		tx, err := db.Begin()
		if err != nil {
			t.Fatal(err)
		}
		if err := migrateLegacyNodesSchema(tx); err == nil {
			t.Fatal("expected create table error")
		}
	})

	t.Run("migrateLegacyNodesSchema empty node type", func(t *testing.T) {
		db, err := sql.Open("sqlite", ":memory:")
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { db.Close() })
		legacy := `
CREATE TABLE pops (id INTEGER PRIMARY KEY, name TEXT, city TEXT, country TEXT, lat REAL, lon REAL, created_at TEXT, updated_at TEXT);
CREATE TABLE nodes (
	id INTEGER PRIMARY KEY, name TEXT, description TEXT, node_type TEXT, pop_id INTEGER,
	credential_id INTEGER, active INTEGER DEFAULT 1, bgp_router_id TEXT, bgp_local_as INTEGER,
	bgp_peer_as INTEGER, bgp_peer_addr TEXT, bgp_peer_port INTEGER, bgp_auth_pwd TEXT,
	bgp_passive INTEGER, bgp_tools_source_ip TEXT, agent_url TEXT, enabled_cmds TEXT DEFAULT '[]',
	created_at TEXT, updated_at TEXT, agent_token TEXT, is_default INTEGER DEFAULT 0
);
INSERT INTO nodes (id, name, node_type, active, enabled_cmds, created_at, updated_at)
VALUES (1, 'R1', '  ', 1, '[]', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP);
`
		if _, err := db.Exec(legacy); err != nil {
			t.Fatal(err)
		}
		tx, err := db.Begin()
		if err != nil {
			t.Fatal(err)
		}
		if err := migrateLegacyNodesSchema(tx); err != nil {
			t.Fatal(err)
		}
		tx.Commit()
		var nodeType string
		if err := db.QueryRow(`SELECT type FROM nodes WHERE id = 1`).Scan(&nodeType); err != nil || nodeType != "standalone" {
			t.Fatalf("type=%q err=%v", nodeType, err)
		}
	})

	t.Run("migrateLegacyNodesSchema json marshal error", func(t *testing.T) {
		old := migrateJSONMarshal
		migrateJSONMarshal = func(v interface{}) ([]byte, error) { return nil, errors.New("marshal failed") }
		t.Cleanup(func() { migrateJSONMarshal = old })

		db, err := sql.Open("sqlite", ":memory:")
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { db.Close() })
		legacy := `
CREATE TABLE pops (id INTEGER PRIMARY KEY, name TEXT, city TEXT, country TEXT, lat REAL, lon REAL, created_at TEXT, updated_at TEXT);
CREATE TABLE nodes (
	id INTEGER PRIMARY KEY, name TEXT, description TEXT, node_type TEXT, pop_id INTEGER,
	credential_id INTEGER, active INTEGER DEFAULT 1, bgp_router_id TEXT, bgp_local_as INTEGER,
	bgp_peer_as INTEGER, bgp_peer_addr TEXT, bgp_peer_port INTEGER, bgp_auth_pwd TEXT,
	bgp_passive INTEGER, bgp_tools_source_ip TEXT, agent_url TEXT, enabled_cmds TEXT DEFAULT '[]',
	created_at TEXT, updated_at TEXT, agent_token TEXT, is_default INTEGER DEFAULT 0
);
INSERT INTO nodes (id, name, node_type, active, bgp_router_id, enabled_cmds, created_at, updated_at)
VALUES (1, 'R1', 'standalone', 1, '1.1.1.1', '[]', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP);
`
		if _, err := db.Exec(legacy); err != nil {
			t.Fatal(err)
		}
		tx, err := db.Begin()
		if err != nil {
			t.Fatal(err)
		}
		if err := migrateLegacyNodesSchema(tx); err == nil {
			t.Fatal("expected marshal error")
		}
	})

	t.Run("migrateLegacyNodesSchema rows err after scan", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { db.Close() })
		cols := []string{"id", "name", "description", "node_type", "pop_id", "credential_id", "active", "is_default", "enabled_cmds", "bgp_router_id", "bgp_local_as", "bgp_peer_as", "bgp_peer_addr", "bgp_peer_port", "bgp_auth_pwd", "bgp_passive", "bgp_tools_source_ip", "agent_url", "agent_token", "created_at", "updated_at", "city", "country", "lat", "lon"}
		mock.ExpectBegin()
		mock.ExpectQuery("PRAGMA table_info").WillReturnRows(sqlmock.NewRows([]string{"cid", "name", "type", "notnull", "dflt_value", "pk"}).AddRow(1, "node_type", "TEXT", 0, nil, 0))
		mock.ExpectQuery("PRAGMA table_info").WillReturnRows(sqlmock.NewRows([]string{"cid", "name", "type", "notnull", "dflt_value", "pk"}))
		mock.ExpectExec("PRAGMA foreign_keys=OFF").WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE TABLE nodes_v16").WillReturnResult(sqlmock.NewResult(0, 0))
		rows := sqlmock.NewRows(cols).
			AddRow(1, "n", "", "standalone", nil, nil, 1, 0, "[]", nil, nil, nil, nil, nil, nil, 0, "", "", "", "now", "now", "", "", nil, nil).
			AddRow(2, "n2", "", "standalone", nil, nil, 1, 0, "[]", nil, nil, nil, nil, nil, nil, 0, "", "", "", "now", "now", "", "", nil, nil).
			RowError(1, errors.New("rows err"))
		mock.ExpectQuery("SELECT n.id").WillReturnRows(rows)
		mock.ExpectExec("INSERT INTO nodes_v16").WillReturnResult(sqlmock.NewResult(1, 1))
		tx, err := db.Begin()
		if err != nil {
			t.Fatal(err)
		}
		if err := migrateLegacyNodesSchema(tx); err == nil {
			t.Fatal("expected rows error")
		}
	})

	t.Run("migrateLegacyNodesSchema rename error", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { db.Close() })
		cols := []string{"id", "name", "description", "node_type", "pop_id", "credential_id", "active", "is_default", "enabled_cmds", "bgp_router_id", "bgp_local_as", "bgp_peer_as", "bgp_peer_addr", "bgp_peer_port", "bgp_auth_pwd", "bgp_passive", "bgp_tools_source_ip", "agent_url", "agent_token", "created_at", "updated_at", "city", "country", "lat", "lon"}
		mock.ExpectBegin()
		mock.ExpectQuery("PRAGMA table_info").WillReturnRows(sqlmock.NewRows([]string{"cid", "name", "type", "notnull", "dflt_value", "pk"}).AddRow(1, "node_type", "TEXT", 0, nil, 0))
		mock.ExpectQuery("PRAGMA table_info").WillReturnRows(sqlmock.NewRows([]string{"cid", "name", "type", "notnull", "dflt_value", "pk"}))
		mock.ExpectExec("PRAGMA foreign_keys=OFF").WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE TABLE nodes_v16").WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectQuery("SELECT n.id").WillReturnRows(sqlmock.NewRows(cols).AddRow(1, "n", "", "standalone", nil, nil, 1, 0, "[]", nil, nil, nil, nil, nil, nil, 0, "", "", "", "now", "now", "", "", nil, nil))
		mock.ExpectExec("INSERT INTO nodes_v16").WillReturnResult(sqlmock.NewResult(1, 1))
		mock.ExpectExec("DROP TABLE nodes").WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("ALTER TABLE nodes_v16 RENAME TO nodes").WillReturnError(errors.New("rename failed"))
		tx, err := db.Begin()
		if err != nil {
			t.Fatal(err)
		}
		if err := migrateLegacyNodesSchema(tx); err == nil {
			t.Fatal("expected rename error")
		}
	})

	t.Run("migrateLegacyNodesSchema create index error", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { db.Close() })
		cols := []string{"id", "name", "description", "node_type", "pop_id", "credential_id", "active", "is_default", "enabled_cmds", "bgp_router_id", "bgp_local_as", "bgp_peer_as", "bgp_peer_addr", "bgp_peer_port", "bgp_auth_pwd", "bgp_passive", "bgp_tools_source_ip", "agent_url", "agent_token", "created_at", "updated_at", "city", "country", "lat", "lon"}
		mock.ExpectBegin()
		mock.ExpectQuery("PRAGMA table_info").WillReturnRows(sqlmock.NewRows([]string{"cid", "name", "type", "notnull", "dflt_value", "pk"}).AddRow(1, "node_type", "TEXT", 0, nil, 0))
		mock.ExpectQuery("PRAGMA table_info").WillReturnRows(sqlmock.NewRows([]string{"cid", "name", "type", "notnull", "dflt_value", "pk"}))
		mock.ExpectExec("PRAGMA foreign_keys=OFF").WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE TABLE nodes_v16").WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectQuery("SELECT n.id").WillReturnRows(sqlmock.NewRows(cols).AddRow(1, "n", "", "standalone", nil, nil, 1, 0, "[]", nil, nil, nil, nil, nil, nil, 0, "", "", "", "now", "now", "", "", nil, nil))
		mock.ExpectExec("INSERT INTO nodes_v16").WillReturnResult(sqlmock.NewResult(1, 1))
		mock.ExpectExec("DROP TABLE nodes").WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("ALTER TABLE nodes_v16 RENAME TO nodes").WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE INDEX IF NOT EXISTS idx_nodes_active").WillReturnError(errors.New("index failed"))
		tx, err := db.Begin()
		if err != nil {
			t.Fatal(err)
		}
		if err := migrateLegacyNodesSchema(tx); err == nil {
			t.Fatal("expected index error")
		}
	})
}
