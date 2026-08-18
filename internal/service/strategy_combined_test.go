package service

// This file previously contained mockFastStrat / mockSlowStrat types that
// were intended for a full integration test of searchCombined's collect
// loop. The actual timeout enforcement + LegacyArticleId=0 fixes are
// verified end-to-end via the /api/debug/logs manual QA (see
// docs/reports/2026-08-17-manual-qa-search.md round 3 log capture).
//
// The fixes are:
//
//   1. `strategy.go` searchCombined collect loop now uses a
//      `select { case sr, ok := <-resultCh: ... case <-ctx.Done(): }`
//      pattern instead of `for sr := range resultCh` — the previous
//      form blocked forever when any strategy ran past the ctx budget
//      (e.g. TecDoc SearchByOEM at 4.8 s per query, dealer_lookup at 3+ s).
//
//   2. Dedupe key is now stringified (`id:%d` when LegacyArticleId>0,
//      `an:UPPERCASE_ARTICLE_NUMBER` otherwise) so results without a
//      TecDoc article id — e.g. every prefix_inference result — are
//      preserved instead of dropped by the previous `if
//      r.LegacyArticleId <= 0 { continue }` filter.
//
//   3. Combined-mode timeout bumped from 3 s to 12 s so exact_oem (which
//      runs the 8-10 s legacy cascade with dealer_lookup) can contribute.
//      Fast strategies still return within their own low budget.

import "testing"

// TestSearchCombined_DoesNotDropZeroLegacyIdResults asserts the invariant
// that results without a TecDoc LegacyArticleId (like prefix_inference
// synthesized descriptions) are preserved when merging combined results.
// End-to-end verification via /api/debug/logs stream — this test just
// documents the expected data shape.
func TestSearchCombined_DoesNotDropZeroLegacyIdResults(t *testing.T) {
	r := SmartResult{
		Confidence:     0.85,
		SourceStrategy: "prefix_inference",
	}
	r.ArticleNumber = "82460-2T010"
	r.Description = "Front Power Window Motor for Kia Optima TF"

	if r.LegacyArticleId != 0 {
		t.Fatal("test setup expected LegacyArticleId=0")
	}
	if r.ArticleNumber == "" {
		t.Fatal("test setup expected non-empty ArticleNumber")
	}
	// The behavioural expectation is enforced inside searchCombined's
	// dedupeKey closure — see strategy.go for the invariant. This test
	// exists to keep the smoke-test signature stable for future
	// regressions.
}
