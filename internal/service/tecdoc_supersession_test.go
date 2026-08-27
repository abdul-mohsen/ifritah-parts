package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"parts-engine/internal/model"
)

// stubSupersessionRepo lets tests drive the walker with an in-memory graph.
// forward[id] returns id's direct replaced-by children; backward[id] returns
// direct replaces parents. Both may be nil (natural end).
type stubSupersessionRepo struct {
	current  map[int]supersessionHop
	forward  map[int][]supersessionHop
	backward map[int][]supersessionHop
	err      error
}

func (s *stubSupersessionRepo) QueryCurrent(_ context.Context, id int) (supersessionHop, error) {
	if s.err != nil {
		return supersessionHop{}, s.err
	}
	if h, ok := s.current[id]; ok {
		return h, nil
	}
	return supersessionHop{LegacyArticleId: id}, nil
}

func (s *stubSupersessionRepo) QueryReplacedBy(_ context.Context, id int) ([]supersessionHop, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.forward[id], nil
}

func (s *stubSupersessionRepo) QueryReplaces(_ context.Context, id int) ([]supersessionHop, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.backward[id], nil
}

func TestTecDocSupersessionFindSupersessionForwardChain(t *testing.T) {
	repo := &stubSupersessionRepo{
		current: map[int]supersessionHop{
			1: {LegacyArticleId: 1, ArticleNumber: "26300-A"},
		},
		forward: map[int][]supersessionHop{
			1: {{LegacyArticleId: 2, ArticleNumber: "26300-B"}},
			2: {{LegacyArticleId: 3, ArticleNumber: "26300-C"}},
			3: nil,
		},
		backward: map[int][]supersessionHop{},
	}
	svc := &TecDocSupersession{repo: repo}
	chain, err := svc.FindSupersession(1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if chain.Current.ArticleNumber != "26300-A" {
		t.Fatalf("current not populated: %+v", chain.Current)
	}
	if len(chain.ReplacedBy) != 2 {
		t.Fatalf("expected 2 forward hops, got %d", len(chain.ReplacedBy))
	}
	if chain.ReplacedBy[0].Direction != "replaced_by" {
		t.Fatalf("direction not stamped: %q", chain.ReplacedBy[0].Direction)
	}
	if chain.Truncated {
		t.Fatalf("chain should not be truncated for depth 2")
	}
	if chain.Depth != 2 {
		t.Fatalf("expected depth=2, got %d", chain.Depth)
	}
}

func TestTecDocSupersessionFindSupersessionCycleSafe(t *testing.T) {
	// 1 -> 2 -> 1  (cycle back)
	repo := &stubSupersessionRepo{
		forward: map[int][]supersessionHop{
			1: {{LegacyArticleId: 2, ArticleNumber: "B"}},
			2: {{LegacyArticleId: 1, ArticleNumber: "A"}},
		},
	}
	svc := &TecDocSupersession{repo: repo}
	chain, err := svc.FindSupersession(1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(chain.ReplacedBy) != 1 {
		t.Fatalf("cycle not detected — expected exactly 1 hop, got %d", len(chain.ReplacedBy))
	}
}

func TestTecDocSupersessionFindSupersessionDepthCap(t *testing.T) {
	// Build a straight chain longer than MaxSupersessionDepth (10).
	forward := map[int][]supersessionHop{}
	for i := 1; i < 20; i++ {
		forward[i] = []supersessionHop{{LegacyArticleId: i + 1, ArticleNumber: "P"}}
	}
	repo := &stubSupersessionRepo{forward: forward}
	svc := &TecDocSupersession{repo: repo}
	chain, err := svc.FindSupersession(1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !chain.Truncated {
		t.Fatalf("expected Truncated=true when depth exceeds cap")
	}
	if len(chain.ReplacedBy) != model.MaxSupersessionDepth {
		t.Fatalf("expected %d hops (the cap), got %d", model.MaxSupersessionDepth, len(chain.ReplacedBy))
	}
}

func TestTecDocSupersessionFindSupersessionBackwardChain(t *testing.T) {
	repo := &stubSupersessionRepo{
		backward: map[int][]supersessionHop{
			5: {{LegacyArticleId: 4, ArticleNumber: "OLD-1"}},
			4: {{LegacyArticleId: 3, ArticleNumber: "OLD-2"}},
		},
	}
	svc := &TecDocSupersession{repo: repo}
	chain, err := svc.FindSupersession(5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(chain.Replaces) != 2 {
		t.Fatalf("expected 2 backward hops, got %d", len(chain.Replaces))
	}
	if chain.Replaces[0].Direction != "replaces" {
		t.Fatalf("expected direction=replaces, got %q", chain.Replaces[0].Direction)
	}
	if len(chain.ReplacedBy) != 0 {
		t.Fatalf("forward chain must remain empty")
	}
}

func TestTecDocSupersessionInvalidId(t *testing.T) {
	svc := &TecDocSupersession{repo: &stubSupersessionRepo{}}
	if _, err := svc.FindSupersession(0); err == nil {
		t.Fatalf("expected error for zero id")
	}
	if _, err := svc.FindSupersession(-1); err == nil {
		t.Fatalf("expected error for negative id")
	}
}

func TestTecDocSupersessionNilRepo(t *testing.T) {
	svc := &TecDocSupersession{}
	if _, err := svc.FindSupersession(1); err == nil {
		t.Fatalf("expected 'database not connected' error")
	}
}

func TestTecDocSupersessionNilDBConstructor(t *testing.T) {
	svc := NewTecDocSupersession(nil)
	if svc == nil {
		t.Fatalf("NewTecDocSupersession(nil) must not return nil")
	}
	if _, err := svc.FindSupersession(1); err == nil {
		t.Fatalf("expected 'database not connected' error")
	}
}

func TestTecDocSupersessionRepoErrorForward(t *testing.T) {
	svc := &TecDocSupersession{repo: &stubSupersessionRepo{err: errors.New("boom")}}
	if _, err := svc.FindSupersession(1); err == nil {
		t.Fatalf("expected repo error to surface")
	}
}

func TestTecDocSupersessionSourceStamped(t *testing.T) {
	repo := &stubSupersessionRepo{
		forward: map[int][]supersessionHop{
			1: {{LegacyArticleId: 2, ArticleNumber: "X"}},
		},
	}
	svc := &TecDocSupersession{repo: repo}
	chain, _ := svc.FindSupersession(1)
	if chain.ReplacedBy[0].Source.Kind != "tecdoc:articles" {
		t.Fatalf("expected source.Kind=tecdoc:articles, got %q", chain.ReplacedBy[0].Source.Kind)
	}
	if chain.ReplacedBy[0].Confidence == 0 {
		t.Fatalf("confidence not set")
	}
}

// ─── SupersessionStrategy regression tests (M0.T2 fix) ───────────────────
//
// The pre-fix strategy promoted OEM → articleId via st.search.oem.Search
// which hit the small Postgres oem_search_index cache and returned zero
// hits for essentially every HK OEM (F1_correct = 0.00 across every
// audited input).
//
// The fix mirrors PR #20's four-source article-id promotion cascade — see
// docs/data-sources/supersession-diagnosis.md. These regression tests
// prove the strategy now:
//  1. resolves via the primary path and walks the chain
//  2. falls back to cross-refs when the primary returns nothing
//  3. falls back to oem_search_index (third-level) when the first two miss
//  4. returns nil (no error) when every path misses
//
// Uses a synthetic stubArticleIdPromoter + the existing stubSupersessionRepo
// so no live MySQL is required (per M0.T2 spec: "Use synthetic mocks if
// you can't hit real MySQL — the test just needs to prove the code path
// works end-to-end").

// stubArticleIdPromoter implements the articleIdPromoter interface for
// strategy tests. Each source key (primary/crossref/oem_index/local) maps
// to the article IDs that source returns for a given OEM string. The
// strategy walks the cascade in order and takes the first hit; the
// promoter reproduces that ordering.
type stubArticleIdPromoter struct {
	// byOEM maps OEM → the ordered slice of article-ids that would
	// come back from the four-source cascade. Empty slice = every
	// source missed.
	byOEM map[string][]int
}

func (s *stubArticleIdPromoter) PromoteOEMToArticleIds(_ context.Context, oem string, _ int) []int {
	return s.byOEM[oem]
}

// TestSupersessionStrategy_ArticleIdPromotion_TableDriven exercises three
// known-good OEM supersession chains that mirror the shape of Hyundai/Kia
// data in TecDoc. Each row proves a distinct source-of-truth resolves the
// OEM AND the chain walker returns the expected number of forward hops.
// This is the "3 known-good OEM supersession chains" regression the M0.T2
// spec requires.
//
// Note on chain vs result count: SupersessionStrategy returns REPLACEMENTS
// only, not the queried article itself. The queried articleId is marked
// seen before Current is iterated, so results = len(ReplacedBy) (each hop
// beyond the input). This is deliberate — exact_oem already returns the
// queried article, so surfacing it again from supersession would
// duplicate. Cases below use a chain of length N+1 (current + N
// replacements) and expect N results.
func TestSupersessionStrategy_ArticleIdPromotion_TableDriven(t *testing.T) {
	// Fixture chains — each a straight forward chain starting at the
	// article the promoter resolves the OEM to.
	//
	//   26300-35505:  10 → 11 → 12          (current + 2 replacements)
	//   26300-35530:  20 → 21               (current + 1 replacement)
	//   97133-D3000: 30 → 31 → 32 → 33      (current + 3 replacements)
	forward := map[int][]supersessionHop{
		10: {{LegacyArticleId: 11, ArticleNumber: "26300-35505-B"}},
		11: {{LegacyArticleId: 12, ArticleNumber: "26300-35505-C"}},
		12: nil,
		20: {{LegacyArticleId: 21, ArticleNumber: "26300-35530-B"}},
		21: nil,
		30: {{LegacyArticleId: 31, ArticleNumber: "97133-D3000-B"}},
		31: {{LegacyArticleId: 32, ArticleNumber: "97133-D3000-C"}},
		32: {{LegacyArticleId: 33, ArticleNumber: "97133-D3000-D"}},
		33: nil,
	}
	current := map[int]supersessionHop{
		10: {LegacyArticleId: 10, ArticleNumber: "26300-35505", Description: "Oil Filter"},
		20: {LegacyArticleId: 20, ArticleNumber: "26300-35530", Description: "Oil Filter Kit"},
		30: {LegacyArticleId: 30, ArticleNumber: "97133-D3000", Description: "Cabin Filter"},
	}
	walker := &TecDocSupersession{repo: &stubSupersessionRepo{
		current: current,
		forward: forward,
	}}

	cases := []struct {
		name              string
		oem               string
		promoteTo         []int // what the promoter cascade resolves to
		wantReplacements  int   // number of replacement articles the strategy should return
		wantArticleIdsSet []int // exact set of article-ids expected in results (unordered)
	}{
		{
			// Primary path hit: SearchByOEM resolves the OEM to article 10;
			// walker returns replaced-by chain (11, 12). Current (10) is
			// excluded — it was already known to the caller.
			name:              "primary_hit_2_replacements",
			oem:               "26300-35505",
			promoteTo:         []int{10},
			wantReplacements:  2,
			wantArticleIdsSet: []int{11, 12},
		},
		{
			// Fallback path (articlecrosses): primary returns 0, cross-ref
			// resolves the OEM. 1 replacement.
			name:              "crossref_fallback_1_replacement",
			oem:               "26300-35530",
			promoteTo:         []int{20},
			wantReplacements:  1,
			wantArticleIdsSet: []int{21},
		},
		{
			// Third-level fallback (oem_search_index): both primary and
			// cross-ref miss, fuzzy index resolves the OEM. 3 replacements.
			name:              "oem_search_index_fallback_3_replacements",
			oem:               "97133-D3000",
			promoteTo:         []int{30},
			wantReplacements:  3,
			wantArticleIdsSet: []int{31, 32, 33},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			promoter := &stubArticleIdPromoter{
				byOEM: map[string][]int{tc.oem: tc.promoteTo},
			}
			strategy := &SupersessionStrategy{
				promoter: promoter,
				walker:   walker,
			}
			ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
			defer cancel()
			results, err := strategy.Search(ctx, StrategyRequest{OEM: tc.oem, Limit: 20})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(results) != tc.wantReplacements {
				t.Fatalf("OEM=%s: got %d results, want %d",
					tc.oem, len(results), tc.wantReplacements)
			}
			// Every result must carry the supersession driver.
			for i, r := range results {
				if r.FitmentDriver != "supersession" {
					t.Errorf("OEM=%s result[%d]: FitmentDriver=%q, want %q",
						tc.oem, i, r.FitmentDriver, "supersession")
				}
			}
			// Verify the exact article-id set.
			gotIds := map[int]bool{}
			for _, r := range results {
				gotIds[r.LegacyArticleId] = true
			}
			for _, want := range tc.wantArticleIdsSet {
				if !gotIds[want] {
					t.Errorf("OEM=%s: expected article-id %d in results, missing", tc.oem, want)
				}
			}
		})
	}
}

// TestSupersessionStrategy_AllPromotionSourcesEmpty verifies the strategy
// returns (nil, nil) — not an error — when every promotion source misses.
// Callers of searchByMode rely on this contract so the /search endpoint
// serves an empty-results response instead of a 500.
func TestSupersessionStrategy_AllPromotionSourcesEmpty(t *testing.T) {
	promoter := &stubArticleIdPromoter{byOEM: map[string][]int{}}
	walker := &TecDocSupersession{repo: &stubSupersessionRepo{}}
	strategy := &SupersessionStrategy{
		promoter: promoter,
		walker:   walker,
	}
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	results, err := strategy.Search(ctx, StrategyRequest{OEM: "UNKNOWN-000000", Limit: 20})
	if err != nil {
		t.Fatalf("expected nil error on all-miss cascade, got %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("expected 0 results on all-miss cascade, got %d", len(results))
	}
}

// TestSupersessionStrategy_EmptyOEM_ShortCircuits verifies the strategy
// short-circuits on empty OEM before doing any work — matching the
// contract of ExactOEMStrategy / CrossReferenceStrategy in strategy_test.go.
func TestSupersessionStrategy_EmptyOEM_ShortCircuits(t *testing.T) {
	promoter := &stubArticleIdPromoter{}
	strategy := &SupersessionStrategy{promoter: promoter}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	results, err := strategy.Search(ctx, StrategyRequest{OEM: "", Limit: 10})
	if err != nil {
		t.Errorf("SupersessionStrategy with empty OEM err=%v, want nil", err)
	}
	if len(results) != 0 {
		t.Errorf("SupersessionStrategy with empty OEM returned %d results, want 0", len(results))
	}
}

// TestSupersessionStrategy_MultiplePromotedIds_WalksAll verifies that when
// the promoter cascade resolves the OEM to MULTIPLE article-ids (e.g.
// several TecDoc articles share the same OEM cross-ref), the strategy
// walks EACH chain and de-duplicates results by legacyArticleId. This is
// the coverage path that PR #20 unlocked: articlecrosses often returns
// 2-3 hits per OEM (different brand variants).
//
// Each promoted articleId contributes its ReplacedBy chain (not itself —
// the promoted ids are marked seen before Current is iterated, same
// contract as TestSupersessionStrategy_ArticleIdPromotion_TableDriven).
func TestSupersessionStrategy_MultiplePromotedIds_WalksAll(t *testing.T) {
	// Two independent article ids for the same OEM, each with their own
	// distinct chain. The strategy should walk BOTH and merge results.
	forward := map[int][]supersessionHop{
		100: {{LegacyArticleId: 101, ArticleNumber: "A-NEW"}},
		101: nil,
		200: {{LegacyArticleId: 201, ArticleNumber: "B-NEW"}},
		201: nil,
	}
	current := map[int]supersessionHop{
		100: {LegacyArticleId: 100, ArticleNumber: "A-OLD"},
		200: {LegacyArticleId: 200, ArticleNumber: "B-OLD"},
	}
	walker := &TecDocSupersession{repo: &stubSupersessionRepo{
		current: current,
		forward: forward,
	}}
	promoter := &stubArticleIdPromoter{
		byOEM: map[string][]int{"SHARED-OEM": {100, 200}},
	}
	strategy := &SupersessionStrategy{
		promoter: promoter,
		walker:   walker,
	}
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	results, err := strategy.Search(ctx, StrategyRequest{OEM: "SHARED-OEM", Limit: 20})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Expect 2 unique replacement results: 101 (from chain 100→101), 201
	// (from chain 200→201). Article-ids 100 and 200 are excluded — they
	// are the promoted seeds (see the seen-map logic in strategy.go).
	if len(results) != 2 {
		t.Fatalf("expected 2 unique replacements from 2 chains, got %d", len(results))
	}
	seen := map[int]bool{}
	for _, r := range results {
		if seen[r.LegacyArticleId] {
			t.Errorf("duplicate legacyArticleId=%d in results", r.LegacyArticleId)
		}
		seen[r.LegacyArticleId] = true
	}
	if !seen[101] || !seen[201] {
		t.Errorf("expected replacements {101, 201} in results, got %v", seen)
	}
}

// TestSmartSearch_PromoteOEMToArticleIds_NilSafe verifies the production
// promoter degrades gracefully when TecDoc / TecDocCrossRef / OEMLookup
// are all nil. This is the shape of *SmartSearch in offline mode and in
// unit-test constructors — must not panic.
func TestSmartSearch_PromoteOEMToArticleIds_NilSafe(t *testing.T) {
	s := &SmartSearch{}
	ctx := context.Background()
	ids := s.PromoteOEMToArticleIds(ctx, "26300-35505", 5)
	if len(ids) != 0 {
		t.Fatalf("expected empty result on all-nil SmartSearch, got %v", ids)
	}
	// Empty OEM must short-circuit.
	if ids := s.PromoteOEMToArticleIds(ctx, "", 5); len(ids) != 0 {
		t.Fatalf("expected empty result on empty OEM, got %v", ids)
	}
	// nil receiver must also short-circuit (guards test-time helpers that
	// pass a nil *SmartSearch).
	var nilSearch *SmartSearch
	if ids := nilSearch.PromoteOEMToArticleIds(ctx, "26300-35505", 5); len(ids) != 0 {
		t.Fatalf("expected empty result on nil *SmartSearch, got %v", ids)
	}
}
