package db

import (
	"database/sql"
	"fmt"
	"log"
	"time"

	_ "github.com/go-sql-driver/mysql"

	"parts-engine/internal/config"
)

// NewMySQL opens the optional TecDoc MySQL connection. Returns nil (not an
// error) when the operator has not configured a MySQL host — the TecDoc path
// then stays disabled and the app falls back to Postgres + SQLite cache. This
// keeps local dev + CI runs zero-config while allowing production to enrich
// with the "big data" (articles, articlecrosses, oem_number,
// articlesvehicletrees, articlecriteria) held on the MySQL server.
//
// The connection is non-fatal. Any failure (host down, wrong credentials,
// network blocked) is logged and returns nil so the rest of the server can
// still boot.
func NewMySQL(cfg *config.Config) *sql.DB {
	if !cfg.MySQLEnabled() {
		log.Println("→ MySQL/TecDoc disabled (MYSQL_HOST not set); using Postgres + SQLite cache only")
		return nil
	}

	db, err := sql.Open("mysql", cfg.MySQLDSN())
	if err != nil {
		log.Printf("⚠ MySQL open error (non-fatal, TecDoc disabled): %v", err)
		return nil
	}

	// Modest connection pool — TecDoc queries are heavy (JOINs across
	// 651M-row tables). Give each connection a real budget but cap the
	// pool so a spike doesn't starve the shared MySQL server.
	db.SetMaxOpenConns(20)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)
	db.SetConnMaxIdleTime(3 * time.Minute)

	if err := db.Ping(); err != nil {
		log.Printf("⚠ MySQL unreachable at %s:%s (non-fatal, TecDoc disabled): %v",
			cfg.MySQLHost, cfg.MySQLPort, err)
		_ = db.Close()
		return nil
	}

	fmt.Printf("✓ MySQL/TecDoc connected: %s:%s/%s\n", cfg.MySQLHost, cfg.MySQLPort, cfg.MySQLDB)
	return db
}
