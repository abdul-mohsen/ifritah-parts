package service

import (
	"log"
	"sync"
	"time"
)

// enrichResults fans out TecDoc enrichment for each result in parallel.
// level: "basic" = specs + compatible vehicles; "full" = + docs + supersession + functional equivalents.
// Cap: 10 concurrent goroutines across all results x enrichment calls.
//
// Handles two shapes of result:
//
//   (1) results with LegacyArticleId > 0 (from TecDoc-anchored strategies:
//       exact_oem step 0, cross_reference, vehicle_fitment, etc.)
//       → direct TecDoc lookups by articleId (specs, docs, vehicles,
//         supersession, functional equivalents)
//
//   (2) results with LegacyArticleId == 0 (from prefix_inference,
//       dealer_lookup, cache, and any online scrape)
//       → promote via TecDoc.SearchByOEM(articleNumber) which resolves the
//         OEM string to one or more TecDoc articleIds. Use the FIRST match
//         to run the same enrichment cascade. Also populate
//         AftermarketAlternatives from TecDoc.FindAftermarketForOEM.
//
// This closes the 2026-08-21 user-reported gap: combined-mode returned
// name + category but no aftermarket alternatives, no specs, no vehicles
// because every combined-mode result comes back with LegacyArticleId=0
// (prefix_inference synthesises; dealer_lookup scrapes; cache reuses).
// The previous filter `if enriched[i].LegacyArticleId <= 0 { continue }`
// dropped 100% of user-facing results from the enrichment path.
//
// Timeout: 2s per result wall-clock, checked between service calls. The
// Sprint 1 service methods (FindSpecifications, FindDocuments, etc.) don't
// accept a context.Context and can't be cancelled mid-flight — checking
// between calls prevents 1 slow call from cascading into 4 more slow calls.
func (s *SmartSearch) enrichResults(results []SmartResult, level string) []SmartResult {
	if level == "" || level == "none" {
		return results
	}

	const perResultBudget = 2 * time.Second

	sem := make(chan struct{}, 10)
	var wg sync.WaitGroup
	enriched := make([]SmartResult, len(results))
	copy(enriched, results)

	for i := range enriched {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			deadline := time.Now().Add(perResultBudget)
			budgetLeft := func() bool { return time.Now().Before(deadline) }

			// Resolve an articleId to enrich against. If the result came
			// from a TecDoc-anchored strategy, we already have one. If it
			// came from prefix_inference / dealer_lookup / cache, we look
			// it up by OEM string.
			articleId := enriched[idx].LegacyArticleId
			oem := enriched[idx].Part.ArticleNumber

			if articleId == 0 && oem != "" && s.tecdoc != nil && budgetLeft() {
				// Promote OEM string → TecDoc articleId(s). This also
				// populates OEMNumbers so callers can see the raw cross-refs.
				refs, err := s.tecdoc.SearchByOEM(oem, 5)
				if err == nil && len(refs) > 0 {
					// Pick the first ref with a non-zero articleId.
					for _, ref := range refs {
						if ref.LegacyArticleId > 0 {
							articleId = ref.LegacyArticleId
							break
						}
					}
					// Attach the resolved OEMNumbers for downstream UI use.
					enriched[idx].OEMNumbers = append(enriched[idx].OEMNumbers, refs...)
				}
			}

			// Aftermarket alternatives — works for BOTH articleId paths
			// AND OEM-string-only paths because FindAftermarketForOEM
			// queries by OEM string, not by articleId. Always run.
			if s.tecdoc != nil && oem != "" && budgetLeft() {
				if amParts, err := s.tecdoc.FindAftermarketForOEM(oem); err == nil {
					existing := make(map[string]bool, len(enriched[idx].AftermarketAlternatives))
					for _, p := range enriched[idx].AftermarketAlternatives {
						existing[p.Brand+"|"+p.PartNumber] = true
					}
					for _, p := range amParts {
						key := p.Brand + "|" + p.PartNumber
						if !existing[key] {
							enriched[idx].AftermarketAlternatives = append(enriched[idx].AftermarketAlternatives, p)
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
			if s.tecDocSpecs != nil && budgetLeft() {
				if specs, err := s.tecDocSpecs.FindSpecifications(articleId); err == nil {
					enriched[idx].Specifications = specs
				} else {
					log.Printf("[enrichResults] specs id=%d err=%v", articleId, err)
				}
			}

			// Always: compatible vehicles
			if s.tecDocVehicle != nil && budgetLeft() {
				if vehicles, err := s.tecDocVehicle.FindCompatibleVehicles(articleId, 20); err == nil {
					enriched[idx].CompatibleVehicles = vehicles
					// Populate legacy Compatibility []string for backward compat
					strs := make([]string, 0, len(vehicles))
					for _, v := range vehicles {
						strs = append(strs, v.VehicleName)
					}
					enriched[idx].Compatibility = strs
				}
			}

			if level != "full" {
				return
			}

			// Full only: documents (PDFs, images, technical drawings)
			if s.tecDocDocs != nil && budgetLeft() {
				if docs, err := s.tecDocDocs.FindDocuments(articleId); err == nil {
					enriched[idx].Documents = docs
				}
			}

			// Full only: supersession chain (superseded / successor OEMs)
			if s.tecDocSuper != nil && budgetLeft() {
				if chain, err := s.tecDocSuper.FindSupersession(articleId); err == nil {
					enriched[idx].Supersession = &chain
				}
			}

			// Full only: functional equivalents (same-fit alternatives)
			if s.tecDocFunctional != nil && budgetLeft() {
				if feq, err := s.tecDocFunctional.FindFunctionalEquivalents(articleId, 0, 20); err == nil {
					enriched[idx].FunctionalEquivalents = feq
				}
			}
		}(i)
	}

	wg.Wait()
	return enriched
}
