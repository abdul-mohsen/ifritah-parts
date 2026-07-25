package service

import "testing"

func TestDefaultExternalSourceCatalogHasExpectedBuckets(t *testing.T) {
	records := DefaultExternalSourceCatalog()
	if len(records) < 10 {
		t.Fatalf("expected seeded source registry, got %d sources", len(records))
	}

	var backend, research, rejected int
	for _, record := range records {
		switch record.Recommendation {
		case "backend_enrichment":
			backend++
		case "research_only":
			research++
		case "rejected":
			rejected++
		}
		if record.UserFacingEligible {
			t.Fatalf("expected no user-facing eligible sources at this stage, found %s", record.SourceKey)
		}
	}

	if backend == 0 || research == 0 || rejected == 0 {
		t.Fatalf("expected all source recommendation buckets, got backend=%d research=%d rejected=%d", backend, research, rejected)
	}
}

func TestDefaultExternalSourceAssessmentsMatchCatalog(t *testing.T) {
	records := DefaultExternalSourceCatalog()
	assessments := DefaultExternalSourceAssessments()
	if len(records) != len(assessments) {
		t.Fatalf("expected one assessment per source, got %d sources and %d assessments", len(records), len(assessments))
	}
}
