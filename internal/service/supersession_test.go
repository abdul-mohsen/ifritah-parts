package service

import (
	"context"
	"testing"

	"parts-engine/internal/store"
)

type stubSubstitutionLinkFinder struct {
	rows []store.ListSubstitutionLinksForArticleRow
}

func (s stubSubstitutionLinkFinder) ListSubstitutionLinksForArticle(_ context.Context, _ int32) ([]store.ListSubstitutionLinksForArticleRow, error) {
	return s.rows, nil
}

func TestSupersessionPreservesImportedSourceCaution(t *testing.T) {
	svc := &Supersession{
		queries: stubSubstitutionLinkFinder{
			rows: []store.ListSubstitutionLinksForArticleRow{
				{
					ArticleNumber: "26300-35503",
					Description:   "FILTER ASSY-ENGINE OIL",
					Direction:     "reported_replacement",
					SourceKey:     "legacy_discovered_substitutions",
					SourceLabel:   "Imported substitution evidence",
					SourceDetail:  "Imported from a legacy cache.",
					SourceWarning: "Confirm official replacement status.",
					Confidence:    0.72,
				},
			},
		},
	}

	links, err := svc.GetChain(100001)
	if err != nil {
		t.Fatalf("GetChain returned error: %v", err)
	}
	if len(links) != 1 {
		t.Fatalf("expected one link, got %d", len(links))
	}
	if links[0].ArticleNumber != "26300-35503" || links[0].Source.Label != "Imported substitution evidence" {
		t.Fatalf("unexpected link: %+v", links[0])
	}
	if len(links[0].Warnings) != 1 || links[0].Confidence != 0.72 {
		t.Fatalf("source caution was not preserved: %+v", links[0])
	}
}
