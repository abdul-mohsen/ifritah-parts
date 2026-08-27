package service

import (
	"context"
	"errors"
	"log"
	"strings"
	"sync"
	"time"

	"parts-engine/internal/model"
)

// enrichmentBudget is the outer wall-clock deadline for the entire
// enrichResults call — the total time we're willing to spend waiting for
// TecDoc enrichment across ALL results before returning partial data.
//
// Chosen so:
//   - It's < the SearchWithProgress caller's expected response budget
//     (~15s browser/reverse-proxy cap), leaving headroom for the search
//     phase itself (up to 12s combined-mode ctx).
//   - It's > the fastest realistic enrichment (specs + vehicles + docs
//     on a well-indexed table = ~500 ms) with room for a slow-ish
//     legitimate call.
//   - It's < any single expected slow query — so ONE bad query can't
//     hold up the whole response past the browser timeout.
//
// After sql/07_articlecriteria_indexes.sql lands, FindSpecifications drops
// from 17-36s to sub-10ms; every enrichment finishes in well under 1s.
// This deadline exists so the app degrades gracefully if a new slow-query
// bug lands in the future — partial enrichment is better than a 20s
// hang.
const enrichmentBudget = 10 * time.Second

// perResultBudget is the wall-clock deadline PER RESULT for enrichment
// calls initiated by a single result's goroutine. Combined with the outer
// enrichmentBudget: the OUTER deadline cuts off the collect loop; the
// per-result budget stops any single goroutine from cascading one slow
// call into 4 more slow calls.
const perResultBudget = 2 * time.Second

// promotionLayerLimit is the per-layer LIMIT passed to each of the three
// article-id promotion sources. Five candidates is enough that the
// canonical-pick tiebreak has something to compare on wide-hit OEMs
// (Bosch/MANN/MAHLE all catalog the same Hyundai OEM) but small enough
// that the IN-list in FetchDataSupplierIds stays under a dozen items.
const promotionLayerLimit = 5

// errNoPromotion is the sentinel returned by promoteArticleIds when every
// source (SearchByOEM → SearchCrossReferences → SearchByOEMIndex) returned
// zero refs. Callers should treat it as "no article-anchored enrichment
// possible for this OEM" and continue with the un-promoted result. It is
// NOT a hard error — the OEM string may still yield aftermarket
// alternatives via FindAftermarketForOEM.
var errNoPromotion = errors.New("article-id promotion: no candidates from oem_number / articlecrosses / oem_search_index")

// oemPromoter is the injectable interface consulted by the chained
// article-id promotion pipeline (M3.S1.T1). Production wiring is
// smartSearchOEMPromoter which dispatches into SmartSearch.tecdoc +
// SmartSearch.tecDocCrossRef; unit tests inject a stub with recorded call
// counts (see enrichment_test.go).
//
// The three PromoteBy* methods mirror the three source tables:
//
//	PromoteByOEM             → oem_number (+ oem_search_index UNION inside)
//	PromoteByCrossReferences → articlecrosses.oemNumberNormalized
//	PromoteByOEMIndex        → oem_search_index.normalized (rescue path)
//
// FetchDataSupplierIds returns dataSupplierId per legacyArticleId so the
// pipeline can pick the canonical article when a layer returns multiple
// candidates (roadmap M3.S1.T1 tiebreak signal).
type oemPromoter interface {
	PromoteByOEM(oem string, limit int) ([]model.OEMReference, error)
	PromoteByCrossReferences(oem string, limit int) ([]model.OEMReference, error)
	PromoteByOEMIndex(oem string, limit int) ([]model.OEMReference, error)
	FetchDataSupplierIds(articleIds []int) (map[int]int, error)
}

// smartSearchOEMPromoter is the production adapter binding SmartSearch's
// TecDoc + TecDocCrossRef fields to the oemPromoter interface. Constructed
// per enrichResults call rather than cached on SmartSearch because the
// underlying services can be nil in offline / partial-DB configurations,
// and we want each call to consult the CURRENT s.tecdoc / s.tecDocCrossRef
// state (the SetTecDoc* setters can rewire mid-lifecycle).
type smartSearchOEMPromoter struct {
	tecdoc   *TecDoc
	crossRef *TecDocCrossRef
}

func (p *smartSearchOEMPromoter) PromoteByOEM(oem string, limit int) ([]model.OEMReference, error) {
	if p == nil || p.tecdoc == nil {
		return nil, nil
	}
	return p.tecdoc.SearchByOEM(oem, limit)
}

func (p *smartSearchOEMPromoter) PromoteByCrossReferences(oem string, limit int) ([]model.OEMReference, error) {
	if p == nil || p.crossRef == nil {
		return nil, nil
	}
	return p.crossRef.SearchCrossReferences(oem, limit)
}

func (p *smartSearchOEMPromoter) PromoteByOEMIndex(oem string, limit int) ([]model.OEMReference, error) {
	if p == nil || p.tecdoc == nil {
		return nil, nil
	}
	return p.tecdoc.SearchByOEMIndex(oem, limit)
}

func (p *smartSearchOEMPromoter) FetchDataSupplierIds(articleIds []int) (map[int]int, error) {
	if p == nil || p.tecdoc == nil {
		return nil, nil
	}
	return p.tecdoc.FetchDataSupplierIds(articleIds)
}

// promoteArticleIds is the chained-fallback article-id promotion pipeline
// (M3.S1.T1). Given an OEM string, consults three sources in order and
// returns the canonical article id plus the deduplicated ref set of the
// FIRST successful layer.
//
// Fast-path semantics: when layer 1 returns any refs, layers 2 and 3 are
// NOT called. This is by design — see
// docs/data-sources/article-id-promotion-diagnosis.md for the
// fallthrough-vs-UNION trade-off analysis. The short version:
//   - layer 1 (oem_number) is the highest-quality signal — trust it when
//     it hits.
//   - layer 3 (oem_search_index) is a subset of layer 1 in the happy path
//     (SearchByOEM unions both internally), so calling it unconditionally
//     would double-hit the same table.
//   - the DoD is a promotion RATE lift, not maximum-candidate recall.
//
// The canonical pick applies INSIDE the winning layer: if layer 1 returns
// five distinct article ids (Bosch / MANN / MAHLE / Denso / Valeo all
// catalog the same Hyundai OEM), we pick the id whose row in `articles`
// has the highest dataSupplierId (proxy for most-recently-cataloged, hence
// most-authoritative). Single-candidate layers skip the DB round-trip.
//
// Returns (bestArticleId, deduplicatedRefs, nil) on success. Returns
// (0, nil, errNoPromotion) when every source returned zero refs. Ctx
// cancellation between layers surfaces as (0, nil, ctx.Err()).
//
// Errors from individual layer calls are LOGGED but not fatal — the
// pipeline moves on to the next layer, matching the pre-M3 inline
// behavior. This is a soft-fail design: a MySQL blip on articlecrosses
// shouldn't kill enrichment when oem_search_index is still available.
func promoteArticleIds(ctx context.Context, p oemPromoter, oem string, perLayerLimit int) (int, []model.OEMReference, error) {
	if p == nil || oem == "" {
		return 0, nil, errNoPromotion
	}

	// Table-driven layer list — keeps the fallthrough loop compact and
	// makes the log labels stable for post-hoc audit.
	layers := []struct {
		name string
		call func(string, int) ([]model.OEMReference, error)
	}{
		{"oem_number", p.PromoteByOEM},
		{"articlecrosses", p.PromoteByCrossReferences},
		{"oem_search_index", p.PromoteByOEMIndex},
	}

	for _, layer := range layers {
		if err := ctx.Err(); err != nil {
			return 0, nil, err
		}

		refs, err := layer.call(oem, perLayerLimit)
		if err != nil {
			log.Printf("[promoteArticleIds] layer=%s oem=%q err=%v (continuing)", layer.name, oem, err)
			continue
		}
		if len(refs) == 0 {
			continue
		}

		// Layer produced candidates. Dedupe + canonical-pick, return.
		deduped := dedupeOEMRefsByArticleId(refs)
		best := pickCanonicalArticleId(p, deduped)
		if best == 0 {
			// Layer returned only zero-id refs (raw cross-refs with no
			// article match). Preserve them for OEMNumbers population
			// but the caller sees articleId==0 and skips
			// article-anchored enrichment — matches pre-M3 behavior.
			return 0, deduped, nil
		}
		return best, deduped, nil
	}

	return 0, nil, errNoPromotion
}

// dedupeOEMRefsByArticleId collapses refs sharing the same LegacyArticleId,
// preserving the first-seen entry. Refs with articleId <= 0 are kept
// verbatim (they carry raw cross-reference data useful for OEMNumbers
// display even when they don't resolve to a TecDoc article).
func dedupeOEMRefsByArticleId(refs []model.OEMReference) []model.OEMReference {
	if len(refs) == 0 {
		return refs
	}
	seen := make(map[int]bool, len(refs))
	out := make([]model.OEMReference, 0, len(refs))
	for _, r := range refs {
		if r.LegacyArticleId <= 0 {
			out = append(out, r)
			continue
		}
		if seen[r.LegacyArticleId] {
			continue
		}
		seen[r.LegacyArticleId] = true
		out = append(out, r)
	}
	return out
}

// pickCanonicalArticleId chooses the "canonical" article id when a
// promotion layer returned multiple distinct candidates. Signal is
// `articles.dataSupplierId` — the TecDoc supplier-registration id (higher
// = more recent supplier catalog entry, per the roadmap M3.S1.T1 spec).
//
// Zero-candidate refs → 0 (caller treats as "no promotion possible").
// Single-candidate refs → that id (no DB round-trip).
// Multi-candidate refs → batch-fetch supplier ids, return the highest.
// Ties → first-seen wins (stable across identical requests once the
// deduped slice is stable).
//
// FetchDataSupplierIds failure is treated as "no tiebreak available" —
// falls back to the first-seen id rather than surfacing the DB error,
// because the caller's contract is "give me the best id you can" not
// "fail if the tiebreak query failed".
func pickCanonicalArticleId(p oemPromoter, refs []model.OEMReference) int {
	ids := make([]int, 0, len(refs))
	for _, r := range refs {
		if r.LegacyArticleId > 0 {
			ids = append(ids, r.LegacyArticleId)
		}
	}
	if len(ids) == 0 {
		return 0
	}
	if len(ids) == 1 {
		return ids[0]
	}

	supplierByArt, err := p.FetchDataSupplierIds(ids)
	if err != nil || len(supplierByArt) == 0 {
		if err != nil {
			log.Printf("[pickCanonicalArticleId] FetchDataSupplierIds err=%v — falling back to first-seen id=%d", err, ids[0])
		}
		return ids[0]
	}

	bestID := ids[0]
	bestSupplier := supplierByArt[bestID] // zero if not in map
	for _, id := range ids[1:] {
		s := supplierByArt[id]
		if s > bestSupplier {
			bestID = id
			bestSupplier = s
		}
	}
	return bestID
}

// enrichedResult is what a single result-goroutine returns to the collect
// loop. Using a channel + collect-loop drain (rather than wg.Wait() + a
// shared enriched slice) is what makes ctx-timeout non-blocking: if the
// outer ctx fires, we stop collecting and return the un-enriched originals
// for any result whose goroutine has not yet reported in. Slow goroutines
// still complete in the background (they finish writing to resultCh which
// nobody reads any more — the channel and its buffered elements get GC'd
// when the last reference drops).
type enrichedResult struct {
	idx int
	r   SmartResult
}

// enrichResults fans out TecDoc enrichment for each result in parallel.
// level: "basic" = specs + compatible vehicles; "full" = + docs + supersession + functional equivalents.
// Cap: 10 concurrent goroutines across all results × enrichment calls.
//
// Handles two shapes of result:
//
//	(1) results with LegacyArticleId > 0 (from TecDoc-anchored strategies:
//	    exact_oem step 0, cross_reference, vehicle_fitment, etc.)
//	    → direct TecDoc lookups by articleId (specs, docs, vehicles,
//	      supersession, functional equivalents)
//
//	(2) results with LegacyArticleId == 0 (from prefix_inference,
//	    dealer_lookup, cache, and any online scrape)
//	    → promote via the chained article-id pipeline (M3.S1.T1):
//	      SearchByOEM → SearchCrossReferences → SearchByOEMIndex, with
//	      canonical-pick by dataSupplierId when a layer returns multiple
//	      candidates. Also populate AftermarketAlternatives from
//	      TecDoc.FindAftermarketForOEM.
//
// Timeout model (see 2026-08-22 audit for the bug this replaces):
//
//	OLD (pre-2026-08-22): per-result 2s budget only, wg.Wait() had no
//	deadline. When ONE goroutine hit a 17s slow query
//	(FindSpecifications on the un-indexed articlecriteria table), the
//	whole call blocked past the browser 20s cap even though the SEARCH
//	phase returned in 2s. Every result showed empty Specifications /
//	CompatibleVehicles / AftermarketAlternatives arrays. See
//	docs/reports/2026-08-22-*.md §3 RC-1 (fixed by
//	sql/07_articlecriteria_indexes.sql).
//
//	NEW: outer ctx.WithTimeout(enrichmentBudget) at the top; collect
//	loop uses select { resultCh, ctx.Done() } exactly like
//	searchCombined. If the ctx fires, we return whatever has arrived on
//	resultCh so far; un-enriched results fall back to their originals.
//	Slow goroutines leak (finish in the background) but the caller
//	is never blocked past enrichmentBudget.
//
// Callers pass a context so that cancellation composes with any outer
// deadline (e.g. the SSE handler abandoning the client). Pass
// context.Background() when there's no upstream deadline.
func (s *SmartSearch) enrichResults(ctx context.Context, results []SmartResult, level string) []SmartResult {
	if level == "" || level == "none" {
		return results
	}
	if len(results) == 0 {
		return results
	}

	// Compose the caller's ctx with the enrichment budget. Whichever
	// fires first wins — a caller cancel on abandoned SSE stream is as
	// good a reason to stop as our own budget expiring.
	ctx, cancel := context.WithTimeout(ctx, enrichmentBudget)
	defer cancel()

	sem := make(chan struct{}, 10)
	resultCh := make(chan enrichedResult, len(results))
	var wg sync.WaitGroup

	// Build the article-id promoter once, share across all per-result
	// goroutines. Adapter is a plain pointer struct — no locking, no
	// allocation per goroutine. Callers with fully-nil TecDoc wiring
	// still get a valid promoter; every PromoteBy* returns (nil, nil)
	// in that case, matching the old behavior where the inline block
	// was gated by `s.tecdoc != nil` at each layer.
	promoter := &smartSearchOEMPromoter{tecdoc: s.tecdoc, crossRef: s.tecDocCrossRef}

	for i, orig := range results {
		wg.Add(1)
		go func(idx int, enriched SmartResult) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			// Even if the outer ctx has already fired by the time this
			// goroutine acquires the semaphore, ship the un-enriched
			// original into resultCh so the collect loop has something
			// to place at this idx.
			defer func() { resultCh <- enrichedResult{idx: idx, r: enriched} }()

			deadline := time.Now().Add(perResultBudget)
			budgetLeft := func() bool {
				if ctx.Err() != nil {
					return false
				}
				return time.Now().Before(deadline)
			}

			// Resolve an articleId to enrich against. If the result came
			// from a TecDoc-anchored strategy, we already have one. If
			// it came from prefix_inference / dealer_lookup / cache, we
			// look it up by OEM string via the chained article-id
			// promotion pipeline (M3.S1.T1). errNoPromotion means "no
			// article-anchored enrichment for this OEM" but aftermarket
			// alternatives still run below (they're OEM-keyed, not
			// articleId-keyed).
			articleId := enriched.LegacyArticleId
			oem := enriched.Part.ArticleNumber

			if articleId == 0 && oem != "" && budgetLeft() {
				bestID, refs, err := promoteArticleIds(ctx, promoter, oem, promotionLayerLimit)
				if err != nil && !errors.Is(err, errNoPromotion) {
					// ctx.Err() or some non-sentinel error. Not fatal —
					// aftermarket path below still runs on the raw OEM.
					log.Printf("[enrichResults] promoteArticleIds oem=%q err=%v", oem, err)
				}
				if bestID > 0 {
					articleId = bestID
				}
				if len(refs) > 0 {
					enriched.OEMNumbers = append(enriched.OEMNumbers, refs...)
				}
			}

			// Aftermarket alternatives — works for BOTH articleId paths
			// AND OEM-string-only paths because FindAftermarketForOEM
			// queries by OEM string, not by articleId. Always run.
			if s.tecdoc != nil && oem != "" && budgetLeft() {
				if amParts, err := s.tecdoc.FindAftermarketForOEM(oem); err == nil {
					existing := make(map[string]bool, len(enriched.AftermarketAlternatives))
					for _, p := range enriched.AftermarketAlternatives {
						existing[NormalizeBrand(p.Brand)+"|"+strings.ToLower(p.PartNumber)] = true
					}
					for _, p := range amParts {
						key := NormalizeBrand(p.Brand) + "|" + strings.ToLower(p.PartNumber)
						if !existing[key] {
							enriched.AftermarketAlternatives = append(enriched.AftermarketAlternatives, p)
							existing[key] = true
						}
					}
				}
			}

			// TecDoc-articleId-anchored enrichments — need a non-zero
			// articleId either from the original result or from the
			// promotion pipeline above.
			if articleId <= 0 {
				return
			}

			// Always: specifications
			// M3.S1.T2: skip specs in the per-result goroutine — we
			// batch-fetch them after collect. Legacy per-result call
			// left in place for callers that don't have tecDocSpecs
			// wired but the batch path can still succeed.
			_ = articleId // suppress unused warning if we remove the block
			if false && s.tecDocSpecs != nil && budgetLeft() {
				if specs, err := s.tecDocSpecs.FindSpecifications(articleId); err == nil {
					enriched.Specifications = specs
				} else {
					log.Printf("[enrichResults] specs id=%d err=%v", articleId, err)
				}
			}

			// Always: compatible vehicles
			if s.tecDocVehicle != nil && budgetLeft() {
				if vehicles, err := s.tecDocVehicle.FindCompatibleVehicles(articleId, 20); err == nil {
					enriched.CompatibleVehicles = vehicles
					// Populate legacy Compatibility []string for backward compat
					strs := make([]string, 0, len(vehicles))
					for _, v := range vehicles {
						strs = append(strs, v.VehicleName)
					}
					enriched.Compatibility = strs
				}
			}

			if level != "full" {
				return
			}

			// Full only: documents (PDFs, images, technical drawings)
			if s.tecDocDocs != nil && budgetLeft() {
				if docs, err := s.tecDocDocs.FindDocuments(articleId); err == nil {
					enriched.Documents = docs
				}
			}

			// Full only: supersession chain (superseded / successor OEMs).
			// M2.S3.T2: every link in the chain also becomes an
			// OEMReference in the result's OEMNumbers list so parts
			// sellers can order any variant. Tagged Manufacturer =
			// "SUPERSESSION" so downstream deduping doesn't confuse
			// them with primary cross-refs.
			//
			// M2.S1.T2: also fetch aftermarket for every OEM in the
			// chain. Widens the aftermarket net by including brands
			// cataloged against the parent/child OEM (a Bosch filter
			// listed as fitting the SUPERSEDED OEM should surface for
			// the current OEM too).
			if s.tecDocSuper != nil && budgetLeft() {
				if chain, err := s.tecDocSuper.FindSupersession(articleId); err == nil {
					enriched.Supersession = &chain
					existing := make(map[string]bool, len(enriched.OEMNumbers))
					for _, r := range enriched.OEMNumbers {
						existing[strings.ToUpper(r.ArticleNumber)] = true
					}
					chainOEMs := make([]string, 0, len(chain.ReplacedBy)+len(chain.Replaces))
					for _, link := range chain.ReplacedBy {
						an := strings.ToUpper(link.ArticleNumber)
						if an == "" || existing[an] {
							continue
						}
						existing[an] = true
						enriched.OEMNumbers = append(enriched.OEMNumbers, model.OEMReference{
							RawNumber:       link.ArticleNumber,
							ArticleNumber:   link.ArticleNumber,
							Description:     link.Description,
							BrandName:       link.BrandName,
							LegacyArticleId: link.LegacyArticleId,
							Manufacturer:    "SUPERSESSION",
						})
						chainOEMs = append(chainOEMs, link.ArticleNumber)
					}
					for _, link := range chain.Replaces {
						an := strings.ToUpper(link.ArticleNumber)
						if an == "" || existing[an] {
							continue
						}
						existing[an] = true
						enriched.OEMNumbers = append(enriched.OEMNumbers, model.OEMReference{
							RawNumber:       link.ArticleNumber,
							ArticleNumber:   link.ArticleNumber,
							Description:     link.Description,
							BrandName:       link.BrandName,
							LegacyArticleId: link.LegacyArticleId,
							Manufacturer:    "SUPERSESSION",
						})
						chainOEMs = append(chainOEMs, link.ArticleNumber)
					}

					// M2.S1.T2: fetch aftermarket for each chain OEM
					// and union into AftermarketAlternatives. Bounded
					// to 5 chain OEMs so we don't fan out unbounded.
					if len(chainOEMs) > 5 {
						chainOEMs = chainOEMs[:5]
					}
					if s.tecdoc != nil && len(chainOEMs) > 0 && budgetLeft() {
						amExisting := make(map[string]bool, len(enriched.AftermarketAlternatives))
						for _, p := range enriched.AftermarketAlternatives {
							amExisting[NormalizeBrand(p.Brand)+"|"+strings.ToLower(p.PartNumber)] = true
						}
						for _, chainOEM := range chainOEMs {
							if !budgetLeft() {
								break
							}
							amParts, aerr := s.tecdoc.FindAftermarketForOEM(chainOEM)
							if aerr != nil {
								continue
							}
							for _, p := range amParts {
								key := NormalizeBrand(p.Brand) + "|" + strings.ToLower(p.PartNumber)
								if amExisting[key] {
									continue
								}
								amExisting[key] = true
								enriched.AftermarketAlternatives = append(enriched.AftermarketAlternatives, p)
							}
						}
					}
				}
			}

			// Full only: functional equivalents (same-fit alternatives)
			if s.tecDocFunctional != nil && budgetLeft() {
				if feq, err := s.tecDocFunctional.FindFunctionalEquivalents(articleId, 0, 20); err == nil {
					enriched.FunctionalEquivalents = feq
				}
			}
		}(i, orig)
	}

	// Close resultCh once every goroutine has reported in. Runs in its
	// own goroutine so the collect loop below can select on both the
	// channel and the ctx without deadlocking.
	go func() {
		wg.Wait()
		close(resultCh)
	}()

	// Pre-fill with un-enriched originals so any result whose goroutine
	// doesn't report before ctx fires falls back to the raw search hit
	// instead of a zero-valued SmartResult.
	out := make([]SmartResult, len(results))
	copy(out, results)

collectLoop:
	for i := 0; i < len(results); i++ {
		select {
		case er, ok := <-resultCh:
			if !ok {
				break collectLoop
			}
			out[er.idx] = er.r
		case <-ctx.Done():
			log.Printf("[enrichResults] ctx deadline exceeded after %d/%d results — returning partial (some goroutines may still be running)", i, len(results))
			break collectLoop
		}
	}

	// M3.S1.T2: post-collect batch enrichment for specifications. Cheaper
	// than per-result FindSpecifications when the response has many hits
	// AND requires sql/07_articlecriteria_indexes.sql applied on qa
	// (otherwise the IN-list query is N full scans of articlecriteria).
	//
	// Only runs when tecDocSpecs is wired, the request asks for enrichment
	// beyond 'basic', and ctx still has budget.
	if s.tecDocSpecs != nil && ctx.Err() == nil {
		articleIds := make([]int, 0, len(out))
		idToIdx := make(map[int][]int, len(out))
		for i, r := range out {
			id := r.LegacyArticleId
			if id > 0 {
				articleIds = append(articleIds, id)
				idToIdx[id] = append(idToIdx[id], i)
			}
		}
		if len(articleIds) > 0 {
			specsById, err := s.tecDocSpecs.FindSpecificationsBatch(articleIds)
			if err != nil {
				log.Printf("[enrichResults] batch specs err=%v (falling back to skip specs)", err)
			} else {
				for id, specs := range specsById {
					for _, idx := range idToIdx[id] {
						if len(out[idx].Specifications) == 0 {
							out[idx].Specifications = specs
						}
					}
				}
			}
		}
	}

	return out
}
