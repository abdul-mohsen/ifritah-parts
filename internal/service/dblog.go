package service

import (
	"context"
	"database/sql"
	"log"
	"strings"
	"time"
)

// logQuery wraps db.Query with timing and slow-query detection.
func logQuery(db *sql.DB, label, query string, args ...interface{}) (*sql.Rows, error) {
	start := time.Now()
	rows, err := db.Query(query, args...)
	dur := time.Since(start)
	trimmed := strings.Join(strings.Fields(strings.TrimSpace(query)), " ")
	if len(trimmed) > 150 {
		trimmed = trimmed[:150] + "..."
	}
	if err != nil {
		log.Printf("[SQL ERROR] %s: %v err=%v — %s", label, dur, err, trimmed)
	} else if dur > 500*time.Millisecond {
		log.Printf("[SQL SLOW ⚠⚠] %s: %v — %s", label, dur, trimmed)
	} else if dur > 100*time.Millisecond {
		log.Printf("[SQL SLOW ⚠] %s: %v — %s", label, dur, trimmed)
	} else {
		log.Printf("[SQL] %s: %v — %s", label, dur, trimmed)
	}
	return rows, err
}

// logQueryCtx wraps db.QueryContext with timing and slow-query detection.
func logQueryCtx(db *sql.DB, ctx context.Context, label, query string, args ...interface{}) (*sql.Rows, error) {
	start := time.Now()
	rows, err := db.QueryContext(ctx, query, args...)
	dur := time.Since(start)
	trimmed := strings.Join(strings.Fields(strings.TrimSpace(query)), " ")
	if len(trimmed) > 150 {
		trimmed = trimmed[:150] + "..."
	}
	if err != nil {
		log.Printf("[SQL ERROR] %s: %v err=%v — %s", label, dur, err, trimmed)
	} else if dur > 500*time.Millisecond {
		log.Printf("[SQL SLOW ⚠⚠] %s: %v — %s", label, dur, trimmed)
	} else if dur > 100*time.Millisecond {
		log.Printf("[SQL SLOW ⚠] %s: %v — %s", label, dur, trimmed)
	} else {
		log.Printf("[SQL] %s: %v — %s", label, dur, trimmed)
	}
	return rows, err
}

// logQueryRow wraps db.QueryRow with timing and slow-query detection.
func logQueryRow(db *sql.DB, label, query string, args ...interface{}) *timedRow {
	start := time.Now()
	row := db.QueryRow(query, args...)
	trimmed := strings.Join(strings.Fields(strings.TrimSpace(query)), " ")
	if len(trimmed) > 150 {
		trimmed = trimmed[:150] + "..."
	}
	return &timedRow{row: row, start: start, label: label, sql: trimmed}
}

type timedRow struct {
	row   *sql.Row
	start time.Time
	label string
	sql   string
}

func (t *timedRow) Scan(dest ...interface{}) error {
	err := t.row.Scan(dest...)
	dur := time.Since(t.start)
	if err != nil && err != sql.ErrNoRows {
		log.Printf("[SQL ERROR] %s: %v err=%v — %s", t.label, dur, err, t.sql)
	} else if dur > 500*time.Millisecond {
		log.Printf("[SQL SLOW ⚠⚠] %s: %v — %s", t.label, dur, t.sql)
	} else if dur > 100*time.Millisecond {
		log.Printf("[SQL SLOW ⚠] %s: %v — %s", t.label, dur, t.sql)
	} else {
		log.Printf("[SQL] %s: %v — %s", t.label, dur, t.sql)
	}
	return err
}
