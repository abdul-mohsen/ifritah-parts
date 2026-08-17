package service

import (
	"context"
	"fmt"
	"log"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"parts-engine/internal/model"
)

// ─── SearchStrategy interface (S4-T1) ────────────────────────────────────────

// StrategyRequest is the unified input for any search strategy.
type StrategyRequest struct {
	OEM             string
	Query           string
	LinkageTargetId int
	VehicleCC       int
	FuelType        string
	Category        string
	Limit           int
}

// SearchStrategy is a hot-swappable search implementation.
type SearchStrategy interface {
	Search(ctx context.Context, req StrategyRequest) ([]SmartResult, error)
	Name() string
	ConfidenceBase() float64
	Priority() float64 // 0–1; higher = ranked first in combined merge
}

// ─── SearchMode descriptor (for /api/search/modes) ───────────────────────────

// SearchMode is the metadata for a registered strategy exposed via the API.
type SearchMode struct {
	Key         string `json:"key"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

// ─── Mode routing (S4-T6) ────────────────────────────────────────────────────

// searchByMode dispatches a single named strategy.
func (s *SmartSearch) searchByMode(query string, linkageTargetId, vehicleCC int, fuelType, category string, page, limit int, mode string) (*SmartSearchResponse, error) {
	req := StrategyRequest{
		Query:           query,
		LinkageTargetId: linkageTargetId,
		VehicleCC:       vehicleCC,
		FuelType:        fuelType,
		Category:        category,
		Limit:           limit,
	}
	if looksLikeOEMNumber(query) {
		req.OEM = query
	}

	strategy := s.strategyForMode(mode)
	if strategy == nil {
		// Fallback to legacy cascade for unknown mode
		return s.searchDispatch(query, linkageTargetId, vehicleCC, fuelType, category, page, limit)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	results, err := strategy.Search(ctx, req)
	if err != nil {
		// Match searchCombined behaviour: log and return empty results rather than
		// surfacing a 500 to the caller. A single strategy failing should not
		// prevent the user from seeing a graceful empty-results response.
		log.Printf("[searchByMode] strategy=%s err=%v — returning empty results", strategy.Name(), err)
		return &SmartSearchResponse{
			Query:          query,
			Results:        []SmartResult{},
			Total:          0,
			SearchStrategy: strategy.Name(),
			Mode:           mode,
			Warnings:       []string{fmt.Sprintf("%s search unavailable: %v", mode, err)},
		}, nil
	}

	// Tag source strategy on each result
	for i := range results {
		results[i].SourceStrategy = strategy.Name()
	}

	return &SmartSearchResponse{
		Query:          query,
		Results:        results,
		Total:          len(results),
		SearchStrategy: strategy.Name(),
		Mode:           mode,
	}, nil
}

// AvailableModes returns the list of registered search modes for the API endpoint.
func (s *SmartSearch) AvailableModes() []SearchMode {
	modes := []SearchMode{
		{Key: "exact_oem", Name: "Exact OEM", Description: "Search by exact OEM part number using the cross-reference index"},
		{Key: "cross_reference", Name: "Cross Reference", Description: "Find aftermarket alternatives via TecDoc articlecrosses (30M cross-refs)"},
		{Key: "vehicle_fitment", Name: "Vehicle Fitment", Description: "All parts linked to your specific vehicle from the TecDoc vehicle-part table"},
		{Key: "supersession", Name: "Supersession Chain", Description: "Walk the OEM replacement chain to find current and successor part numbers"},
		{Key: "cross_brand", Name: "Cross Brand", Description: "Hyundai ↔ Kia platform equivalents — parts that fit both brands"},
		{Key: "combined", Name: "Smart Search", Description: "Automatically runs all strategies in parallel and merges the best results"},
	}
	// Conditionally add modes that require TecDoc services.
	// Guard: tecDocSpecs is the minimum requirement for spec-based modes.
	if s.tecDocSpecs != nil {
		modes = append(modes, SearchMode{Key: "spec_match", Name: "Spec Match", Description: "Find parts sharing the same physical specifications (thread, diameter, pressure) as your input"})
		modes = append(modes,
			SearchMode{Key: "assembly_context", Name: "Sub-Parts", Description: "Give a parent component — get all sub-parts that physically fit it"},
			SearchMode{Key: "vin_assembly", Name: "Vehicle Spec Match", Description: "VIN or vehicle → derive compatible parts from engine and chassis specs, not just the parts database"},
		)
	}
	return modes
}

// strategyForMode returns the SearchStrategy for a given mode key.
// Guard: tecDocSpecs is the minimum requirement for spec-based modes;
// all three (spec_match, assembly_context, vin_assembly) use it.
func (s *SmartSearch) strategyForMode(mode string) SearchStrategy {
	switch mode {
	case "exact_oem":
		return &ExactOEMStrategy{search: s}
	case "cross_reference":
		return &CrossReferenceStrategy{search: s}
	case "vehicle_fitment":
		return &VehicleFitmentStrategy{search: s}
	case "supersession":
		return &SupersessionStrategy{search: s}
	case "cross_brand":
		return &CrossBrandStrategy{search: s}
	case "spec_match", "assembly_context", "vin_assembly":
		if s.tecDocSpecs == nil {
			return nil
		}
		switch mode {
		case "spec_match":
			return &SpecMatchStrategy{search: s}
		case "assembly_context":
			return &AssemblyContextStrategy{search: s}
		case "vin_assembly":
			return &VinAssemblyStrategy{search: s}
		}
	}
	return nil
}

// ─── Smart Combined (S4-T3 / S4-T4) ─────────────────────────────────────────

// searchCombined fans out all available strategies in parallel, merges and
// deduplicates results, and ranks by confidence × priority.
// Hard budget: 3s; strategies that miss or err within that window are skipped.
func (s *SmartSearch) searchCombined(query string, linkageTargetId, vehicleCC int, fuelType, category string, page, limit int) (*SmartSearchResponse, error) {
	req := StrategyRequest{
		Query:           query,
		LinkageTargetId: linkageTargetId,
		VehicleCC:       vehicleCC,
		FuelType:        fuelType,
		Category:        category,
		Limit:           limit,
	}
	if looksLikeOEMNumber(query) {
		req.OEM = query
	}

	strategies := []SearchStrategy{
		&ExactOEMStrategy{search: s},
		&CrossReferenceStrategy{search: s},
		&VehicleFitmentStrategy{search: s},
		&SupersessionStrategy{search: s},
		&CrossBrandStrategy{search: s},
	}
	if s.tecDocSpecs != nil {
		strategies = append(strategies,
			&SpecMatchStrategy{search: s},
			&AssemblyContextStrategy{search: s},
			&VinAssemblyStrategy{search: s}, // requires tecDocSpecs same as assembly
		)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	type stratResult struct {
		name     string
		priority float64
		results  []SmartResult
	}
	resultCh := make(chan stratResult, len(strategies))

	var wg sync.WaitGroup
	for _, st := range strategies {
		if s.circuitOpen(st.Name()) {
			continue
		}
		wg.Add(1)
		go func(strat SearchStrategy) {
			defer wg.Done()
			start := time.Now()
			res, err := strat.Search(ctx, req)
			elapsed := time.Since(start)
			if err != nil {
				log.Printf("[Combined] strategy=%s err=%v elapsed=%v", strat.Name(), err, elapsed)
				s.recordStrategyFailure(strat.Name())
				return
			}
			s.recordStrategySuccess(strat.Name())
			for i := range res {
				res[i].SourceStrategy = strat.Name()
			}
			resultCh <- stratResult{name: strat.Name(), priority: strat.Priority(), results: res}
		}(st)
	}

	go func() {
		wg.Wait()
		close(resultCh)
	}()

	// Collect and merge results
	// Priority lookup: populated when a strategy sends to resultCh
	priorities := make(map[string]float64)
	seen := make(map[int]SmartResult)
	var order []int

	for sr := range resultCh {
		priorities[sr.name] = sr.priority
		for _, r := range sr.results {
			if r.LegacyArticleId <= 0 {
				continue
			}
			if existing, ok := seen[r.LegacyArticleId]; ok {
				// Convergence bonus: same article from ≥2 strategies
				// Cap at 1.0 to keep confidence meaningful.
				bonus := min64(max64(r.Confidence, existing.Confidence)*1.05, 1.0)
				r.Confidence = bonus
				if !strings.Contains(existing.SourceStrategy, r.SourceStrategy) {
					r.SourceStrategy = existing.SourceStrategy + "," + r.SourceStrategy
				}
				seen[r.LegacyArticleId] = r
			} else {
				seen[r.LegacyArticleId] = r
				order = append(order, r.LegacyArticleId)
			}
		}
	}

	// Rank: score = confidence × strategy priority
	results := make([]SmartResult, 0, len(seen))
	for _, id := range order {
		if r, ok := seen[id]; ok {
			results = append(results, r)
		}
	}
	sort.Slice(results, func(i, j int) bool {
		// Parse the first strategy from comma-joined SourceStrategy to get its priority
		pi := strategyPriority(results[i].SourceStrategy, priorities)
		pj := strategyPriority(results[j].SourceStrategy, priorities)
		si := results[i].Confidence * pi
		sj := results[j].Confidence * pj
		return si > sj
	})
	if len(results) > limit {
		results = results[:limit]
	}

	return &SmartSearchResponse{
		Query:          query,
		Results:        results,
		Total:          len(results),
		SearchStrategy: "combined",
		Mode:           "combined",
	}, nil
}

func max64(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

func min64(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

// strategyPriority returns the priority for the first strategy name in a
// comma-joined source string, falling back to 0.5 when not found.
func strategyPriority(source string, priorities map[string]float64) float64 {
	first := source
	if idx := strings.Index(source, ","); idx >= 0 {
		first = source[:idx]
	}
	if p, ok := priorities[first]; ok {
		return p
	}
	return 0.5
}

// ─── Circuit breaker (S4-T5) ─────────────────────────────────────────────────
//
// State is stored on SmartSearch (not package globals) so multiple instances
// in tests do not share breaker state.

const (
	cbFailureThreshold int64 = 3
	cbCooldown               = 60 * time.Second
)

func (s *SmartSearch) circuitOpen(name string) bool {
	if v, ok := s.cbDisabled.Load(name); ok {
		if t, ok := v.(time.Time); ok && time.Now().Before(t) {
			return true
		}
		s.cbDisabled.Delete(name)
	}
	return false
}

func (s *SmartSearch) recordStrategyFailure(name string) {
	actual, _ := s.cbFailures.LoadOrStore(name, new(atomic.Int64))
	cnt := actual.(*atomic.Int64).Add(1)
	if cnt >= cbFailureThreshold {
		s.cbDisabled.Store(name, time.Now().Add(cbCooldown))
		actual.(*atomic.Int64).Store(0)
		log.Printf("[CircuitBreaker] strategy=%s tripped — disabled for %v", name, cbCooldown)
	}
}

func (s *SmartSearch) recordStrategySuccess(name string) {
	if v, ok := s.cbFailures.Load(name); ok {
		v.(*atomic.Int64).Store(0)
	}
}

// ─── Strategy implementations ────────────────────────────────────────────────

// ExactOEMStrategy wraps the Postgres oem_search_index + TecDoc oem_number path.
type ExactOEMStrategy struct{ search *SmartSearch }

func (st *ExactOEMStrategy) Name() string          { return "exact_oem" }
func (st *ExactOEMStrategy) ConfidenceBase() float64 { return 0.95 }
func (st *ExactOEMStrategy) Priority() float64      { return 1.0 }
func (st *ExactOEMStrategy) Search(ctx context.Context, req StrategyRequest) ([]SmartResult, error) {
	if req.OEM == "" {
		return nil, nil
	}
	resp, err := st.search.searchByOEM(req.OEM, req.LinkageTargetId, req.VehicleCC, req.FuelType, req.Limit)
	if err != nil || resp == nil {
		return nil, err
	}
	return resp.Results, nil
}

// CrossReferenceStrategy uses TecDoc articlecrosses (30M rows).
type CrossReferenceStrategy struct{ search *SmartSearch }

func (st *CrossReferenceStrategy) Name() string          { return "cross_reference" }
func (st *CrossReferenceStrategy) ConfidenceBase() float64 { return 0.92 }
func (st *CrossReferenceStrategy) Priority() float64      { return 0.9 }
func (st *CrossReferenceStrategy) Search(ctx context.Context, req StrategyRequest) ([]SmartResult, error) {
	if req.OEM == "" || st.search.tecDocCrossRef == nil {
		return nil, nil
	}
	refs, err := st.search.tecDocCrossRef.SearchCrossReferences(req.OEM, req.Limit)
	if err != nil {
		return nil, err
	}
	results := make([]SmartResult, 0, len(refs))
	for _, ref := range refs {
		if ref.LegacyArticleId <= 0 {
			continue
		}
		rule := ClassifyCategory(ref.Description)
		results = append(results, SmartResult{
			Part: model.Part{
				LegacyArticleId: ref.LegacyArticleId,
				ArticleNumber:   ref.ArticleNumber,
				Description:     ref.Description,
				BrandName:       ref.BrandName,
			},
			Confidence:    0.92,
			FitmentDriver: driverName(rule.Driver),
			BrandResolved: ref.BrandName,
		})
	}
	return results, nil
}

// VehicleFitmentStrategy uses articlesvehicletrees via searchByVehicle.
type VehicleFitmentStrategy struct{ search *SmartSearch }

func (st *VehicleFitmentStrategy) Name() string          { return "vehicle_fitment" }
func (st *VehicleFitmentStrategy) ConfidenceBase() float64 { return 0.98 }
func (st *VehicleFitmentStrategy) Priority() float64      { return 0.9 }
func (st *VehicleFitmentStrategy) Search(ctx context.Context, req StrategyRequest) ([]SmartResult, error) {
	if req.LinkageTargetId <= 0 {
		return nil, nil
	}
	resp, err := st.search.searchByVehicle("", req.LinkageTargetId, req.VehicleCC, req.FuelType, req.Category, 1, req.Limit)
	if err != nil || resp == nil {
		return nil, err
	}
	return resp.Results, nil
}

// SupersessionStrategy walks the replacement chain and returns the current/successor parts.
type SupersessionStrategy struct{ search *SmartSearch }

func (st *SupersessionStrategy) Name() string          { return "supersession" }
func (st *SupersessionStrategy) ConfidenceBase() float64 { return 0.85 }
func (st *SupersessionStrategy) Priority() float64      { return 0.85 }
func (st *SupersessionStrategy) Search(ctx context.Context, req StrategyRequest) ([]SmartResult, error) {
	if req.OEM == "" || st.search.tecDocSuper == nil {
		return nil, nil
	}
	// We need a legacyArticleId first — run OEM lookup to get it
	oemResult, err := st.search.oem.Search(req.OEM, 5)
	if err != nil || oemResult == nil || len(oemResult.Results) == 0 {
		return nil, nil
	}
	var results []SmartResult
	seen := map[int]bool{}
	for _, ref := range oemResult.Results {
		if ref.LegacyArticleId <= 0 || seen[ref.LegacyArticleId] {
			continue
		}
		seen[ref.LegacyArticleId] = true
		chain, cerr := st.search.tecDocSuper.FindSupersession(ref.LegacyArticleId)
		if cerr != nil {
			continue
		}
		// Add current + all replacedBy articles
		for _, link := range append(chain.ReplacedBy, chain.Current) {
			if link.LegacyArticleId <= 0 || seen[link.LegacyArticleId] {
				continue
			}
			seen[link.LegacyArticleId] = true
			results = append(results, SmartResult{
				Part: model.Part{
					LegacyArticleId: link.LegacyArticleId,
					ArticleNumber:   link.ArticleNumber,
					Description:     link.Description,
					BrandName:       link.BrandName,
				},
				Confidence:     link.Confidence,
				ConfidenceNote: fmt.Sprintf("Supersession chain (%s)", link.Direction),
				FitmentDriver:  "supersession",
			})
		}
	}
	return results, nil
}

// CrossBrandStrategy uses FindCrossBrandEquivalents for Hyundai ↔ Kia sharing.
type CrossBrandStrategy struct{ search *SmartSearch }

func (st *CrossBrandStrategy) Name() string          { return "cross_brand" }
func (st *CrossBrandStrategy) ConfidenceBase() float64 { return 0.85 }
func (st *CrossBrandStrategy) Priority() float64      { return 0.75 }
func (st *CrossBrandStrategy) Search(ctx context.Context, req StrategyRequest) ([]SmartResult, error) {
	if req.OEM == "" {
		return nil, nil
	}
	// TecDocCrossRef SearchCrossReferences already surfaces cross-brand; delegate
	if st.search.tecDocCrossRef == nil {
		return nil, nil
	}
	refs, err := st.search.tecDocCrossRef.SearchCrossReferences(req.OEM, req.Limit)
	if err != nil {
		return nil, err
	}
	var results []SmartResult
	for _, ref := range refs {
		if ref.LegacyArticleId <= 0 {
			continue
		}
		rule := ClassifyCategory(ref.Description)
		results = append(results, SmartResult{
			Part: model.Part{
				LegacyArticleId: ref.LegacyArticleId,
				ArticleNumber:   ref.ArticleNumber,
				Description:     ref.Description,
				BrandName:       ref.BrandName,
			},
			Confidence:     0.85,
			ConfidenceNote: "Cross-brand platform equivalent",
			FitmentDriver:  driverName(rule.Driver),
			BrandResolved:  ref.BrandName,
		})
	}
	return results, nil
}
