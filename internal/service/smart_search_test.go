package service

import (
	"testing"

	"parts-engine/internal/model"
)

func TestSortOEMReferencesRanksExactCatalogPartFirst(t *testing.T) {
	refs := []model.OEMReference{
		{RawNumber: "26300-35505", ArticleNumber: "W 811/80", BrandName: "MANN-FILTER"},
		{RawNumber: "26300-35505", ArticleNumber: "26300-35505", BrandName: "HYUNDAI/KIA"},
		{RawNumber: "26300-35505", ArticleNumber: "OC 205", BrandName: "MAHLE"},
	}

	sortOEMReferences(refs, "26300-35505")

	if refs[0].ArticleNumber != "26300-35505" {
		t.Fatalf("first result = %q, want exact catalog part number", refs[0].ArticleNumber)
	}
}
