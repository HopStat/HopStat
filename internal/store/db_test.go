package store

import (
	"os"
	"path/filepath"
	"testing"
)

func TestOpen_CreatesDatabaseFile(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	db, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open(%q) error: %v", dbPath, err)
	}
	defer db.Close()

	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		t.Fatal("database file was not created")
	}

	if err := db.Ping(); err != nil {
		t.Fatalf("Ping on opened database failed: %v", err)
	}
}

func TestOpen_InMemory(t *testing.T) {
	db, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open(\":memory:\") error: %v", err)
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		t.Fatalf("Ping on in-memory database failed: %v", err)
	}

	var mode string
	if err := db.QueryRow("PRAGMA journal_mode").Scan(&mode); err != nil {
		t.Fatalf("query journal_mode: %v", err)
	}
	if mode != "memory" && mode != "wal" {
		t.Fatalf("unexpected journal mode %q, want \"memory\" or \"wal\"", mode)
	}
}

func TestMigrate_CreatesTables(t *testing.T) {
	db, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()

	if err := Migrate(db); err != nil {
		t.Fatalf("Migrate error: %v", err)
	}

	expected := []string{
		"schema_migrations",
		"users",
		"pops",
		"nodes",
		"audit_log",
		"community_rules",
	}

	rows, err := db.Query("SELECT name FROM sqlite_master WHERE type='table' ORDER BY name")
	if err != nil {
		t.Fatalf("query sqlite_master: %v", err)
	}
	defer rows.Close()

	got := map[string]bool{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatalf("scan table name: %v", err)
		}
		got[name] = true
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate sqlite_master rows: %v", err)
	}

	for _, table := range expected {
		if !got[table] {
			t.Errorf("table %q not found in sqlite_master; got tables: %v", table, got)
		}
	}
}

func TestMigrate_Idempotent(t *testing.T) {
	db, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()

	if err := Migrate(db); err != nil {
		t.Fatalf("first Migrate error: %v", err)
	}

	if err := Migrate(db); err != nil {
		t.Fatalf("second Migrate error: %v", err)
	}

	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM schema_migrations").Scan(&count); err != nil {
		t.Fatalf("count schema_migrations: %v", err)
	}
	if count != len(migrationsList()) {
		t.Fatalf("schema_migrations has %d rows after double migrate, want %d", count, len(migrationsList()))
	}
}
