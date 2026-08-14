package db

import (
	"database/sql"
	"fmt"
	"log"
	"time"

	_ "github.com/go-sql-driver/mysql"

	"parts-engine/internal/config"
)

// NewMySQL connects to MySQL. Returns nil if the connection fails (non-fatal).
func NewMySQL(cfg *config.Config) *sql.DB {
	db, err := sql.Open("mysql", cfg.MySQLDSN())
	if err != nil {
		log.Printf("⚠ MySQL open error (non-fatal): %v", err)
		return nil
	}
	db.SetMaxOpenConns(20)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)
	db.SetConnMaxIdleTime(3 * time.Minute)

	if err := db.Ping(); err != nil {
		log.Printf("⚠ MySQL unreachable (non-fatal): %v", err)
		db.Close()
		return nil
	}
	fmt.Println("✓ MySQL connected:", cfg.MySQLHost)
	return db
}
