// Package db — app-managed Postgres migration runner.
//
// Runs at every server start. Idempotent + safe against double-runs across
// concurrent replicas via Postgres advisory locks. Replaces the previous
// docker-entrypoint-initdb.d/ mechanism which only fired on first-ever boot
// and couldn't apply migrations added after the initial deploy.
//
// Design:
//
//	1. Take a Postgres advisory lock (fixed key). Only one process can
//	   run migrations at a time — safe against docker-compose bringing up
//	   several app replicas simultaneously.
//	2. CREATE TABLE IF NOT EXISTS schema_migrations. First-time bootstrap.
//	3. SELECT already-applied versions into an in-memory set.
//	4. Read the embedded migration files (sorted by filename), skipping any
//	   version already in the set.
//	5. For each new migration: BEGIN TX, execute file, INSERT into
//	   schema_migrations, COMMIT. Failure rolls back the file's changes
//	   and aborts the run.
//	6. Release the advisory lock.
//
// Grandfathering: on an existing deployment where migrations 000001-000010
// were already applied via the postgres initdb hook, this runner will:
//   - Notice schema_migrations is empty
//   - Try to re-run 000001…000010 — SAFE because every existing migration
//     uses CREATE TABLE IF NOT EXISTS
//   - Insert the version rows so subsequent boots skip them
//
// Advisory lock key: hash of the string "parts-engine.schema-migrations" —
// stable across runs, unique enough not to collide with app-level locks.
package db

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"log"
	"sort"
	"strings"
	"time"
)

// MigrationsFS should be pointed at db/migrations/*.sql via //go:embed
// in the caller (main.go). We accept an fs.FS so tests can inject a
// scratch filesystem.
type MigrationsFS = fs.FS

// Advisory lock key. Any 64-bit integer works — we pick a distinctive one
// so it's easy to spot in pg_locks during ops. Value chosen to match the
// mnemonic "parts engine migrator" in a way that doesn't collide with
// obvious sequences.
const migrationLockKey int64 = 7412_0801_0817_1234

// RunMigrations applies every un-applied migration in migrationsFS to db.
// migrationsSubdir is the path inside migrationsFS to look under (e.g.
// "db/migrations"). Files must be named NNNNNN_description.sql — sorted
// lexicographically to determine execution order.
//
// Returns nil on success (including the no-work case). Returns an error
// if any migration file fails to execute; the failed migration is rolled
// back but successfully-applied earlier migrations remain applied.
func RunMigrations(db *sql.DB, migrationsFS MigrationsFS, migrationsSubdir string) error {
	if db == nil {
		return fmt.Errorf("migrator: db is nil")
	}

	// Take advisory lock — blocks until we get it, released on Close().
	// pg_advisory_lock is transaction-independent and connection-scoped,
	// so we borrow a dedicated connection for the whole migration pass.
	conn, err := db.Conn(context.Background())
	if err != nil {
		return fmt.Errorf("migrator: acquire connection: %w", err)
	}
	defer conn.Close()

	if _, err := conn.ExecContext(context.Background(), "SELECT pg_advisory_lock($1)", migrationLockKey); err != nil {
		return fmt.Errorf("migrator: acquire advisory lock: %w", err)
	}
	defer func() {
		if _, err := conn.ExecContext(context.Background(), "SELECT pg_advisory_unlock($1)", migrationLockKey); err != nil {
			log.Printf("[migrator] WARN release advisory lock: %v", err)
		}
	}()

	// Ensure the tracking table exists. Some migration files might INCLUDE
	// this same DDL (e.g. 000013) — CREATE TABLE IF NOT EXISTS makes both
	// paths harmless.
	if _, err := conn.ExecContext(context.Background(), `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version      TEXT PRIMARY KEY,
			applied_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			applied_by   TEXT NOT NULL DEFAULT 'app_migrator'
		)`); err != nil {
		return fmt.Errorf("migrator: bootstrap schema_migrations: %w", err)
	}

	// Applied set.
	applied := map[string]bool{}
	rows, err := conn.QueryContext(context.Background(), "SELECT version FROM schema_migrations")
	if err != nil {
		return fmt.Errorf("migrator: query applied set: %w", err)
	}
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			rows.Close()
			return fmt.Errorf("migrator: scan applied version: %w", err)
		}
		applied[v] = true
	}
	rows.Close()

	// List migration files.
	entries, err := fs.ReadDir(migrationsFS, migrationsSubdir)
	if err != nil {
		return fmt.Errorf("migrator: read %s: %w", migrationsSubdir, err)
	}
	var files []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		files = append(files, e.Name())
	}
	sort.Strings(files) // lexicographic — filename prefix (000012, 000013, …) drives order

	newlyApplied := 0
	for _, f := range files {
		version := strings.TrimSuffix(f, ".sql")
		if applied[version] {
			continue
		}
		content, err := fs.ReadFile(migrationsFS, migrationsSubdir+"/"+f)
		if err != nil {
			return fmt.Errorf("migrator: read %s: %w", f, err)
		}

		start := time.Now()
		tx, err := conn.BeginTx(context.Background(), nil)
		if err != nil {
			return fmt.Errorf("migrator: begin tx for %s: %w", f, err)
		}
		if _, err := tx.ExecContext(context.Background(), string(content)); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("migrator: apply %s: %w", f, err)
		}
		if _, err := tx.ExecContext(context.Background(),
			`INSERT INTO schema_migrations (version) VALUES ($1) ON CONFLICT DO NOTHING`,
			version); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("migrator: record %s: %w", f, err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("migrator: commit %s: %w", f, err)
		}
		log.Printf("[migrator] applied %s in %v", version, time.Since(start))
		newlyApplied++
	}

	if newlyApplied == 0 {
		log.Printf("[migrator] schema up-to-date (%d migrations already applied)", len(applied))
	} else {
		log.Printf("[migrator] applied %d new migration(s), %d already applied", newlyApplied, len(applied))
	}
	return nil
}

// MigrationStatus reports on the state of the migration tracker — used by
// the diagnostics endpoint to expose "which migrations have run" for ops.
type MigrationStatus struct {
	Version   string    `json:"version"`
	AppliedAt time.Time `json:"appliedAt"`
	AppliedBy string    `json:"appliedBy"`
}

func ListAppliedMigrations(db *sql.DB) ([]MigrationStatus, error) {
	if db == nil {
		return nil, nil
	}
	rows, err := db.Query(`SELECT version, applied_at, applied_by FROM schema_migrations ORDER BY version`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []MigrationStatus
	for rows.Next() {
		var s MigrationStatus
		if err := rows.Scan(&s.Version, &s.AppliedAt, &s.AppliedBy); err != nil {
			continue
		}
		out = append(out, s)
	}
	return out, nil
}

// MigrationsFSFromEmbed is a small helper so main.go can pass its embedded FS
// without callers needing to import embed. Not strictly required.
func MigrationsFSFromEmbed(embedded embed.FS) MigrationsFS { return embedded }
