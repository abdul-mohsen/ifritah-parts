package service

import (
	"log"
	"sync"
	"time"
)

// enrichResults fans out TecDoc enrichment for each result in parallel.
// level: "basic" → specs + compatible vehicles; "full" → + docs + supersession + functional equivalents.
// Cap: 10 concurrent goroutines across all results × enrichment calls.
//
// Timeout behaviour is HONEST about its limitation:
//   - We track a 2s per-result wall-clock via `perResultBudget` and abort the
//     goroutine when it expires between service calls.
//   - Sprint 1 service methods (FindSpecifications, FindDocuments, etc.) do
//     NOT accept a context.Context — they build their own with
//     context.Background() internally, so we cannot cancel a query mid-flight.
//   - What we CAN do: check the deadline BETWEEN calls and short-circuit the
//     remaining calls when the budget is exhausted. This is not real
//     cancellation but it prevents a single slow call from cascading into
//     4 more slow calls for the same result.
//   - Real cancellation requires adding ctx to the Sprint 1 service methods;
//     tracked as tech debt.
func (s *SmartSearch) enrichResults(results []SmartResult, level string) []SmartResult {
	if level == "" || level == "none" {
		return results
	}

	const perResultBudget = 2 * time.Second

	sem := make(chan struct{}, 10) // max 10 concurrent goroutines
	var wg sync.WaitGroup
	enriched := make([]SmartResult, len(results))
	copy(enriched, results)

	for i := range enriched {
		if enriched[i].LegacyArticleId <= 0 {
			continue
		}
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			deadline := time.Now().Add(perResultBudget)
			budgetLeft := func() bool { return time.Now().Before(deadline) }
			id := enriched[idx].LegacyArticleId

			// Always: specifications
			if s.tecDocSpecs != nil && budgetLeft() {
				if specs, err := s.tecDocSpecs.FindSpecifications(id); err == nil {
					enriched[idx].Specifications = specs
				} else {
					log.Printf("[enrichResults] specs id=%d err=%v", id, err)
				}
			}

			// Always: compatible vehicles
			if s.tecDocVehicle != nil && budgetLeft() {
				if vehicles, err := s.tecDocVehicle.FindCompatibleVehicles(id, 20); err == nil {
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

			// Full only: documents
			if s.tecDocDocs != nil && budgetLeft() {
				if docs, err := s.tecDocDocs.FindDocuments(id); err == nil {
					enriched[idx].Documents = docs
				}
			}

			// Full only: supersession
			if s.tecDocSuper != nil && budgetLeft() {
				if chain, err := s.tecDocSuper.FindSupersession(id); err == nil {
					enriched[idx].Supersession = &chain
				}
			}

			// Full only: functional equivalents
			if s.tecDocFunctional != nil && budgetLeft() {
				if feq, err := s.tecDocFunctional.FindFunctionalEquivalents(id, 0, 20); err == nil {
					enriched[idx].FunctionalEquivalents = feq
				}
			}
		}(i)
	}

	wg.Wait()
	return enriched
}
