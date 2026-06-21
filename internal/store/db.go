package store

import (
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"
)

var (
	openSQLite   = sql.Open
	execPragma   = func(db *sql.DB, query string) error { _, err := db.Exec(query); return err }
	pingDatabase = func(db *sql.DB) error { return db.Ping() }
)

func Open(dbPath string) (*sql.DB, error) {
	db, err := openSQLite("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	db.SetMaxOpenConns(1)

	pragmas := []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA busy_timeout=5000",
		"PRAGMA foreign_keys=ON",
		"PRAGMA synchronous=NORMAL",
	}
	for _, p := range pragmas {
		if err := execPragma(db, p); err != nil {
			db.Close()
			return nil, fmt.Errorf("set pragma %q: %w", p, err)
		}
	}

	if err := pingDatabase(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}

	return db, nil
}
