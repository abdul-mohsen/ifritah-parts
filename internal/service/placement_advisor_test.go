package service

import (
	"testing"

	"parts-engine/internal/model"
)

func TestPlacementAdvisorFallsBackToInferredCabinFilterHint(t *testing.T) {
	advisor := NewPlacementAdvisor(nil)
	part := &model.Part{
		ArticleNumber: "97133-D3000",
		Description:   "Cabin Filter",
		Category:      "Cabin Filter & Blower",
	}

	placement := advisor.Build(part, nil, nil)
	if placement.Kind != "inferred" {
		t.Fatalf("expected inferred placement, got %s", placement.Kind)
	}
	if placement.LocationArea == "" {
		t.Fatalf("expected inferred placement to include a location area")
	}
	if len(placement.Hints) == 0 {
		t.Fatalf("expected inferred placement guidance hints")
	}
}

func TestPlacementAdvisorDoesNotOverclaimUnknownPart(t *testing.T) {
	advisor := NewPlacementAdvisor(nil)
	part := &model.Part{
		ArticleNumber: "X-UNKNOWN",
		Description:   "Unmapped component",
	}

	placement := advisor.Build(part, nil, nil)
	if placement.Kind != "unavailable" {
		t.Fatalf("expected unavailable placement, got %s", placement.Kind)
	}
}
