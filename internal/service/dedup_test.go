package service

import (
	"testing"

	"parts-engine/internal/model"
)

// TestDedupeByArticleId_RemovesDuplicates covers S0-T2: BUG-2/7/8/12 where the
// same legacyArticleId appeared twice in results after being pulled through
// two independent strategies (e.g. crossref + oem_number).
func TestDedupeByArticleId_RemovesDuplicates(t *testing.T) {
	input := []SmartResult{
		{Part: model.Part{LegacyArticleId: 90390, BrandName: "FISPA", ArticleNumber: "111"}},
		{Part: model.Part{LegacyArticleId: 90390, BrandName: "SIDAT", ArticleNumber: "111"}}, // BUG-7 duplicate
		{Part: model.Part{LegacyArticleId: 452, BrandName: "PRASCO", ArticleNumber: "HYK452"}},
		{Part: model.Part{LegacyArticleId: 452, BrandName: "AVA", ArticleNumber: "HYK452"}}, // BUG-8 duplicate
	}

	got := dedupeByArticleId(input)

	if len(got) != 2 {
		t.Fatalf("expected 2 unique results, got %d", len(got))
	}
	if got[0].LegacyArticleId != 90390 || got[0].BrandName != "FISPA" {
		t.Errorf("expected first-wins for 90390 (FISPA), got %+v", got[0])
	}
	if got[1].LegacyArticleId != 452 || got[1].BrandName != "PRASCO" {
		t.Errorf("expected first-wins for 452 (PRASCO), got %+v", got[1])
	}
}

// TestDedupeByArticleId_EmptyAndSingle covers boundary conditions: nil, empty,
// and single-item slices must be returned unchanged (short-circuit path).
func TestDedupeByArticleId_EmptyAndSingle(t *testing.T) {
	if got := dedupeByArticleId(nil); got != nil {
		t.Errorf("nil input: expected nil, got %v", got)
	}
	empty := []SmartResult{}
	if got := dedupeByArticleId(empty); len(got) != 0 {
		t.Errorf("empty input: expected empty, got %v", got)
	}
	single := []SmartResult{{Part: model.Part{LegacyArticleId: 1}}}
	got := dedupeByArticleId(single)
	if len(got) != 1 || got[0].LegacyArticleId != 1 {
		t.Errorf("single input: expected unchanged, got %v", got)
	}
}

// TestDedupeByArticleId_KeepsZeroIds asserts that entries with LegacyArticleId==0
// (online lookups, dealer scrapes, aftermarket_crossref_only responses) are NOT
// de-duplicated against each other, since they carry no reliable id. This is
// intentional per the S0-T2 design doc — de-duping those would require a
// brand+articleNumber string-normalized compare, out of scope.
func TestDedupeByArticleId_KeepsZeroIds(t *testing.T) {
	input := []SmartResult{
		{Part: model.Part{LegacyArticleId: 0, ArticleNumber: "26300-35505", BrandName: "HYUNDAI/KIA"}},
		{Part: model.Part{LegacyArticleId: 0, ArticleNumber: "26300-35505", BrandName: "MANN"}},
		{Part: model.Part{LegacyArticleId: 100, ArticleNumber: "X", BrandName: "BOSCH"}},
		{Part: model.Part{LegacyArticleId: 100, ArticleNumber: "X", BrandName: "BOSCH"}}, // dup, must be removed
	}
	got := dedupeByArticleId(input)
	if len(got) != 3 {
		t.Fatalf("expected 3 results (2 zero-id kept, 1 dup removed), got %d: %+v", len(got), got)
	}
	if got[0].BrandName != "HYUNDAI/KIA" || got[1].BrandName != "MANN" {
		t.Errorf("expected both zero-id entries preserved in order, got %+v", got[:2])
	}
	if got[2].LegacyArticleId != 100 {
		t.Errorf("expected non-zero id 100 preserved, got %+v", got[2])
	}
}

// TestDedupeByArticleId_FirstWinsPreservesOrder asserts insertion order is
// preserved for the surviving entries (append-safe).
func TestDedupeByArticleId_FirstWinsPreservesOrder(t *testing.T) {
	input := []SmartResult{
		{Part: model.Part{LegacyArticleId: 3}},
		{Part: model.Part{LegacyArticleId: 1}},
		{Part: model.Part{LegacyArticleId: 2}},
		{Part: model.Part{LegacyArticleId: 1}}, // dup
		{Part: model.Part{LegacyArticleId: 3}}, // dup
	}
	got := dedupeByArticleId(input)
	if len(got) != 3 {
		t.Fatalf("expected 3 unique, got %d", len(got))
	}
	expectedOrder := []int{3, 1, 2}
	for i, want := range expectedOrder {
		if got[i].LegacyArticleId != want {
			t.Errorf("position %d: expected id=%d, got id=%d (full: %v)",
				i, want, got[i].LegacyArticleId, got)
		}
	}
}
