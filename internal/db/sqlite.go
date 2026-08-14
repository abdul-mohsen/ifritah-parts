package db

import (
	"database/sql"
	"fmt"
	"log"
	"os"

	_ "modernc.org/sqlite"
)

// NewSQLite opens a SQLite database for offline mode.
// Returns nil if the file doesn't exist (non-fatal).
func NewSQLite(path string) *sql.DB {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		log.Printf("⚠ SQLite DB not found at %s (offline mode unavailable)", path)
		return nil
	}

	db, err := sql.Open("sqlite", path+"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)")
	if err != nil {
		log.Printf("⚠ SQLite open error (non-fatal): %v", err)
		return nil
	}
	db.SetMaxOpenConns(4) // allow concurrent reads; SQLite WAL mode supports multiple readers

	if err := db.Ping(); err != nil {
		log.Printf("⚠ SQLite unreachable (non-fatal): %v", err)
		db.Close()
		return nil
	}

	// Verify the key table exists
	var cnt int
	if err := db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='hk_parts_cache'").Scan(&cnt); err != nil || cnt == 0 {
		log.Printf("⚠ SQLite DB missing hk_parts_cache table")
		db.Close()
		return nil
	}

	fmt.Println("✓ SQLite offline DB loaded:", path)
	return db
}
