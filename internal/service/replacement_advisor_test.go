package service

import (
	"testing"

	"parts-engine/internal/model"
)

type stubReplacementOEMFinder struct {
	results []model.OEMReference
}

func (s stubReplacementOEMFinder) FindByOEM(_ string, _ int) ([]model.OEMReference, error) {
	return s.results, nil
}

type stubReplacementPartFinder struct {
	parts map[int]*model.Part
}

func (s stubReplacementPartFinder) FindByArticle(id int, _ int) (*model.Part, error) {
	return s.parts[id], nil
}

type stubReplacementAlternativesFinder struct {
	alts []AlternativePart
}

func (s stubReplacementAlternativesFinder) FindForArticle(_ int, _ int, _ int) ([]AlternativePart, error) {
	return s.alts, nil
}

func TestReplacementAdvisorPrioritizesSharedOEMOverCatalogAlternatives(t *testing.T) {
	advisor := &ReplacementAdvisor{
		crossRef: stubReplacementOEMFinder{
			results: []model.OEMReference{
				{RawNumber: "97133-D3000", LegacyArticleId: 2001},
			},
		},
		parts: stubReplacementPartFinder{
			parts: map[int]*model.Part{
				2001: {LegacyArticleId: 2001, ArticleNumber: "97133-F2000", Description: "Cabin Filter", BrandName: "HYUNDAI/KIA"},
			},
		},
		alternatives: stubReplacementAlternativesFinder{
			alts: []AlternativePart{
				{Part: model.Part{LegacyArticleId: 3001, ArticleNumber: "97133-J9000", Description: "Cabin Filter", BrandName: "HYUNDAI/KIA"}},
			},
		},
	}

	current := &model.Part{LegacyArticleId: 100307, ArticleNumber: "97133-D3000", Description: "FILTER-AIR"}
	got, warnings, err := advisor.Build(current, 10001, []string{"97133-D3000"}, 8)
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 replacement candidates, got %d", len(got))
	}
	if got[0].CandidateType != "shared_oem_reference" {
		t.Fatalf("expected first candidate to be shared_oem_reference, got %s", got[0].CandidateType)
	}
	if got[1].CandidateType != "catalog_compatible" {
		t.Fatalf("expected second candidate to be catalog_compatible, got %s", got[1].CandidateType)
	}
	if len(warnings) == 0 {
		t.Fatalf("expected compatibility warning when catalog alternatives are included")
	}
}

func TestReplacementAdvisorDeduplicatesCurrentPart(t *testing.T) {
	advisor := &ReplacementAdvisor{
		crossRef: stubReplacementOEMFinder{
			results: []model.OEMReference{
				{RawNumber: "26300-35505", LegacyArticleId: 101},
			},
		},
		parts:        stubReplacementPartFinder{parts: map[int]*model.Part{}},
		alternatives: stubReplacementAlternativesFinder{},
	}

	current := &model.Part{LegacyArticleId: 101, ArticleNumber: "26300-35505", Description: "Oil Filter"}
	got, _, err := advisor.Build(current, 0, []string{"26300-35505"}, 8)
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected no candidates when only the current part matches, got %d", len(got))
	}
}
