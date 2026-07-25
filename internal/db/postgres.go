package db

import (
	"database/sql"
	"fmt"
	"log"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	"parts-engine/internal/config"
)

// NewPostgres connects to PostgreSQL. Returns nil if the connection fails.
func NewPostgres(cfg *config.Config) *sql.DB {
	db, err := sql.Open("pgx", cfg.PostgresDSN())
	if err != nil {
		log.Printf("⚠ PostgreSQL open error (non-fatal): %v", err)
		return nil
	}
	db.SetMaxOpenConns(20)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)
	db.SetConnMaxIdleTime(3 * time.Minute)

	if err := db.Ping(); err != nil {
		log.Printf("⚠ PostgreSQL unreachable (non-fatal): %v", err)
		db.Close()
		return nil
	}

	fmt.Println("✓ PostgreSQL connected:", cfg.PostgresHost)
	return db
}
