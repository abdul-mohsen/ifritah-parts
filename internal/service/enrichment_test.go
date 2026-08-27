package service

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"

	"parts-engine/internal/model"
)

// ─── promoteArticleIds — chained article-id promotion (M3.S1.T1) ─────────

// stubOEMPromoter is a per-test fixture implementing oemPromoter with
// recorded call counts. Each PromoteBy* returns the pre-seeded refs
// (or the pre-seeded error) and increments its counter, so assertions
// can verify the fast-path shape (layers 2+3 not called when layer 1
// hits) without a live MySQL connection.
//
// FetchDataSupplierIds returns the pre-seeded supplierByArt map; the
// zero-value map is treated by pickCanonicalArticleId as "no tiebreak
// data" → falls back to first-seen id, so tests that don't care about
// the canonical pick can leave supplierByArt nil.
type stubOEMPromoter struct {
	byOEMRefs      []model.OEMReference
	byOEMErr       error
	crossRefRefs   []model.OEMReference
	crossRefErr    error
	byIndexRefs    []model.OEMReference
	byIndexErr     error
	supplierByArt  map[int]int
	fetchErr       error
	fetchCallCount int32

	byOEMCalls    int32
	crossRefCalls int32
	byIndexCalls  int32
}

func (s *stubOEMPromoter) PromoteByOEM(oem string, limit int) ([]model.OEMReference, error) {
	atomic.AddInt32(&s.byOEMCalls, 1)
	return s.byOEMRefs, s.byOEMErr
}

func (s *stubOEMPromoter) PromoteByCrossReferences(oem string, limit int) ([]model.OEMReference, error) {
	atomic.AddInt32(&s.crossRefCalls, 1)
	return s.crossRefRefs, s.crossRefErr
}

func (s *stubOEMPromoter) PromoteByOEMIndex(oem string, limit int) ([]model.OEMReference, error) {
	atomic.AddInt32(&s.byIndexCalls, 1)
	return s.byIndexRefs, s.byIndexErr
}

func (s *stubOEMPromoter) FetchDataSupplierIds(articleIds []int) (map[int]int, error) {
	atomic.AddInt32(&s.fetchCallCount, 1)
	if s.fetchErr != nil {
		return nil, s.fetchErr
	}
	if s.supplierByArt == nil {
		return nil, nil
	}
	// Return only the requested subset — matches real DB behaviour where
	// missing ids don't appear in the result set.
	out := make(map[int]int, len(articleIds))
	for _, id := range articleIds {
		if v, ok := s.supplierByArt[id]; ok {
			out[id] = v
		}
	}
	return out, nil
}

// TestPromoteArticleIds_PrimaryHit_LayersTwoAndThreeNotCalled verifies (a):
// when layer 1 (oem_number) returns candidates, layers 2 and 3 must NOT
// be consulted. This is the fast-path guarantee — the layer-1 signal is
// authoritative when it fires, so the pipeline must not pay the DB
// round-trip cost of the fallbacks.
func TestPromoteArticleIds_PrimaryHit_LayersTwoAndThreeNotCalled(t *testing.T) {
	stub := &stubOEMPromoter{
		byOEMRefs: []model.OEMReference{
			{LegacyArticleId: 100001, ArticleNumber: "26300-35503"},
		},
	}
	best, refs, err := promoteArticleIds(context.Background(), stub, "26300-35503", 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if best != 100001 {
		t.Errorf("best articleId = %d, want 100001", best)
	}
	if len(refs) != 1 {
		t.Errorf("refs len = %d, want 1", len(refs))
	}
	if got := atomic.LoadInt32(&stub.byOEMCalls); got != 1 {
		t.Errorf("byOEMCalls = %d, want 1", got)
	}
	if got := atomic.LoadInt32(&stub.crossRefCalls); got != 0 {
		t.Errorf("crossRefCalls = %d, want 0 (fast-path: layer 2 must not be called)", got)
	}
	if got := atomic.LoadInt32(&stub.byIndexCalls); got != 0 {
		t.Errorf("byIndexCalls = %d, want 0 (fast-path: layer 3 must not be called)", got)
	}
	// Single-candidate → no supplier lookup.
	if got := atomic.LoadInt32(&stub.fetchCallCount); got != 0 {
		t.Errorf("FetchDataSupplierIds called %d times; single-candidate path must not consult supplier lookup", got)
	}
}

// TestPromoteArticleIds_FallbackToLayerTwo verifies (b): layer 1 returns
// nothing (empty), layer 2 (articlecrosses) returns candidates → pipeline
// uses layer 2's article id and does NOT call layer 3.
func TestPromoteArticleIds_FallbackToLayerTwo(t *testing.T) {
	stub := &stubOEMPromoter{
		byOEMRefs: nil, // empty
		crossRefRefs: []model.OEMReference{
			{LegacyArticleId: 200002, ArticleNumber: "W-811-80"},
		},
	}
	best, refs, err := promoteArticleIds(context.Background(), stub, "26300-35503", 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if best != 200002 {
		t.Errorf("best articleId = %d, want 200002 (from layer 2)", best)
	}
	if len(refs) != 1 {
		t.Errorf("refs len = %d, want 1", len(refs))
	}
	if got := atomic.LoadInt32(&stub.byOEMCalls); got != 1 {
		t.Errorf("byOEMCalls = %d, want 1", got)
	}
	if got := atomic.LoadInt32(&stub.crossRefCalls); got != 1 {
		t.Errorf("crossRefCalls = %d, want 1", got)
	}
	if got := atomic.LoadInt32(&stub.byIndexCalls); got != 0 {
		t.Errorf("byIndexCalls = %d, want 0 (layer 3 must not run after layer 2 succeeded)", got)
	}
}

// TestPromoteArticleIds_FallbackToLayerThree verifies (c): both layer 1
// and layer 2 return empty; layer 3 (oem_search_index) still promotes.
// This is the audit-driven case — HK OEMs that only appear in the fuzzy
// oem_search_index rescue table.
func TestPromoteArticleIds_FallbackToLayerThree(t *testing.T) {
	stub := &stubOEMPromoter{
		byOEMRefs:    nil,
		crossRefRefs: nil,
		byIndexRefs: []model.OEMReference{
			{LegacyArticleId: 300003, ArticleNumber: "26300-2Y500"},
		},
	}
	best, refs, err := promoteArticleIds(context.Background(), stub, "26300-2Y500", 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if best != 300003 {
		t.Errorf("best articleId = %d, want 300003 (from layer 3)", best)
	}
	if len(refs) != 1 {
		t.Errorf("refs len = %d, want 1", len(refs))
	}
	if got := atomic.LoadInt32(&stub.byOEMCalls); got != 1 {
		t.Errorf("byOEMCalls = %d, want 1", got)
	}
	if got := atomic.LoadInt32(&stub.crossRefCalls); got != 1 {
		t.Errorf("crossRefCalls = %d, want 1", got)
	}
	if got := atomic.LoadInt32(&stub.byIndexCalls); got != 1 {
		t.Errorf("byIndexCalls = %d, want 1", got)
	}
}

// TestPromoteArticleIds_AllEmpty_ReturnsSentinel verifies (d): when every
// layer returns zero refs, the pipeline returns errNoPromotion. Callers
// use errors.Is to distinguish "no article-anchored enrichment possible"
// from a real error (ctx cancellation, DB failure).
func TestPromoteArticleIds_AllEmpty_ReturnsSentinel(t *testing.T) {
	stub := &stubOEMPromoter{
		byOEMRefs:    nil,
		crossRefRefs: nil,
		byIndexRefs:  nil,
	}
	best, refs, err := promoteArticleIds(context.Background(), stub, "UNKNOWN-OEM", 5)
	if !errors.Is(err, errNoPromotion) {
		t.Errorf("err = %v, want errNoPromotion", err)
	}
	if best != 0 {
		t.Errorf("best articleId = %d, want 0", best)
	}
	if refs != nil {
		t.Errorf("refs = %+v, want nil", refs)
	}
	// All three layers must have been consulted before giving up.
	if got := atomic.LoadInt32(&stub.byOEMCalls); got != 1 {
		t.Errorf("byOEMCalls = %d, want 1", got)
	}
	if got := atomic.LoadInt32(&stub.crossRefCalls); got != 1 {
		t.Errorf("crossRefCalls = %d, want 1", got)
	}
	if got := atomic.LoadInt32(&stub.byIndexCalls); got != 1 {
		t.Errorf("byIndexCalls = %d, want 1", got)
	}
}

// TestPromoteArticleIds_MultiCandidateDedupePicksCanonical verifies (e):
// when layer 1 returns multiple candidates (real-world case: Bosch, MANN,
// MAHLE all catalog the same Hyundai OEM), the pipeline must:
//   - dedupe by articleId (same supplier row appearing twice collapses)
//   - pick the candidate with the highest dataSupplierId (canonical /
//     most recently cataloged), NOT first-seen
//
// The mock returns 4 refs: id=1 (first-seen, low supplier), id=2 (dup of
// id=1 — must be deduped), id=3 (highest supplier), id=4 (mid supplier).
// After dedup we have [1, 3, 4]; supplier map = {1: 100, 3: 999, 4: 500};
// winner must be id=3.
func TestPromoteArticleIds_MultiCandidateDedupePicksCanonical(t *testing.T) {
	stub := &stubOEMPromoter{
		byOEMRefs: []model.OEMReference{
			{LegacyArticleId: 1, ArticleNumber: "A-first-seen"},
			{LegacyArticleId: 2, ArticleNumber: "A-dup1-should-live"},
			{LegacyArticleId: 1, ArticleNumber: "A-dup-of-first"}, // dedup drops this
			{LegacyArticleId: 3, ArticleNumber: "A-canonical-winner"},
			{LegacyArticleId: 4, ArticleNumber: "A-runner-up"},
		},
		supplierByArt: map[int]int{
			1: 100,
			2: 250,
			3: 999, // highest → winner
			4: 500,
		},
	}
	best, refs, err := promoteArticleIds(context.Background(), stub, "26300-35503", 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if best != 3 {
		t.Errorf("best articleId = %d, want 3 (highest dataSupplierId=999)", best)
	}
	// Deduped ref list: id=1, id=2, id=3, id=4 → 4 refs (the id=1 dup dropped).
	if len(refs) != 4 {
		t.Errorf("refs len = %d, want 4 (dedup should drop 1 duplicate)", len(refs))
	}
	// Supplier lookup called exactly once (batch), with 4 unique ids.
	if got := atomic.LoadInt32(&stub.fetchCallCount); got != 1 {
		t.Errorf("FetchDataSupplierIds called %d times, want 1 (batched)", got)
	}
	// Fast-path still applies — layers 2 and 3 must not be called.
	if got := atomic.LoadInt32(&stub.crossRefCalls); got != 0 {
		t.Errorf("crossRefCalls = %d, want 0", got)
	}
	if got := atomic.LoadInt32(&stub.byIndexCalls); got != 0 {
		t.Errorf("byIndexCalls = %d, want 0", got)
	}
}

// ─── promoteArticleIds — error and edge-case handling ─────────────────────

// TestPromoteArticleIds_NilPromoter returns errNoPromotion immediately
// without panicking. Defensive guard for callers whose services are
// wired lazily.
func TestPromoteArticleIds_NilPromoter(t *testing.T) {
	best, refs, err := promoteArticleIds(context.Background(), nil, "26300-35503", 5)
	if !errors.Is(err, errNoPromotion) {
		t.Errorf("err = %v, want errNoPromotion", err)
	}
	if best != 0 || refs != nil {
		t.Errorf("best/refs = %d/%v, want 0/nil", best, refs)
	}
}

// TestPromoteArticleIds_EmptyOEM likewise returns errNoPromotion without
// dispatching any query.
func TestPromoteArticleIds_EmptyOEM(t *testing.T) {
	stub := &stubOEMPromoter{}
	_, _, err := promoteArticleIds(context.Background(), stub, "", 5)
	if !errors.Is(err, errNoPromotion) {
		t.Errorf("err = %v, want errNoPromotion", err)
	}
	if got := atomic.LoadInt32(&stub.byOEMCalls); got != 0 {
		t.Errorf("byOEMCalls = %d, want 0 (empty OEM must short-circuit)", got)
	}
}

// TestPromoteArticleIds_CtxCancelled honors context cancellation between
// layers. If ctx fires before layer 1 completes, the pipeline surfaces
// ctx.Err() to the caller (not errNoPromotion, so callers can
// distinguish "no data" from "gave up").
func TestPromoteArticleIds_CtxCancelled(t *testing.T) {
	stub := &stubOEMPromoter{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // pre-cancel

	best, refs, err := promoteArticleIds(ctx, stub, "26300-35503", 5)
	if err == nil {
		t.Fatalf("err = nil, want ctx.Canceled")
	}
	if errors.Is(err, errNoPromotion) {
		t.Errorf("err = %v, want ctx cancellation (not errNoPromotion)", err)
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want context.Canceled", err)
	}
	if best != 0 || refs != nil {
		t.Errorf("best/refs = %d/%v, want 0/nil on cancel", best, refs)
	}
	// Ctx was already cancelled — no layer should have been reached.
	if got := atomic.LoadInt32(&stub.byOEMCalls); got != 0 {
		t.Errorf("byOEMCalls = %d, want 0 (ctx pre-cancelled)", got)
	}
}

// TestPromoteArticleIds_LayerErrorSoftFalls verifies that when a layer
// returns an ERROR (not just zero refs), the pipeline logs and moves to
// the next layer instead of giving up. Regression guard: a MySQL blip
// on articlecrosses shouldn't kill enrichment when oem_search_index is
// still available.
func TestPromoteArticleIds_LayerErrorSoftFalls(t *testing.T) {
	stub := &stubOEMPromoter{
		byOEMRefs:   nil, // empty
		crossRefErr: fmt.Errorf("simulated MySQL timeout on articlecrosses"),
		byIndexRefs: []model.OEMReference{
			{LegacyArticleId: 300003, ArticleNumber: "R-rescue"},
		},
	}
	best, refs, err := promoteArticleIds(context.Background(), stub, "26300-35503", 5)
	if err != nil {
		t.Fatalf("err = %v, want nil (layer error must not be fatal)", err)
	}
	if best != 300003 {
		t.Errorf("best articleId = %d, want 300003 (layer 3 rescue)", best)
	}
	if len(refs) != 1 {
		t.Errorf("refs len = %d, want 1", len(refs))
	}
	if got := atomic.LoadInt32(&stub.crossRefCalls); got != 1 {
		t.Errorf("crossRefCalls = %d, want 1 (was called but errored)", got)
	}
	if got := atomic.LoadInt32(&stub.byIndexCalls); got != 1 {
		t.Errorf("byIndexCalls = %d, want 1 (must run after layer 2 errored)", got)
	}
}

// TestPromoteArticleIds_ZeroIdRefsPreservedWithSentinelArticleId verifies
// the edge case where a layer returns refs but all have LegacyArticleId==0
// (raw cross-references with no article-table match). The pipeline
// returns best=0 (caller skips article-anchored enrichment) but preserves
// the refs (so they can populate OEMNumbers for display).
func TestPromoteArticleIds_ZeroIdRefsPreservedWithSentinelArticleId(t *testing.T) {
	stub := &stubOEMPromoter{
		byOEMRefs: []model.OEMReference{
			{LegacyArticleId: 0, ArticleNumber: "raw-crossref-1"},
			{LegacyArticleId: 0, ArticleNumber: "raw-crossref-2"},
		},
	}
	best, refs, err := promoteArticleIds(context.Background(), stub, "26300-35503", 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if best != 0 {
		t.Errorf("best articleId = %d, want 0 (no articleId in refs)", best)
	}
	if len(refs) != 2 {
		t.Errorf("refs len = %d, want 2 (zero-id refs preserved for OEMNumbers)", len(refs))
	}
	// Zero-id refs must NOT trigger a supplier lookup.
	if got := atomic.LoadInt32(&stub.fetchCallCount); got != 0 {
		t.Errorf("FetchDataSupplierIds called %d times; zero-id path must not trigger tiebreak", got)
	}
	// Fast-path: layer 2+3 not called even when best=0 because layer 1
	// DID return refs (albeit with articleId=0).
	if got := atomic.LoadInt32(&stub.crossRefCalls); got != 0 {
		t.Errorf("crossRefCalls = %d, want 0", got)
	}
}

// ─── pickCanonicalArticleId — supplier-lookup fallback semantics ─────────

// TestPickCanonicalArticleId_FetchErrorFallsBackToFirstSeen verifies that
// when FetchDataSupplierIds fails, the pipeline falls back to the
// first-seen article id (deterministic, no panic) instead of failing the
// whole enrichment path.
func TestPickCanonicalArticleId_FetchErrorFallsBackToFirstSeen(t *testing.T) {
	stub := &stubOEMPromoter{
		fetchErr: fmt.Errorf("simulated supplier lookup failure"),
	}
	refs := []model.OEMReference{
		{LegacyArticleId: 42, ArticleNumber: "first"},
		{LegacyArticleId: 43, ArticleNumber: "second"},
	}
	best := pickCanonicalArticleId(stub, refs)
	if best != 42 {
		t.Errorf("best = %d, want 42 (first-seen fallback on fetch error)", best)
	}
}

// TestPickCanonicalArticleId_EmptySupplierMapFallsBack tests the case where
// the DB returned no rows for the requested article ids (map is empty but
// no error). Same fallback: first-seen wins.
func TestPickCanonicalArticleId_EmptySupplierMapFallsBack(t *testing.T) {
	stub := &stubOEMPromoter{
		supplierByArt: nil, // stub returns (nil, nil) — empty map
	}
	refs := []model.OEMReference{
		{LegacyArticleId: 42, ArticleNumber: "first"},
		{LegacyArticleId: 43, ArticleNumber: "second"},
	}
	best := pickCanonicalArticleId(stub, refs)
	if best != 42 {
		t.Errorf("best = %d, want 42 (first-seen fallback on empty supplier map)", best)
	}
}

// TestPickCanonicalArticleId_TieBreaksToFirstSeen verifies that when two
// candidates have the SAME (highest) dataSupplierId, the first-seen wins.
// This is important for determinism — identical requests must return the
// same article id.
func TestPickCanonicalArticleId_TieBreaksToFirstSeen(t *testing.T) {
	stub := &stubOEMPromoter{
		supplierByArt: map[int]int{
			42: 500,
			43: 500, // tied
			44: 300,
		},
	}
	refs := []model.OEMReference{
		{LegacyArticleId: 42, ArticleNumber: "tied-first"},
		{LegacyArticleId: 43, ArticleNumber: "tied-second"},
		{LegacyArticleId: 44, ArticleNumber: "lower"},
	}
	best := pickCanonicalArticleId(stub, refs)
	if best != 42 {
		t.Errorf("best = %d, want 42 (tie broken to first-seen)", best)
	}
}

// ─── dedupeOEMRefsByArticleId — dedup correctness ─────────────────────────

// TestDedupeOEMRefsByArticleId_CollapsesDuplicates asserts that refs with
// the same LegacyArticleId are collapsed to a single entry (first-seen
// preserved) while zero-id refs pass through untouched.
func TestDedupeOEMRefsByArticleId_CollapsesDuplicates(t *testing.T) {
	in := []model.OEMReference{
		{LegacyArticleId: 42, ArticleNumber: "first"},
		{LegacyArticleId: 42, ArticleNumber: "dup-of-first"}, // dropped
		{LegacyArticleId: 43, ArticleNumber: "unique"},
		{LegacyArticleId: 0, ArticleNumber: "zero-id-passes-1"}, // kept
		{LegacyArticleId: 0, ArticleNumber: "zero-id-passes-2"}, // kept — no dedup for zero-id
		{LegacyArticleId: 43, ArticleNumber: "dup-of-43"},       // dropped
	}
	out := dedupeOEMRefsByArticleId(in)
	// Expected: id=42 (first), id=43 (first), zero-id-1, zero-id-2 → 4 entries
	if len(out) != 4 {
		t.Fatalf("out len = %d, want 4: %+v", len(out), out)
	}
	if out[0].LegacyArticleId != 42 || out[0].ArticleNumber != "first" {
		t.Errorf("out[0] = %+v, want {42, first}", out[0])
	}
	if out[1].LegacyArticleId != 43 || out[1].ArticleNumber != "unique" {
		t.Errorf("out[1] = %+v, want {43, unique}", out[1])
	}
	if out[2].LegacyArticleId != 0 || out[2].ArticleNumber != "zero-id-passes-1" {
		t.Errorf("out[2] = %+v, want {0, zero-id-passes-1}", out[2])
	}
	if out[3].LegacyArticleId != 0 || out[3].ArticleNumber != "zero-id-passes-2" {
		t.Errorf("out[3] = %+v, want {0, zero-id-passes-2}", out[3])
	}
}

// TestDedupeOEMRefsByArticleId_EmptyPassthrough verifies nil/empty input.
func TestDedupeOEMRefsByArticleId_EmptyPassthrough(t *testing.T) {
	if got := dedupeOEMRefsByArticleId(nil); got != nil {
		t.Errorf("dedupe(nil) = %+v, want nil", got)
	}
	if got := dedupeOEMRefsByArticleId([]model.OEMReference{}); len(got) != 0 {
		t.Errorf("dedupe([]) len = %d, want 0", len(got))
	}
}

// TestDedupeOEMRefsByArticleId_NegativeIdsPreserved verifies that negative
// LegacyArticleId values (impossible in DB but defensive against bad data)
// are treated like zero — passed through without dedup.
func TestDedupeOEMRefsByArticleId_NegativeIdsPreserved(t *testing.T) {
	in := []model.OEMReference{
		{LegacyArticleId: -1, ArticleNumber: "neg-1"},
		{LegacyArticleId: -1, ArticleNumber: "neg-1-again"},
	}
	out := dedupeOEMRefsByArticleId(in)
	if len(out) != 2 {
		t.Errorf("negative-id refs should not be deduped, got len=%d", len(out))
	}
}

// ─── smartSearchOEMPromoter — production adapter nil-safety ──────────────

// TestSmartSearchOEMPromoter_NilTecDoc verifies the adapter is safe when
// SmartSearch has no TecDoc wired (offline mode). PromoteBy* methods
// return (nil, nil) instead of panicking on nil deref.
func TestSmartSearchOEMPromoter_NilTecDoc(t *testing.T) {
	p := &smartSearchOEMPromoter{tecdoc: nil, crossRef: nil}

	refs, err := p.PromoteByOEM("26300-35503", 5)
	if err != nil || refs != nil {
		t.Errorf("PromoteByOEM(nil tecdoc) = %v/%v, want nil/nil", refs, err)
	}
	refs, err = p.PromoteByCrossReferences("26300-35503", 5)
	if err != nil || refs != nil {
		t.Errorf("PromoteByCrossReferences(nil crossRef) = %v/%v, want nil/nil", refs, err)
	}
	refs, err = p.PromoteByOEMIndex("26300-35503", 5)
	if err != nil || refs != nil {
		t.Errorf("PromoteByOEMIndex(nil tecdoc) = %v/%v, want nil/nil", refs, err)
	}
	m, err := p.FetchDataSupplierIds([]int{42})
	if err != nil || m != nil {
		t.Errorf("FetchDataSupplierIds(nil tecdoc) = %v/%v, want nil/nil", m, err)
	}
}

// TestSmartSearchOEMPromoter_NilReceiver verifies the guard for a nil
// receiver — even the pointer itself being nil must not panic. The
// production code builds this struct unconditionally in enrichResults,
// so the nil-receiver path only fires in tests that pass a nil adapter;
// still, the guard costs nothing and protects against future refactors.
func TestSmartSearchOEMPromoter_NilReceiver(t *testing.T) {
	var p *smartSearchOEMPromoter // nil

	if refs, err := p.PromoteByOEM("26300-35503", 5); err != nil || refs != nil {
		t.Errorf("PromoteByOEM(nil receiver) = %v/%v, want nil/nil", refs, err)
	}
	if refs, err := p.PromoteByCrossReferences("26300-35503", 5); err != nil || refs != nil {
		t.Errorf("PromoteByCrossReferences(nil receiver) = %v/%v, want nil/nil", refs, err)
	}
	if refs, err := p.PromoteByOEMIndex("26300-35503", 5); err != nil || refs != nil {
		t.Errorf("PromoteByOEMIndex(nil receiver) = %v/%v, want nil/nil", refs, err)
	}
	if m, err := p.FetchDataSupplierIds([]int{42}); err != nil || m != nil {
		t.Errorf("FetchDataSupplierIds(nil receiver) = %v/%v, want nil/nil", m, err)
	}
}
