// derive_hk_maps — CLI wrapper around service.DeriveWorker.
//
// The server now runs this automatically at startup + monthly (see
// internal/service/derive_worker.go). This CLI still exists so operators
// can:
//
//   - Force an immediate derive without waiting for the server's schedule
//     (useful right after a fresh TecDoc dump lands)
//   - Run the derive on a machine that isn't currently running the app
//   - Verify the derive works before promoting a schema change
//
// Env vars:
//
//	MYSQL_HOST / MYSQL_PORT / MYSQL_USER / MYSQL_PASSWORD / MYSQL_DATABASE
//	   (or TECDOC_DSN as full DSN)
//	POSTGRES_HOST / POSTGRES_PORT / POSTGRES_USER / POSTGRES_PASSWORD /
//	POSTGRES_DB / POSTGRES_SSLMODE
//	   (or POSTGRES_URL as full DSN)
//
// Exit codes:
//
//	0  — derive succeeded (or was skipped because cadence hasn't elapsed
//	     and --force wasn't passed)
//	1  — configuration error (Postgres unreachable, invalid DSN, etc.)
//	2  — derive attempted but failed (TecDoc reachable but query errored)
//
// Passing --force skips the 30-day cadence check.
package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	_ "github.com/go-sql-driver/mysql"
	_ "github.com/jackc/pgx/v5/stdlib"

	"parts-engine/internal/service"
)

func main() {
	force := flag.Bool("force", false, "skip the 30-day cadence check and run derive now")
	flag.Parse()

	mysqlDSN := getenv("TECDOC_DSN", "")
	if mysqlDSN == "" {
		mysqlDSN = fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?charset=utf8mb4&parseTime=true",
			os.Getenv("MYSQL_USER"), os.Getenv("MYSQL_PASSWORD"),
			getenv("MYSQL_HOST", "localhost"), getenv("MYSQL_PORT", "3306"),
			getenv("MYSQL_DATABASE", "tecdoc"))
	}
	pgDSN := getenv("POSTGRES_URL", "")
	if pgDSN == "" {
		pgDSN = fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
			getenv("POSTGRES_HOST", "localhost"), getenv("POSTGRES_PORT", "5432"),
			getenv("POSTGRES_USER", "postgres"), os.Getenv("POSTGRES_PASSWORD"),
			getenv("POSTGRES_DB", "hk_parts"), getenv("POSTGRES_SSLMODE", "disable"))
	}

	mysqlDB, err := sql.Open("mysql", mysqlDSN)
	if err != nil {
		log.Printf("[derive_hk_maps] TecDoc MySQL open error (falling back to seed baseline): %v", err)
		os.Exit(1)
	}
	defer mysqlDB.Close()

	pgDB, err := sql.Open("pgx", pgDSN)
	if err != nil {
		log.Printf("[derive_hk_maps] Postgres open error: %v", err)
		os.Exit(1)
	}
	defer pgDB.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	if err := pgDB.PingContext(ctx); err != nil {
		log.Printf("[derive_hk_maps] Postgres unreachable: %v", err)
		os.Exit(1)
	}

	worker := service.NewDeriveWorker(pgDB, mysqlDB)
	if *force {
		worker.SetForce(true)
	}
	if err := worker.RunOnce(ctx); err != nil {
		log.Printf("[derive_hk_maps] run failed: %v", err)
		os.Exit(2)
	}
	log.Printf("[derive_hk_maps] done")
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
