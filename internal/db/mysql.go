package db

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"

	"parts-engine/internal/config"
)

// NewMySQL opens the MySQL TecDoc connection.
//
// Contract:
//
//   - MySQL is the PRIMARY data source (TecDoc: articles, articlecrosses,
//     oem_number, articlesvehicletrees, articlecriteria — ~651M linkages).
//     Production MUST connect. If MYSQL_HOST is set and the ping fails, the
//     process EXITS with log.Fatalf so a broken deployment never boots into
//     silent-fallback mode.
//
//   - Local dev / CI escape hatch: set ALLOW_NO_MYSQL=1 (or leave MYSQL_HOST
//     unset). The function then returns nil and the server boots on Postgres +
//     SQLite alone. Every caller of this must handle nil.
//
// Legacy env-var names (DBUSER / PASSWORD / HOST / DBPORT / DBNAME) documented
// in C:\ssda\chatGPT\parts\test_queries.go are read as fallbacks by
// config.Load() — see internal/config/config.go for the resolution order.
func NewMySQL(cfg *config.Config) *sql.DB {
	allowNoMySQL := strings.ToLower(strings.TrimSpace(os.Getenv("ALLOW_NO_MYSQL"))) == "1" ||
		strings.EqualFold(os.Getenv("ALLOW_NO_MYSQL"), "true")

	if !cfg.MySQLEnabled() {
		if allowNoMySQL {
			log.Println("→ MySQL/TecDoc SKIPPED (ALLOW_NO_MYSQL=1 and no MYSQL_HOST); running on Postgres + SQLite only")
			return nil
		}
		log.Fatalf("✗ MySQL required but MYSQL_HOST (or legacy HOST) is not set. " +
			"Set MYSQL_HOST + MYSQL_USER + MYSQL_PASSWORD + MYSQL_DATABASE in .env, " +
			"or set ALLOW_NO_MYSQL=1 to run without the TecDoc data source.")
		return nil // unreachable
	}

	db, err := sql.Open("mysql", cfg.MySQLDSN())
	if err != nil {
		if allowNoMySQL {
			log.Printf("⚠ MySQL open error (ALLOW_NO_MYSQL=1, degrading gracefully): %v", err)
			return nil
		}
		log.Fatalf("✗ MySQL open error: %v (set ALLOW_NO_MYSQL=1 to bypass in dev)", err)
	}

	db.SetMaxOpenConns(20)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)
	db.SetConnMaxIdleTime(3 * time.Minute)

	if err := db.Ping(); err != nil {
		_ = db.Close()
		if allowNoMySQL {
			log.Printf("⚠ MySQL unreachable at %s:%s (ALLOW_NO_MYSQL=1, degrading): %v",
				cfg.MySQLHost, cfg.MySQLPort, err)
			return nil
		}
		log.Fatalf("✗ MySQL unreachable at %s:%s — %v. "+
			"Fix connectivity, or set ALLOW_NO_MYSQL=1 to bypass in dev.",
			cfg.MySQLHost, cfg.MySQLPort, err)
	}

	fmt.Printf("✓ MySQL/TecDoc connected: %s:%s/%s\n", cfg.MySQLHost, cfg.MySQLPort, cfg.MySQLDB)
	return db
}
