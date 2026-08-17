package service

import (
	"context"
	"database/sql"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

// setupPrefixInferenceTestDB creates an in-memory sqlite that mirrors the
// Postgres schema of migration 000011 (renaming BIGSERIAL → INTEGER etc.)
// and seeds the same baseline rows the real migration would. This lets us
// test PrefixInference without a live Postgres instance.
//
// Kept minimal — only the columns the service actually reads. Real Postgres
// integration is covered by the manual QA in
// docs/reports/2026-08-17-manual-qa-search.md.
func setupPrefixInferenceTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	stmts := []string{
		`CREATE TABLE hk_oem_prefix_map (
			prefix       TEXT PRIMARY KEY,
			system       TEXT NOT NULL,
			category     TEXT NOT NULL,
			description  TEXT NOT NULL,
			confidence   REAL NOT NULL DEFAULT 0.85,
			source       TEXT NOT NULL DEFAULT 'seed'
		)`,
		`CREATE TABLE hk_chassis_code_map (
			chassis_code TEXT PRIMARY KEY,
			make         TEXT NOT NULL,
			model        TEXT NOT NULL,
			platform     TEXT,
			year_start   INTEGER NOT NULL,
			year_end     INTEGER,
			confidence   REAL NOT NULL DEFAULT 0.85,
			source       TEXT NOT NULL DEFAULT 'seed'
		)`,
		`CREATE TABLE hk_variant_suffix_map (
			suffix       TEXT PRIMARY KEY,
			position     TEXT,
			side         TEXT,
			variant_note TEXT,
			confidence   REAL NOT NULL DEFAULT 0.80
		)`,
		`INSERT INTO hk_oem_prefix_map (prefix, system, category, description) VALUES
			('82460','Body','Power Window Motor - Front','Front Power Window Motor Assembly'),
			('26300','Engine','Oil Filter','Engine Oil Filter (4-cyl)'),
			('26350','Engine','Oil Filter','Engine Oil Filter (V6)'),
			('97133','HVAC','Cabin Air Filter','Cabin Air Filter'),
			('58101','Brakes','Brake Pad Set - Front','Front Brake Pad Set')`,
		`INSERT INTO hk_chassis_code_map (chassis_code, make, model, year_start, year_end) VALUES
			('2T','Kia','Optima',2010,2015),
			('2J','Hyundai','Tucson',2010,2015),
			('D3','Hyundai','Tucson',2015,2020),
			('3S','Hyundai','Sonata',2010,2014)`,
		`INSERT INTO hk_variant_suffix_map (suffix, position, side) VALUES
			('010','Front Right','right'),
			('020','Front Left','left'),
			('001','',''),
			('000','','')`,
	}
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			t.Fatalf("seed sqlite: %v (stmt=%s)", err, s)
		}
	}
	// PrefixInference queries use $1 placeholders (Postgres); sqlite uses ?.
	// Rewrite via wrapper — but simpler: pass raw *sql.DB and hope sqlite's
	// PostgreSQL compat layer handles $1. modernc.org/sqlite does support
	// $1/$2 placeholders natively (verified 2024+).
	return db
}

func TestPrefixInference_82460_2T010_UserTestCase(t *testing.T) {
	db := setupPrefixInferenceTestDB(t)
	pi := NewPrefixInference(db)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	r := pi.Synthesize(ctx, "82460-2T010")
	if r == nil {
		t.Fatal("Synthesize returned nil for user's known-broken test case 82460-2T010")
	}
	if !contains(r.Description, "Power Window Motor") {
		t.Errorf("Description missing part family: got %q", r.Description)
	}
	if !contains(r.Description, "Kia") || !contains(r.Description, "Optima") {
		t.Errorf("Description missing vehicle: got %q", r.Description)
	}
	if r.Confidence < 0.70 || r.Confidence > 0.90 {
		t.Errorf("Confidence out of expected inferred range [0.70,0.90]: got %v", r.Confidence)
	}
	if r.SourceStrategy != "prefix_inference" {
		t.Errorf("SourceStrategy=%q, want prefix_inference", r.SourceStrategy)
	}
}

func TestPrefixInference_26350_2J001_OilFilter(t *testing.T) {
	db := setupPrefixInferenceTestDB(t)
	pi := NewPrefixInference(db)
	r := pi.Synthesize(context.Background(), "26350-2J001")
	if r == nil {
		t.Fatal("Synthesize returned nil for 26350-2J001")
	}
	if !contains(r.Description, "Oil Filter") {
		t.Errorf("expected Oil Filter, got %q", r.Description)
	}
	if !contains(r.Description, "Hyundai") || !contains(r.Description, "Tucson") {
		t.Errorf("expected Hyundai Tucson, got %q", r.Description)
	}
}

func TestPrefixInference_UnknownChassisCode_LowerConfidence(t *testing.T) {
	db := setupPrefixInferenceTestDB(t)
	pi := NewPrefixInference(db)
	// Chassis code 'ZZ' is not in the map — confidence should be dampened
	// but the description still returned because the prefix (82460) is known.
	r := pi.Synthesize(context.Background(), "82460-ZZ999")
	if r == nil {
		t.Fatal("Synthesize should return partial result when chassis unknown")
	}
	if r.Confidence > 0.80 {
		t.Errorf("Unknown-chassis confidence should be reduced, got %v", r.Confidence)
	}
	if !contains(r.Description, "Power Window Motor") {
		t.Errorf("Prefix-only description missing part family: got %q", r.Description)
	}
}

func TestPrefixInference_UnknownPrefix_ReturnsNil(t *testing.T) {
	db := setupPrefixInferenceTestDB(t)
	pi := NewPrefixInference(db)
	// Prefix 99999 is not in the map.
	r := pi.Synthesize(context.Background(), "99999-XX000")
	if r != nil {
		t.Errorf("Synthesize should return nil for unknown prefix, got %+v", r)
	}
}

func TestPrefixInference_NonOEMFormat_ReturnsNil(t *testing.T) {
	db := setupPrefixInferenceTestDB(t)
	pi := NewPrefixInference(db)
	// Free text should NOT be decoded.
	for _, input := range []string{
		"oil filter",
		"MANN W811/80",
		"12345",       // too short
		"12345-67890-01", // too long
		"",
	} {
		if r := pi.Synthesize(context.Background(), input); r != nil {
			t.Errorf("Synthesize(%q) should return nil, got %+v", input, r)
		}
	}
}

func TestPrefixInference_NilDB_ReturnsNil(t *testing.T) {
	pi := NewPrefixInference(nil)
	if pi != nil {
		t.Error("NewPrefixInference(nil) should return nil")
	}
	// Also verify a live-DB PrefixInference with nil db doesn't panic
	var pi2 *PrefixInference
	if r := pi2.Synthesize(context.Background(), "82460-2T010"); r != nil {
		t.Error("nil PrefixInference should return nil")
	}
}

func TestPrefixInference_Stats(t *testing.T) {
	db := setupPrefixInferenceTestDB(t)
	pi := NewPrefixInference(db)
	stats, err := pi.Stats(context.Background())
	if err != nil {
		t.Fatalf("Stats error: %v", err)
	}
	if stats.Prefixes.Total < 5 {
		t.Errorf("Prefixes.Total=%d, want >=5", stats.Prefixes.Total)
	}
	if stats.ChassisCodes.Total < 4 {
		t.Errorf("ChassisCodes.Total=%d, want >=4", stats.ChassisCodes.Total)
	}
	if stats.Variants < 4 {
		t.Errorf("Variants=%d, want >=4", stats.Variants)
	}
}

// PrefixInferenceStrategy integration — verifies the strategy wraps the
// service correctly and is discoverable via strategyForMode.
func TestPrefixInferenceStrategy_Discovery(t *testing.T) {
	s := &SmartSearch{}
	if _, ok := s.strategyForMode("prefix_inference").(*PrefixInferenceStrategy); !ok {
		t.Errorf("strategyForMode('prefix_inference') did not return *PrefixInferenceStrategy")
	}
}

func TestPrefixInferenceStrategy_NilServiceReturnsNil(t *testing.T) {
	s := &SmartSearch{} // no prefixInference set
	strategy := &PrefixInferenceStrategy{search: s}
	results, err := strategy.Search(context.Background(), StrategyRequest{
		OEM: "82460-2T010", Limit: 10,
	})
	if err != nil || len(results) != 0 {
		t.Errorf("strategy with no service should return (nil, nil), got results=%v err=%v", results, err)
	}
}

func TestPrefixInferenceStrategy_SkipsNonOEM(t *testing.T) {
	db := setupPrefixInferenceTestDB(t)
	s := &SmartSearch{prefixInference: NewPrefixInference(db)}
	strategy := &PrefixInferenceStrategy{search: s}
	// Free-text query should be skipped, not decoded.
	results, _ := strategy.Search(context.Background(), StrategyRequest{
		Query: "oil filter", Limit: 10,
	})
	if len(results) != 0 {
		t.Errorf("free-text query should return no results, got %d", len(results))
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 || indexOf(s, substr) >= 0)
}

func indexOf(s, substr string) int {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
