package service

import (
	"context"
	"database/sql"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

// setupOEMCacheTestDB mirrors the Postgres schema of migration 000012
// against in-memory SQLite. Only the columns the service actually reads/
// writes are modeled — full Postgres integration is covered by the manual
// QA in docs/reports/2026-08-17-manual-qa-search.md after deployment.
//
// Note: SQLite doesn't have Postgres JSONB — we use TEXT for corroborations.
// The service uses jsonb concatenation in Postgres; the sqlite tests exercise
// only the read + first-insert paths.
func setupOEMCacheTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	stmts := []string{
		`CREATE TABLE oem_resolution_cache (
			oem_normalized        TEXT PRIMARY KEY,
			oem_raw               TEXT NOT NULL,
			description           TEXT NOT NULL,
			category              TEXT,
			make                  TEXT,
			model                 TEXT,
			year_start            INTEGER,
			year_end              INTEGER,
			confidence            REAL NOT NULL,
			source                TEXT NOT NULL,
			source_url            TEXT,
			corroborating_sources INTEGER NOT NULL DEFAULT 1,
			corroborations        TEXT NOT NULL DEFAULT '[]',
			verified_by_user      INTEGER NOT NULL DEFAULT 0,
			downgrade_count       INTEGER NOT NULL DEFAULT 0,
			first_seen_at         DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			last_seen_at          DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			last_verified_at      DATETIME
		)`,
	}
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			t.Fatalf("seed sqlite: %v", err)
		}
	}
	return db
}

func TestOEMCache_LookupMissReturnsNil(t *testing.T) {
	db := setupOEMCacheTestDB(t)
	c := NewOEMCache(db)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	r, err := c.Lookup(ctx, "82460-2T010")
	if err != nil {
		t.Fatalf("Lookup err: %v", err)
	}
	if r != nil {
		t.Errorf("Lookup on empty cache should return nil, got %+v", r)
	}
}

// insertRow bypasses StoreResult's Postgres-only jsonb logic and inserts
// a raw row so we can test Lookup semantics on sqlite.
func insertRow(t *testing.T, db *sql.DB, oemNorm, oemRaw, desc, source string, confidence float64, corroborating, downgrade int, verifiedByUser bool) {
	t.Helper()
	verified := 0
	if verifiedByUser {
		verified = 1
	}
	_, err := db.Exec(`INSERT INTO oem_resolution_cache
		(oem_normalized, oem_raw, description, category, make, confidence, source, corroborating_sources, downgrade_count, verified_by_user)
		VALUES (?, ?, ?, 'Body / Power Window Motor - Front', 'Kia', ?, ?, ?, ?, ?)`,
		oemNorm, oemRaw, desc, confidence, source, corroborating, downgrade, verified)
	if err != nil {
		t.Fatalf("insert: %v", err)
	}
}

func TestOEMCache_LookupHit(t *testing.T) {
	db := setupOEMCacheTestDB(t)
	c := NewOEMCache(db)
	// NormalizeOEM lowercases — cache rows are stored with lowercased normalized form.
	insertRow(t, db, "824602t010", "82460-2T010", "Front Power Window Motor for Kia Optima TF (2010-2015)", "dealer_lookup", 0.85, 1, 0, false)

	r, err := c.Lookup(context.Background(), "82460-2T010")
	if err != nil {
		t.Fatalf("Lookup err: %v", err)
	}
	if r == nil {
		t.Fatal("Lookup should have returned a result")
	}
	if !contains(r.Description, "Front Power Window Motor") {
		t.Errorf("Description=%q", r.Description)
	}
	if r.SourceStrategy != "oem_cache" {
		t.Errorf("SourceStrategy=%q, want oem_cache", r.SourceStrategy)
	}
	if r.Confidence != 0.85 {
		t.Errorf("Confidence=%v, want 0.85", r.Confidence)
	}
}

func TestOEMCache_LookupSkipsDowngradedRow(t *testing.T) {
	db := setupOEMCacheTestDB(t)
	c := NewOEMCache(db)
	// 3+ downgrade flags with no user verification → skip cache
	insertRow(t, db, "824602t010", "82460-2T010", "Wrong Description", "dealer_lookup", 0.85, 1, 3, false)

	r, err := c.Lookup(context.Background(), "82460-2T010")
	if err != nil {
		t.Fatalf("Lookup err: %v", err)
	}
	if r != nil {
		t.Errorf("Downgraded row should be skipped, got %+v", r)
	}
}

func TestOEMCache_LookupHonorsUserVerifiedOverDowngrades(t *testing.T) {
	db := setupOEMCacheTestDB(t)
	c := NewOEMCache(db)
	// Even with 5 downgrades, user_verified beats them
	insertRow(t, db, "824602t010", "82460-2T010", "Power Window Motor", "user", 1.0, 1, 5, true)

	r, err := c.Lookup(context.Background(), "82460-2T010")
	if err != nil {
		t.Fatalf("Lookup err: %v", err)
	}
	if r == nil {
		t.Fatal("User-verified row should override downgrade count")
	}
	if !contains(r.ConfidenceNote, "user-verified") {
		t.Errorf("ConfidenceNote should mention user-verified: got %q", r.ConfidenceNote)
	}
}

func TestOEMCache_LookupCorroborationNote(t *testing.T) {
	db := setupOEMCacheTestDB(t)
	c := NewOEMCache(db)
	// 3 corroborating sources
	insertRow(t, db, "824602t010", "82460-2T010", "Power Window Motor", "dealer_lookup", 0.95, 3, 0, false)

	r, err := c.Lookup(context.Background(), "82460-2T010")
	if err != nil {
		t.Fatalf("Lookup err: %v", err)
	}
	if r == nil {
		t.Fatal("expected hit")
	}
	if !contains(r.ConfidenceNote, "corroborated by 3") {
		t.Errorf("ConfidenceNote should mention corroboration count: got %q", r.ConfidenceNote)
	}
}

func TestOEMCache_LookupNonOEMReturnsNil(t *testing.T) {
	db := setupOEMCacheTestDB(t)
	c := NewOEMCache(db)
	// NormalizeOEM strips to empty for free-text; Lookup should return nil.
	r, err := c.Lookup(context.Background(), "")
	if err != nil {
		t.Fatalf("empty query err: %v", err)
	}
	if r != nil {
		t.Errorf("empty query should return nil")
	}
}

func TestOEMCache_NilServiceSafe(t *testing.T) {
	var c *OEMCache
	if r, err := c.Lookup(context.Background(), "82460-2T010"); r != nil || err != nil {
		t.Errorf("nil cache should return nil,nil got %v,%v", r, err)
	}
	if err := c.StoreResult(context.Background(), "82460-2T010", &SmartResult{}, "test", ""); err != nil {
		t.Errorf("nil cache StoreResult should be no-op: %v", err)
	}
	// Async variant on nil cache — should not panic.
	c.StoreResultAsync("82460-2T010", &SmartResult{}, "test", "")
}

// CacheStrategy discoverability + behaviour
func TestCacheStrategy_Discovery(t *testing.T) {
	s := &SmartSearch{}
	if _, ok := s.strategyForMode("cache").(*CacheStrategy); !ok {
		t.Errorf("strategyForMode('cache') did not return *CacheStrategy")
	}
}

func TestCacheStrategy_NoServiceReturnsNil(t *testing.T) {
	s := &SmartSearch{} // no oemCache
	strategy := &CacheStrategy{search: s}
	results, err := strategy.Search(context.Background(), StrategyRequest{OEM: "82460-2T010", Limit: 10})
	if err != nil || len(results) != 0 {
		t.Errorf("no-cache strategy should return (nil,nil), got %v/%v", results, err)
	}
}

func TestCacheStrategy_ReturnsHitFromCache(t *testing.T) {
	db := setupOEMCacheTestDB(t)
	c := NewOEMCache(db)
	insertRow(t, db, "824602t010", "82460-2T010", "Front Power Window Motor", "dealer_lookup", 0.85, 1, 0, false)

	s := &SmartSearch{oemCache: c}
	strategy := &CacheStrategy{search: s}
	results, err := strategy.Search(context.Background(), StrategyRequest{OEM: "82460-2T010", Limit: 10})
	if err != nil {
		t.Fatalf("Search err: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if !contains(results[0].Description, "Power Window Motor") {
		t.Errorf("wrong description: %q", results[0].Description)
	}
	if results[0].SourceStrategy != "oem_cache" {
		t.Errorf("SourceStrategy=%q, want oem_cache", results[0].SourceStrategy)
	}
}
