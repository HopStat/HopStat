package store

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"
)

func TestEnsureFirstAdmin_NoUsers(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`CREATE TABLE users (id INTEGER PRIMARY KEY, email TEXT, password_hash TEXT)`); err != nil {
		t.Fatal(err)
	}
	email, password, generated, err := EnsureFirstAdmin(db)
	if err != nil {
		t.Fatal(err)
	}
	if generated || email != "" || password != "" {
		t.Fatalf("generated=%v email=%q password=%q", generated, email, password)
	}
}

func TestMigrateQuickQueryNodeID_NoTable(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`CREATE TABLE schema_migrations (version INTEGER PRIMARY KEY)`); err != nil {
		t.Fatal(err)
	}
	for v := 1; v <= 18; v++ {
		if _, err := db.Exec(`INSERT INTO schema_migrations (version) VALUES (?)`, v); err != nil {
			t.Fatal(err)
		}
	}
	if err := Migrate(db); err != nil {
		t.Fatalf("migrate v19: %v", err)
	}
}

func TestMigrateBGPNeighborPeerType_NoTable(t *testing.T) {
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
	if err := Migrate(db); err != nil {
		t.Fatalf("migrate v18: %v", err)
	}
}

func TestOpen_InvalidPath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "db.sqlite")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	db, err := Open(path)
	if err != nil {
		t.Fatalf("Open nested path: %v", err)
	}
	db.Close()
}

func TestMigrateLegacyNodesSchema_AlreadyMigrated(t *testing.T) {
	db := openTestDB(t)
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if err := migrateLegacyNodesSchema(tx); err != nil {
		t.Fatal(err)
	}
	tx.Rollback()
}

func TestNullHelpers(t *testing.T) {
	if v := nullIntArg(sql.NullInt64{}); v != nil {
		t.Fatalf("nullIntArg invalid = %v", v)
	}
	if v := asNumber(sql.NullInt64{}); v != 0 {
		t.Fatalf("asNumber invalid = %d", v)
	}
	if v := asNumber(sql.NullInt64{Int64: 5, Valid: true}); v != 5 {
		t.Fatalf("asNumber valid = %d", v)
	}
}

func TestTableExistsTxMissing(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	ok, err := tableExistsTx(tx, "missing_table")
	if err != nil || ok {
		t.Fatalf("tableExistsTx = %v err %v", ok, err)
	}
	tx.Rollback()
}
