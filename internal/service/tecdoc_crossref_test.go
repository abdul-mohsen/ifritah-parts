package service

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"parts-engine/internal/model"
)

// stubCrossRefRepo is a table-driven stub for TecDocCrossRef tests.
// It records the arguments the service passed through so tests can assert
// on both the return shape and the query wiring.
type stubCrossRefRepo struct {
	rows       []crossRefRow
	err        error
	lastClean  string
	lastLimit  int
	callCount  int
}

func (s *stubCrossRefRepo) QueryCrossRefs(_ context.Context, cleanOEM string, limit int) ([]crossRefRow, error) {
	s.callCount++
	s.lastClean = cleanOEM
	s.lastLimit = limit
	if s.err != nil {
		return nil, s.err
	}
	return s.rows, nil
}

// QueryCrossRefsBatch is the batched form for S2-T4. Stub distributes the
// same fixture rows across the requested OEMs so tests can assert the batch
// path is wired.
func (s *stubCrossRefRepo) QueryCrossRefsBatch(_ context.Context, cleanOEMs []string, limit int) (map[string][]crossRefRow, error) {
	s.callCount++
	s.lastLimit = limit
	if s.err != nil {
		return nil, s.err
	}
	out := make(map[string][]crossRefRow, len(cleanOEMs))
	for _, oem := range cleanOEMs {
		out[oem] = s.rows
	}
	return out, nil
}

func TestTecDocCrossRefSearchCrossReferences(t *testing.T) {
	repo := &stubCrossRefRepo{
		rows: []crossRefRow{
			{
				RawCrossNumber:          "26300-35503",
				MfrName:                 "Mobis",
				LegacyArticleId:         100001,
				ArticleNumber:           "26300-35503",
				Description:             "FILTER ASSY-ENGINE OIL",
				BrandName:               "Hyundai",
				OriginalOEMManufacturer: "Hyundai",
			},
			{
				RawCrossNumber:          "26300-35503-A",
				MfrName:                 "MANN-FILTER",
				LegacyArticleId:         200002,
				ArticleNumber:           "W 811/80",
				Description:             "Oil Filter",
				BrandName:               "MANN-FILTER",
				OriginalOEMManufacturer: "Hyundai",
			},
		},
	}
	svc := &TecDocCrossRef{repo: repo}

	refs, err := svc.SearchCrossReferences("26300-35503", 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(refs) != 2 {
		t.Fatalf("expected 2 refs, got %d", len(refs))
	}
	if refs[0].ArticleNumber != "26300-35503" || refs[0].Manufacturer != "Hyundai" {
		t.Fatalf("first ref not mapped correctly: %+v", refs[0])
	}
	if refs[0].Normalized == "" {
		t.Fatalf("normalized field not populated")
	}
	if repo.lastClean == "" {
		t.Fatalf("repo did not receive normalized OEM")
	}
	if repo.lastLimit != 10 {
		t.Fatalf("limit not passed through, got %d", repo.lastLimit)
	}
}

func TestTecDocCrossRefDeduplicatesByArticleId(t *testing.T) {
	repo := &stubCrossRefRepo{
		rows: []crossRefRow{
			{LegacyArticleId: 42, ArticleNumber: "A", RawCrossNumber: "OEM-1"},
			{LegacyArticleId: 42, ArticleNumber: "A", RawCrossNumber: "OEM-1-dup"},
			{LegacyArticleId: 43, ArticleNumber: "B", RawCrossNumber: "OEM-2"},
			{LegacyArticleId: 0, ArticleNumber: "", RawCrossNumber: "OEM-orphan"},
			{LegacyArticleId: 0, ArticleNumber: "", RawCrossNumber: "OEM-orphan2"},
		},
	}
	svc := &TecDocCrossRef{repo: repo}

	refs, err := svc.SearchCrossReferences("42", 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// 42 (once) + 43 + 2 orphans = 4
	if len(refs) != 4 {
		t.Fatalf("expected 4 refs after dedup, got %d: %+v", len(refs), refs)
	}
}

func TestTecDocCrossRefEmptyOEM(t *testing.T) {
	svc := &TecDocCrossRef{repo: &stubCrossRefRepo{}}
	if _, err := svc.SearchCrossReferences("", 10); err == nil {
		t.Fatalf("expected error for empty OEM, got nil")
	}
	if _, err := svc.SearchCrossReferences("   ", 10); err == nil {
		t.Fatalf("expected error for whitespace OEM, got nil")
	}
	// A string that normalizes to empty (all punctuation) also errors.
	if _, err := svc.SearchCrossReferences("---", 10); err == nil {
		t.Fatalf("expected error for OEM that normalizes to empty, got nil")
	}
}

func TestTecDocCrossRefRepoError(t *testing.T) {
	svc := &TecDocCrossRef{repo: &stubCrossRefRepo{err: errors.New("boom")}}
	if _, err := svc.SearchCrossReferences("26300-35503", 10); err == nil {
		t.Fatalf("expected repo error to surface")
	}
}

func TestTecDocCrossRefNilRepo(t *testing.T) {
	svc := &TecDocCrossRef{}
	if _, err := svc.SearchCrossReferences("X", 10); err == nil {
		t.Fatalf("expected 'database not connected' error")
	}
}

func TestTecDocCrossRefLimitClamp(t *testing.T) {
	repo := &stubCrossRefRepo{}
	svc := &TecDocCrossRef{repo: repo}
	_, _ = svc.SearchCrossReferences("26300-35503", 0)
	if repo.lastLimit != 30 {
		t.Fatalf("expected zero limit to clamp to 30, got %d", repo.lastLimit)
	}
	_, _ = svc.SearchCrossReferences("26300-35503", 999)
	if repo.lastLimit != 30 {
		t.Fatalf("expected out-of-range limit to clamp to 30, got %d", repo.lastLimit)
	}
	_, _ = svc.SearchCrossReferences("26300-35503", 5)
	if repo.lastLimit != 5 {
		t.Fatalf("expected in-range limit to pass through, got %d", repo.lastLimit)
	}
}

func TestTecDocCrossRefManufacturerFallback(t *testing.T) {
	repo := &stubCrossRefRepo{
		rows: []crossRefRow{
			{LegacyArticleId: 1, MfrName: "Mobis", OriginalOEMManufacturer: ""},
			{LegacyArticleId: 2, MfrName: "", OriginalOEMManufacturer: "Kia"},
			{LegacyArticleId: 3, MfrName: "Bosch", OriginalOEMManufacturer: "Hyundai"},
		},
	}
	svc := &TecDocCrossRef{repo: repo}
	refs, _ := svc.SearchCrossReferences("X-1", 10)
	if len(refs) != 3 {
		t.Fatalf("expected 3 refs, got %d", len(refs))
	}
	if refs[0].Manufacturer != "Mobis" {
		t.Fatalf("expected Mobis fallback, got %q", refs[0].Manufacturer)
	}
	if refs[1].Manufacturer != "Kia" {
		t.Fatalf("expected Kia (originalOem), got %q", refs[1].Manufacturer)
	}
	if refs[2].Manufacturer != "Hyundai" {
		t.Fatalf("expected Hyundai (originalOem wins over mfr), got %q", refs[2].Manufacturer)
	}
}

func TestTecDocCrossRefNilDBConstructor(t *testing.T) {
	svc := NewTecDocCrossRef(nil)
	if svc == nil {
		t.Fatalf("NewTecDocCrossRef(nil) must not return nil")
	}
	if _, err := svc.SearchCrossReferences("X", 5); err == nil {
		t.Fatalf("expected 'database not connected' when constructed with nil DB")
	}
}

// TestTecDocCrossRefBatch_SingleCallForManyOEMs verifies the S2-T4 batching:
// N OEM numbers go through ONE call to QueryCrossRefsBatch (not N per-OEM calls).
func TestTecDocCrossRefBatch_SingleCallForManyOEMs(t *testing.T) {
	repo := &stubCrossRefRepo{
		rows: []crossRefRow{{
			RawCrossNumber: "MANN W712/4",
			LegacyArticleId: 999,
			BrandName:      "MANN",
		}},
	}
	svc := &TecDocCrossRef{repo: repo}
	out, err := svc.SearchCrossReferencesBatch([]string{"26300-35505", "26300-35530", "97133-D3000"}, 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.callCount != 1 {
		t.Errorf("expected exactly 1 batch call for 3 OEMs, got %d", repo.callCount)
	}
	// Should have entries for each normalised OEM
	if len(out) != 3 {
		t.Errorf("expected 3 keys in output map, got %d", len(out))
	}
	// Each key should be the normalised form
	if _, ok := out["2630035505"]; !ok {
		t.Errorf("missing normalised key '2630035505'; got keys: %v", keysOf(out))
	}
}

func TestTecDocCrossRefBatch_DedupesInput(t *testing.T) {
	repo := &stubCrossRefRepo{rows: []crossRefRow{}}
	svc := &TecDocCrossRef{repo: repo}
	// Same OEM 5 times — should be deduped to 1 lookup
	_, _ = svc.SearchCrossReferencesBatch([]string{"X-1", "X-1", "X-1", "X-1", "X-1"}, 5)
	if repo.callCount != 1 {
		t.Errorf("expected 1 call after dedup, got %d", repo.callCount)
	}
}

func TestTecDocCrossRefBatch_EmptyInput(t *testing.T) {
	svc := &TecDocCrossRef{repo: &stubCrossRefRepo{}}
	out, err := svc.SearchCrossReferencesBatch(nil, 5)
	if err != nil {
		t.Errorf("empty input err=%v, want nil", err)
	}
	if len(out) != 0 {
		t.Errorf("empty input returned %d entries, want 0", len(out))
	}
}

func keysOf(m map[string][]model.OEMReference) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	return ks
}


// TestCrossRefSQL_NoBrokenColumnReference guards against BUG-A regression:
// the TecDoc 2020 schema does NOT have `ac.articleCrossNumber` NOR
// `ac.cleanCrossNumber` NOR `ac.mfrName` — the actual columns are
// `ac.oemNumber`, `ac.number`, `ac.brandName`, `ac.mfrId`, `ac.legacyArticleId`.
// Using any of the wrong names surfaced as:
//
//	Error 1054 (42S22): Unknown column 'ac.XXXNumber' in 'field list'
//
// and silently broke cross_reference mode for every query.
//
// This test inspects the raw SQL source file to ensure the broken column
// names are never reintroduced. It's a cheap static check — full
// integration is covered by the /api/debug/logs manual QA in
// docs/reports/2026-08-17-manual-qa-search.md.
func TestCrossRefSQL_NoBrokenColumnReference(t *testing.T) {
	sourcePath := "tecdoc_crossref.go"
	content, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatalf("read source: %v", err)
	}
	src := string(content)

	// The literal SQL fragments must not appear in the query strings.
	// Doc comments mentioning the historical bug ARE allowed (they explain
	// the fix). Grep line-by-line and skip lines starting with `//`.
	forbidden := []string{
		"ac.articleCrossNumber", // was in v1 of the fix, broke CI
		"ac.cleanCrossNumber",   // was in v2 of the fix, broke qa deploy
		"ac.mfrName",            // wrong column — TecDoc 2020 uses ac.mfrId + join manufacturers
	}
	lines := strings.Split(src, "\n")
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "//") {
			continue
		}
		for _, bad := range forbidden {
			if strings.Contains(line, bad) {
				t.Errorf("tecdoc_crossref.go:%d — column %q does not exist in TecDoc 2020 (see BUG FIX comment in QueryCrossRefs)", i+1, bad)
			}
		}
	}
}


// TestCrossRef_UsesIndexedColumn verifies both queries hit the indexed
// generated column oemNumberNormalized. The migration
// sql/06_articlecrosses_normalized_oem_index.sql creates the column;
// this test guards against a regression that reverts to the LOWER(REPLACE(...))
// full-scan pattern (which caused 3-8 HOUR queries on qa.ifritah.com per
// docs/reports/2026-08-19-post-pr14-data-quality.md).
func TestCrossRef_UsesIndexedColumn(t *testing.T) {
	content, err := os.ReadFile("tecdoc_crossref.go")
	if err != nil {
		t.Fatalf("read source: %v", err)
	}
	src := string(content)

	// Must reference the indexed column in both single + batch queries.
	if !strings.Contains(src, "ac.oemNumberNormalized = ?") {
		t.Error("tecdoc_crossref.go must use WHERE ac.oemNumberNormalized = ? (indexed generated column). See sql/06_articlecrosses_normalized_oem_index.sql.")
	}
	if !strings.Contains(src, "ac.oemNumberNormalized IN (") {
		t.Error("tecdoc_crossref.go batch query must use ac.oemNumberNormalized IN (...) — the indexed column.")
	}

	// Must NOT re-introduce the function-on-column slow pattern in the SQL text
	// (comments + documentation are allowed to reference it as historical context).
	lines := strings.Split(src, "\n")
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "//") {
			continue
		}
		if strings.Contains(line, "LOWER(REPLACE(REPLACE(REPLACE(REPLACE(ac.oemNumber") {
			t.Errorf("tecdoc_crossref.go:%d - slow function-on-column pattern reintroduced; use ac.oemNumberNormalized (indexed) instead. See migration sql/06_articlecrosses_normalized_oem_index.sql.", i+1)
		}
	}
}

// TestCrossRef_SchemaMigrationReferenced verifies the SQL migration file
// exists and the Go source references it in a comment. Guards against
// the pair drifting out of sync.
func TestCrossRef_SchemaMigrationReferenced(t *testing.T) {
	migrationPath := "../../sql/06_articlecrosses_normalized_oem_index.sql"
	if _, err := os.Stat(migrationPath); err != nil {
		t.Fatalf("MySQL migration file %s missing: %v", migrationPath, err)
	}
	content, err := os.ReadFile("tecdoc_crossref.go")
	if err != nil {
		t.Fatalf("read source: %v", err)
	}
	if !strings.Contains(string(content), "06_articlecrosses_normalized_oem_index.sql") {
		t.Error("tecdoc_crossref.go should reference sql/06_articlecrosses_normalized_oem_index.sql so operators can find the migration from the codebase")
	}
}
