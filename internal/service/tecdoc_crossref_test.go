package service

import (
	"context"
	"errors"
	"testing"
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
