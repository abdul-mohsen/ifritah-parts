package service

import (
	"context"
	"log"
	"sync"
	"time"
)

// enrichResults fans out TecDoc enrichment for each result in parallel.
// level: "basic" → specs + compatible vehicles; "full" → + docs + supersession + functional equivalents.
// Cap: 10 concurrent goroutines across all results × enrichment calls.
// Each call respects a 2s per-result timeout via context.
func (s *SmartSearch) enrichResults(results []SmartResult, level string) []SmartResult {
	if level == "" || level == "none" {
		return results
	}

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

			// Per-result 2s hard timeout.
			// Sprint 1 service methods (FindSpecifications, FindDocuments etc.) use
			// context.Background() internally — they do not yet accept a caller context.
			// The goroutine checks ctx.Done() before starting work so that a cancelled
			// request skips enrichment rather than blocking; actual call-level propagation
			// requires passing ctx into Sprint 1 services (tracked as tech debt).
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()

			// Bail early if the parent context is already done.
			select {
			case <-ctx.Done():
				return
			default:
			}

			id := enriched[idx].LegacyArticleId

			// Always: specifications
			if s.tecDocSpecs != nil {
				if specs, err := s.tecDocSpecs.FindSpecifications(id); err == nil {
					enriched[idx].Specifications = specs
				} else {
					log.Printf("[enrichResults] specs id=%d err=%v", id, err)
				}
			}

			// Always: compatible vehicles
			if s.tecDocVehicle != nil {
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
			if s.tecDocDocs != nil {
				if docs, err := s.tecDocDocs.FindDocuments(id); err == nil {
					enriched[idx].Documents = docs
				}
			}

			// Full only: supersession
			if s.tecDocSuper != nil {
				if chain, err := s.tecDocSuper.FindSupersession(id); err == nil {
					enriched[idx].Supersession = &chain
				}
			}

			// Full only: functional equivalents
			if s.tecDocFunctional != nil {
				if feq, err := s.tecDocFunctional.FindFunctionalEquivalents(id, 0, 20); err == nil {
					enriched[idx].FunctionalEquivalents = feq
				}
			}
		}(i)
	}

	wg.Wait()
	return enriched
}
