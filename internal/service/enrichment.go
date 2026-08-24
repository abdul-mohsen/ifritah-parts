package service

import (
	"context"
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
//	    → promote via TecDoc.SearchByOEM(articleNumber) which resolves the
//	      OEM string to one or more TecDoc articleIds. Use the FIRST match
//	      to run the same enrichment cascade. Also populate
//	      AftermarketAlternatives from TecDoc.FindAftermarketForOEM.
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
			// from a TecDoc-anchored strategy, we already have one. If it
			// came from prefix_inference / dealer_lookup / cache, we look
			// it up by OEM string.
			articleId := enriched.LegacyArticleId
			oem := enriched.Part.ArticleNumber

			if articleId == 0 && oem != "" && s.tecdoc != nil && budgetLeft() {
				// Primary: oem_number table (sparse HK coverage — ~5% of real HK
				// OEMs per the 2026-08-23 quality audit).
				refs, err := s.tecdoc.SearchByOEM(oem, 5)
				if err == nil && len(refs) > 0 {
					for _, ref := range refs {
						if ref.LegacyArticleId > 0 {
							articleId = ref.LegacyArticleId
							break
						}
					}
					enriched.OEMNumbers = append(enriched.OEMNumbers, refs...)
				}

				// Fallback: articlecrosses via SearchCrossReferences. The
				// 2026-08-23 quality audit found 0% CompatibleVehicles + 2.5%
				// Specs coverage because SearchByOEM returned 0 refs 74% of
				// the time, leaving articleId=0 and skipping all
				// article-anchored enrichment. articlecrosses (30M rows,
				// indexed by sql/06) has broader HK OEM coverage than
				// oem_number (21.5M rows); when the primary path fails, try
				// the cross-ref path before giving up on article-anchored
				// enrichment.
				if articleId == 0 && s.tecDocCrossRef != nil && budgetLeft() {
					crossRefs, cerr := s.tecDocCrossRef.SearchCrossReferences(oem, 5)
					if cerr == nil && len(crossRefs) > 0 {
						for _, ref := range crossRefs {
							if ref.LegacyArticleId > 0 {
								articleId = ref.LegacyArticleId
								break
							}
						}
						enriched.OEMNumbers = append(enriched.OEMNumbers, crossRefs...)
					}
				}

				// M3.S1.T1: third-level fallback via oem_search_index.
				// When both the primary oem_number lookup AND the
				// articlecrosses fallback return 0 refs, try the
				// secondary index table PR #14 introduced. Some OEMs
				// only appear here (fuzzy-match cross-refs cataloged
				// against slightly different OEM strings). Reads via
				// TecDoc.SearchByOEM's secondary path shape.
				if articleId == 0 && s.tecdoc != nil && budgetLeft() {
					idxRefs, ierr := s.tecdoc.SearchByOEMIndex(oem, 5)
					if ierr == nil && len(idxRefs) > 0 {
						for _, ref := range idxRefs {
							if ref.LegacyArticleId > 0 {
								articleId = ref.LegacyArticleId
								break
							}
						}
						enriched.OEMNumbers = append(enriched.OEMNumbers, idxRefs...)
					}
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
			// SearchByOEM promotion above.
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
