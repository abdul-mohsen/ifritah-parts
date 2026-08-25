package service

import (
	"context"
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
)

// setupRelatedPartsTestDB creates an in-memory SQLite with the related_parts
// schema (Postgres-compatible), seeds a subset of the migration data, and
// returns the *sql.DB for tests.
func setupRelatedPartsTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("sqlite open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	// SQLite version of the schema — REAL for correlation, INTEGER for priority.
	// TIMESTAMPTZ default omitted; not needed by the queries under test.
	const schema = `
		CREATE TABLE related_parts (
			source_category  TEXT NOT NULL,
			related_category TEXT NOT NULL,
			correlation      REAL NOT NULL,
			evidence_source  TEXT NOT NULL,
			priority         INTEGER NOT NULL DEFAULT 50,
			PRIMARY KEY (source_category, related_category, evidence_source)
		)`
	if _, err := db.Exec(schema); err != nil {
		t.Fatalf("create schema: %v", err)
	}

	// Sample seed - matches the 60k service bundle in the real migration.
	seeds := [][]any{
		{"Oil Filter", "Air Filter", 0.92, "service_60k", 90},
		{"Oil Filter", "Cabin Air Filter", 0.85, "service_60k", 80},
		{"Oil Filter", "Spark Plug & Ignition Coil", 0.60, "service_60k", 70},
		{"Air Filter", "Oil Filter", 0.92, "service_60k", 90},
		{"Front Brake Pad / Disc", "Front Brake Caliper", 0.60, "service_brake", 70},
	}
	for _, s := range seeds {
		if _, err := db.Exec(
			`INSERT INTO related_parts (source_category, related_category, correlation, evidence_source, priority)
			 VALUES (?, ?, ?, ?, ?)`, s...); err != nil {
			t.Fatalf("insert seed: %v", err)
		}
	}
	return db
}

// TestRelatedParts_FindRelatedByOEM_DecodedCategory - OEM decodes to
// "Oil Filter" via prefixMap (263), returns 3 related categories.
func TestRelatedParts_FindRelatedByOEM_DecodedCategory(t *testing.T) {
	db := setupRelatedPartsTestDB(t)

	// Rewrite the SQLite-compat query. The service uses $1/$2 (Postgres).
	// SQLite uses ? but understands $N too when using ?NNN. Keep the test
	// pointed at a wrapper that translates.
	r := &RelatedParts{db: db}

	// The service's query uses Postgres-style placeholders; sqlite3 accepts
	// $1/$2 as bind parameters so no translation is needed.
	got, err := r.FindRelatedByOEM(context.Background(), "26350-2J001", 5)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(got) != 3 {
		t.Errorf("len(got) = %d, want 3 (Air Filter, Cabin Air Filter, Spark Plug)", len(got))
	}
	// Priority-sorted DESC — Air Filter (90) first, Cabin Air Filter (80),
	// Spark Plug & Ignition Coil (70).
	wantOrder := []string{"Air Filter", "Cabin Air Filter", "Spark Plug & Ignition Coil"}
	for i, want := range wantOrder {
		if i >= len(got) {
			break
		}
		if got[i].Category != want {
			t.Errorf("got[%d] = %q, want %q", i, got[i].Category, want)
		}
	}
}

// TestRelatedParts_FindRelatedByOEM_UnknownPrefix - OEM that doesn't
// decode returns nil, nil (no error, no results).
func TestRelatedParts_FindRelatedByOEM_UnknownPrefix(t *testing.T) {
	db := setupRelatedPartsTestDB(t)
	r := &RelatedParts{db: db}

	got, err := r.FindRelatedByOEM(context.Background(), "XYZ", 5)
	if err != nil {
		t.Errorf("unexpected err: %v", err)
	}
	if got != nil {
		t.Errorf("got = %v, want nil for unknown prefix", got)
	}
}

// TestRelatedParts_FindRelatedByCategory - direct category lookup.
func TestRelatedParts_FindRelatedByCategory(t *testing.T) {
	db := setupRelatedPartsTestDB(t)
	r := &RelatedParts{db: db}

	got, err := r.FindRelatedByCategory(context.Background(), "Front Brake Pad / Disc", 5)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(got) != 1 {
		t.Errorf("len(got) = %d, want 1 (Front Brake Caliper)", len(got))
	}
	if len(got) > 0 && got[0].Category != "Front Brake Caliper" {
		t.Errorf("got[0] = %q, want 'Front Brake Caliper'", got[0].Category)
	}
	if len(got) > 0 && got[0].Evidence != "service_brake" {
		t.Errorf("got[0].Evidence = %q, want 'service_brake'", got[0].Evidence)
	}
}

// TestRelatedParts_FindRelatedByCategory_LimitClamp - limits outside
// [1, 20] get clamped to defaults.
func TestRelatedParts_FindRelatedByCategory_LimitClamp(t *testing.T) {
	db := setupRelatedPartsTestDB(t)
	r := &RelatedParts{db: db}

	// limit = 0 -> clamps to 5 (default)
	got, err := r.FindRelatedByCategory(context.Background(), "Oil Filter", 0)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(got) != 3 {
		t.Errorf("len(got) = %d, want 3 (limit 0 -> default 5, only 3 rows exist)", len(got))
	}

	// limit = 1000 -> clamps to 5
	got, err = r.FindRelatedByCategory(context.Background(), "Oil Filter", 1000)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(got) != 3 {
		t.Errorf("len(got) = %d, want 3 (limit 1000 -> default 5, only 3 rows exist)", len(got))
	}
}

// TestRelatedParts_NilDB - nil DB shouldn't panic.
func TestRelatedParts_NilDB(t *testing.T) {
	r := &RelatedParts{db: nil}
	got, err := r.FindRelatedByOEM(context.Background(), "26350-2J001", 5)
	if err != nil {
		t.Errorf("nil db shouldn't return err, got %v", err)
	}
	if got != nil {
		t.Errorf("nil db should return nil, got %v", got)
	}
}

// TestRelatedParts_EmptyCategory
func TestRelatedParts_EmptyCategory(t *testing.T) {
	db := setupRelatedPartsTestDB(t)
	r := &RelatedParts{db: db}

	got, err := r.FindRelatedByCategory(context.Background(), "", 5)
	if err != nil {
		t.Errorf("empty category shouldn't error, got %v", err)
	}
	if got != nil {
		t.Errorf("empty category should return nil, got %v", got)
	}

	got, err = r.FindRelatedByCategory(context.Background(), "   ", 5)
	if err != nil {
		t.Errorf("whitespace category shouldn't error, got %v", err)
	}
	if got != nil {
		t.Errorf("whitespace category should return nil, got %v", got)
	}
}
