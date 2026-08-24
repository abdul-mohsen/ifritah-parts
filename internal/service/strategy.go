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
		{Key: "owned_catalog", Name: "Owned Catalog", Description: "Direct lookup against the owned hk_parts_cache — fast, exact article-number match"},
		{Key: "keyword_gated", Name: "Keyword (Gated)", Description: "TecDoc full-text search filtered by category so 'oil filter' cannot return 'strut mounting'"},
		{Key: "legacy", Name: "Legacy Cascade", Description: "The original pre-Smart-Search cascade: Postgres index → TecDoc → suffix strip → prefix match → online partsouq → dealer scrape → supersession. Slower but the widest net."},
		{Key: "prefix_inference", Name: "Prefix Inference", Description: "Synthesize an OEM description from part-family prefix + chassis code — always available, no network, ~5 ms. Fills gaps where TecDoc lacks aftermarket crosses."},
		{Key: "cache", Name: "Cache Hit", Description: "Persistent Postgres cache of every previously-resolved OEM. Sub-100ms, permanent, corroborates across sources."},
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
	case "owned_catalog":
		return &OwnedCatalogStrategy{search: s}
	case "keyword_gated":
		return &KeywordGatedStrategy{search: s}
	case "legacy":
		return &LegacyCascadeStrategy{search: s}
	case "prefix_inference":
		return &PrefixInferenceStrategy{search: s}
	case "cache":
		return &CacheStrategy{search: s}
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
// Hard budget: 12s; strategies that miss or err within that window are skipped.
//
// When progressCh is non-nil, emits one "start" event per strategy at dispatch
// time (label = per-strategy human text) and one "done" event when the
// strategy returns — so the SSE UI can render a live per-strategy checklist
// instead of a single opaque "Running all strategies…" spinner.
func (s *SmartSearch) searchCombined(query string, linkageTargetId, vehicleCC int, fuelType, category string, page, limit int, progressCh chan<- ProgressEvent) (*SmartSearchResponse, error) {
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

	// ─── HK scope gate (moved here from searchByOEM in 2026-08-22 audit) ─
	//
	// Combined mode is the DEFAULT search path — the frontend defaults to
	// mode="combined" (frontend/src/components/OemSearch.tsx). Before this
	// gate was added, non-HK OEMs (Toyota 90915-YZZE1, BMW 11-42-7-521-353,
	// Nissan 15208-9F600) bypassed the guard that existed only in
	// searchByOEM (used by the older non-combined path). They fell through
	// to the full strategy fan-out, ran every TecDoc query indexlessly,
	// and hit the 20s browser timeout.
	//
	// Applying the gate at combined-mode entry:
	//   * Rejects non-HK OEMs in <10ms with a helpful "Try Toyota parts
	//     instead" message.
	//   * Frees TecDoc query budget for real HK searches.
	//   * Preserves partial-OEM-stem behaviour (format="unknown" AND no
	//     SuggestedMake means the guard passes — same predicate as
	//     searchByOEM after the same-day fix).
	if req.OEM != "" {
		scope := IsHKOEM(req.OEM)
		if !scope.IsHK && (scope.Format != "unknown" || scope.SuggestedMake != "") {
			log.Printf("[searchCombined] REJECTED by HK-scope gate: %s", scope.Reason)
			resp := &SmartSearchResponse{
				Query:          query,
				SearchStrategy: "hk_scope_rejected",
				Warnings:       []string{scope.Reason},
				Total:          0,
			}
			if scope.SuggestedMake != "" {
				resp.Warnings = append(resp.Warnings,
					"Try the parts distributor for "+scope.SuggestedMake+" instead.")
			}
			// Still emit a "done" event so the SSE UI unblocks its spinner
			// immediately instead of waiting for the ctx timeout.
			send(progressCh, ProgressEvent{
				Type:      "progress",
				Step:      "hk_scope_rejected",
				Label:     "Not a Hyundai/Kia OEM",
				ElapsedMs: 0,
				Done:      true,
				Count:     0,
			})
			return resp, nil
		}
	}

	strategies := []SearchStrategy{
		&CacheStrategy{search: s},
		&LegacyCascadeStrategy{search: s},
		&ExactOEMStrategy{search: s},
		&CrossReferenceStrategy{search: s},
		&VehicleFitmentStrategy{search: s},
		&SupersessionStrategy{search: s},
		&CrossBrandStrategy{search: s},
		&OwnedCatalogStrategy{search: s},
		&KeywordGatedStrategy{search: s},
		&PrefixInferenceStrategy{search: s},
	}
	if s.tecDocSpecs != nil {
		strategies = append(strategies,
			&SpecMatchStrategy{search: s},
			&AssemblyContextStrategy{search: s},
			&VinAssemblyStrategy{search: s}, // requires tecDocSpecs same as assembly
		)
	}

	// Combined-mode fan-out deadline. Chosen so:
	//   * The two slowest strategies (ExactOEMStrategy runs the full
	//     legacy cascade with dealer_lookup at 3s + TecDoc primary at
	//     ~5s = ~8-10s p95) can still contribute
	//   * A pathological upstream (TecDoc lock, PartsOuq hang) can't
	//     block combined mode past this ceiling — the collect loop
	//     drains partial results on ctx.Done()
	//   * User-facing p95 stays below the 15s browser/reverse-proxy
	//     timeout observed in qa
	//
	// Fast strategies (prefix_inference 5 ms, cache 50 ms) always beat
	// this budget so combined-mode always returns SOMETHING useful.
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()

	fanOutStart := time.Now()

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
		// Emit "started" event so the UI shows this strategy as in-flight
		// BEFORE the goroutine begins its (possibly slow) work.
		send(progressCh, ProgressEvent{
			Type:      "progress",
			Step:      st.Name(),
			Label:     labelForMode(st.Name()),
			ElapsedMs: time.Since(fanOutStart).Milliseconds(),
		})

		wg.Add(1)
		go func(strat SearchStrategy) {
			defer wg.Done()
			start := time.Now()
			res, err := strat.Search(ctx, req)
			elapsed := time.Since(start)
			if err != nil {
				log.Printf("[Combined] strategy=%s err=%v elapsed=%v", strat.Name(), err, elapsed)
				s.recordStrategyFailure(strat.Name())
				// Emit "done with 0 results" — user sees the strategy
				// completed (with a fail count) rather than the row
				// hanging in "running" state forever.
				send(progressCh, ProgressEvent{
					Type:      "progress",
					Step:      strat.Name(),
					Label:     labelForMode(strat.Name()),
					ElapsedMs: time.Since(fanOutStart).Milliseconds(),
					Done:      true,
					Count:     0,
				})
				return
			}
			s.recordStrategySuccess(strat.Name())
			for i := range res {
				res[i].SourceStrategy = strat.Name()
			}
			// Emit "done with N results" so the UI can flip this
			// strategy from spinner → check-with-count.
			send(progressCh, ProgressEvent{
				Type:      "progress",
				Step:      strat.Name(),
				Label:     labelForMode(strat.Name()),
				ElapsedMs: time.Since(fanOutStart).Milliseconds(),
				Done:      true,
				Count:     len(res),
			})
			resultCh <- stratResult{name: strat.Name(), priority: strat.Priority(), results: res}
		}(st)
	}

	go func() {
		wg.Wait()
		close(resultCh)
	}()

	// Collect and merge results.
	//
	// BUG FIX 2026-08-17 (round 3): the previous impl used
	//   for sr := range resultCh { ... }
	// which blocks until close(resultCh) — and close only happens when
	// wg.Wait() returns, which requires EVERY strategy goroutine to finish.
	// When a strategy doesn't respect ctx (e.g. a MySQL query that runs
	// past ctx.Done()), the whole combined mode blocks past its 3s budget
	// and the user waits 15 s+ for a "combined" search. The debug-log
	// capture from qa.ifritah.com showed exactly this: TecDoc.SearchByOEM.
	// primary took 4.8 s per call, combined timed out at 15 s.
	//
	// Fix: drain resultCh with a select that ALSO listens on ctx.Done().
	// When the deadline fires we stop collecting even if slow strategies
	// are still running (they'll finish in the background and be
	// discarded). Prefix inference (5 ms) and cache hits (~50 ms) always
	// return within budget so combined mode is never worse than the
	// fastest strategy.
	//
	// BUG FIX 2026-08-17 (round 3, second): also dedupe by ArticleNumber
	// when LegacyArticleId <= 0. Previously the merge dropped every
	// result with LegacyArticleId=0 — which is EVERY prefix_inference
	// result (they synthesize descriptions, not TecDoc rows). This meant
	// combined mode never surfaced prefix inference results even when
	// they were the only correct answer available.
	priorities := make(map[string]float64)
	seen := make(map[string]SmartResult)
	var order []string

	// dedupeKey returns a stable identifier for the article. Preferred:
	// legacyArticleId (fast, guaranteed unique across TecDoc). Fallback:
	// uppercase article number (works for synthesized results that don't
	// have a TecDoc id, like prefix_inference).
	dedupeKey := func(r SmartResult) string {
		if r.LegacyArticleId > 0 {
			return fmt.Sprintf("id:%d", r.LegacyArticleId)
		}
		if r.ArticleNumber != "" {
			return "an:" + strings.ToUpper(r.ArticleNumber)
		}
		return "" // no stable identifier; skip
	}

collectLoop:
	for {
		select {
		case sr, ok := <-resultCh:
			if !ok {
				break collectLoop // channel closed — all strategies finished
			}
			priorities[sr.name] = sr.priority
			for _, r := range sr.results {
				key := dedupeKey(r)
				if key == "" {
					continue
				}
				if existing, ok := seen[key]; ok {
					// Convergence bonus: same article from ≥2 strategies
					// Cap at 1.0 to keep confidence meaningful.
					bonus := min64(max64(r.Confidence, existing.Confidence)*1.05, 1.0)
					r.Confidence = bonus
					if !strings.Contains(existing.SourceStrategy, r.SourceStrategy) {
						r.SourceStrategy = existing.SourceStrategy + "," + r.SourceStrategy
					}
					seen[key] = r
				} else {
					seen[key] = r
					order = append(order, key)
				}
			}
		case <-ctx.Done():
			log.Printf("[Combined] ctx deadline exceeded — returning %d partial results (some strategies may still be running)", len(seen))
			break collectLoop
		}
	}

	// Rank: score = confidence × strategy priority
	results := make([]SmartResult, 0, len(seen))
	for _, id := range order {
		if r, ok := seen[id]; ok {
			results = append(results, r)
		}
	}

	// M1.S2.T2: confidence floor for solo-cache low-conf hits. A single
	// cache-only hit with confidence < 0.5 and no corroboration from any
	// other strategy is almost always stale garbage (an old row for an
	// unrelated part that shares a prefix). Drop it - keeps combined
	// results tight when the cache is dirty. Multi-strategy hits pass
	// through regardless of confidence (the corroboration itself is
	// evidence of correctness).
	if len(results) > 0 {
		filtered := results[:0]
		dropped := 0
		for _, r := range results {
			primary := firstStrategyOf(r.SourceStrategy)
			isSoloCache := primary == "cache" && !strings.Contains(r.SourceStrategy, ",")
			if isSoloCache && r.Confidence < 0.5 {
				dropped++
				continue
			}
			filtered = append(filtered, r)
		}
		if dropped > 0 {
			log.Printf("[Combined] dropped %d solo-cache low-conf hit(s) for oem=%q", dropped, req.OEM)
		}
		results = filtered
	}

	// M1.S3.T2: category-consistency validation. When the queried OEM
	// decodes to a known part-family, drop any result whose description
	// doesn't contain a single token from the expected category label.
	// This catches wrong-category hallucinations that the M1.S1 penalty
	// only demotes but doesn't remove (mirror OEM 86391-* returning
	// "Headlight Assembly" — headlight doesn't overlap with "mirrors"
	// tokens so it gets dropped entirely).
	//
	// Opt-in only: nil expected-tokens means the OEM doesn't decode,
	// so we can't validate — pass everything through.
	if req.OEM != "" {
		expected := CategoryTokensForOEM(req.OEM)
		if len(expected) > 0 {
			kept := results[:0]
			dropped := 0
			for _, r := range results {
				if hasCategoryOverlap(r.Description, expected) {
					kept = append(kept, r)
					continue
				}
				dropped++
			}
			if dropped > 0 {
				log.Printf("[Combined] category-mismatch DROPPED %d/%d results for oem=%q expected_tokens=%v",
					dropped, len(results), req.OEM, expected)
			}
			results = kept
		}
	}

	// M1.S1.T2: cross-family confidence penalty. When a result surfaced by a
	// fallback strategy (cache / legacy / prefix_inference) has a category
	// whose parent system does NOT match the queried OEM prefix's expected
	// system, multiply its confidence by 0.2 so a same-system alternative -
	// if any - sorts above it. The result is preserved (not dropped) for
	// observability. See docs/reports/2026-08-23-quality-audit/README.md
	// for the 45 wrong-category categories that motivated this.
	if req.OEM != "" {
		penalised := 0
		for i := range results {
			primary := firstStrategyOf(results[i].SourceStrategy)
			isFallback := primary == "cache" || primary == "legacy" || primary == "prefix_inference"
			if !isFallback {
				continue
			}
			penalty := strategyCategoryPenalty(req.OEM, results[i].Category)
			if penalty < 1.0 {
				results[i].Confidence *= penalty
				penalised++
			}
		}
		if penalised > 0 {
			log.Printf("[Combined] category-penalty applied oem=%q penalised=%d/%d results", req.OEM, penalised, len(results))
		}
	}

	sort.Slice(results, func(i, j int) bool {
		// Parse the first strategy from comma-joined SourceStrategy to get its priority
		pi := strategyPriority(results[i].SourceStrategy, priorities)
		pj := strategyPriority(results[j].SourceStrategy, priorities)
		si := results[i].Confidence * pi
		sj := results[j].Confidence * pj
		if si != sj {
			return si > sj
		}
		// M1.S1.T3: same-system tiebreak. When scores match, prefer the
		// result whose category-system matches the queried OEM prefix.
		if req.OEM != "" {
			queried := DecodeOEMPrefix(req.OEM)
			if queried != nil {
				iMatch := categoryToSystem(results[i].Category) == queried.System
				jMatch := categoryToSystem(results[j].Category) == queried.System
				if iMatch != jMatch {
					return iMatch
				}
			}
		}
		return false
	})
	if len(results) > limit {
		results = results[:limit]
	}

	// Phase 2 write-back: cache every successful non-cache result so future
	// queries for the same OEM hit the cache in <100 ms. Fire-and-forget —
	// never adds latency to the read path. Skip results that already came
	// from the cache to avoid self-corroboration loops.
	if s.oemCache != nil && query != "" {
		for i := range results {
			if results[i].SourceStrategy == "oem_cache" || results[i].SourceStrategy == "cache" {
				continue
			}
			if strings.TrimSpace(results[i].Description) == "" {
				continue
			}
			s.oemCache.StoreResultAsync(results[i].ArticleNumber, &results[i], results[i].SourceStrategy, "")
		}
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

// firstStrategyOf extracts the primary strategy name from a comma-joined
// SourceStrategy string. Returns the input verbatim when no comma present.
// Kept package-scoped so both the ranker penalty and the confidence-floor
// filter share the same tokenisation rule.
func firstStrategyOf(source string) string {
	if idx := strings.Index(source, ","); idx >= 0 {
		return source[:idx]
	}
	return source
}

// categorySystemMap is a reverse index of prefixMap: given a category
// name it returns the system that category belongs to. Built once at
// init time so the cross-family penalty is O(1) per result.
var categorySystemMap = func() map[string]string {
	m := make(map[string]string, len(prefixMap))
	for _, cat := range prefixMap {
		if cat.Category != "" {
			m[cat.Category] = cat.System
		}
	}
	return m
}()

// categoryToSystem returns the system name associated with a category
// label, empty when unknown. See categorySystemMap for the source.
func categoryToSystem(category string) string {
	return categorySystemMap[category]
}

// strategyCategoryPenalty returns a confidence multiplier applied when a
// search result's category-system does NOT match the queried OEM prefix's
// expected system. Introduced in M1.S1 (2026-08-24 quality audit — see
// docs/reports/2026-08-23-quality-audit/README.md §"Categories that fail"
// for the 45 categories where combined mode returned wrong-family parts).
//
// The penalty preserves the hit (won't be dropped by a hard filter) but
// sinks its rank via confidence multiplication so a same-system
// alternative — if any exists — sorts above it.
//
// Returns 1.0 (no penalty) when:
//   - the queried OEM does not decode (partial stem, non-HK, empty)
//   - the result has no category, or the category is unknown to
//     categorySystemMap
//   - the result's category-system matches the queried OEM's system
//
// Returns 0.2 for cross-family mismatches (e.g. mirror OEM 86xxx-* returning
// a Headlight-Assembly result). Same-family matches at 1.0 always outrank
// cross-family matches at 0.2 unless the base confidence gap is > 5x.
func strategyCategoryPenalty(queryOEM string, resultCategory string) float64 {
	if queryOEM == "" || resultCategory == "" {
		return 1.0
	}
	queried := DecodeOEMPrefix(queryOEM)
	if queried == nil || queried.System == "" {
		return 1.0
	}
	resultSystem := categoryToSystem(resultCategory)
	if resultSystem == "" || resultSystem == queried.System {
		return 1.0
	}
	return 0.2
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

func (st *ExactOEMStrategy) Name() string            { return "exact_oem" }
func (st *ExactOEMStrategy) ConfidenceBase() float64 { return 0.95 }
func (st *ExactOEMStrategy) Priority() float64       { return 1.0 }
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

func (st *CrossReferenceStrategy) Name() string            { return "cross_reference" }
func (st *CrossReferenceStrategy) ConfidenceBase() float64 { return 0.92 }
func (st *CrossReferenceStrategy) Priority() float64       { return 0.9 }
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

func (st *VehicleFitmentStrategy) Name() string            { return "vehicle_fitment" }
func (st *VehicleFitmentStrategy) ConfidenceBase() float64 { return 0.98 }
func (st *VehicleFitmentStrategy) Priority() float64       { return 0.9 }
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

func (st *SupersessionStrategy) Name() string            { return "supersession" }
func (st *SupersessionStrategy) ConfidenceBase() float64 { return 0.85 }
func (st *SupersessionStrategy) Priority() float64       { return 0.85 }
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

func (st *CrossBrandStrategy) Name() string            { return "cross_brand" }
func (st *CrossBrandStrategy) ConfidenceBase() float64 { return 0.85 }
func (st *CrossBrandStrategy) Priority() float64       { return 0.75 }
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

// OwnedCatalogStrategy queries the local hk_parts_cache (Postgres) by article
// number, bypassing all TecDoc paths. Useful when you know the part is in the
// owned catalog and want a fast lookup without cross-ref noise.
type OwnedCatalogStrategy struct{ search *SmartSearch }

func (st *OwnedCatalogStrategy) Name() string            { return "owned_catalog" }
func (st *OwnedCatalogStrategy) ConfidenceBase() float64 { return 0.98 }
func (st *OwnedCatalogStrategy) Priority() float64       { return 0.90 }
func (st *OwnedCatalogStrategy) Search(ctx context.Context, req StrategyRequest) ([]SmartResult, error) {
	key := req.OEM
	if key == "" {
		key = req.Query
	}
	if key == "" || st.search.parts == nil {
		return nil, nil
	}
	parts, err := st.search.parts.FindByArticleNumber(key, req.LinkageTargetId, req.Limit)
	if err != nil {
		return nil, err
	}
	results := make([]SmartResult, 0, len(parts))
	for _, p := range parts {
		rule := ClassifyCategory(p.Description)
		results = append(results, SmartResult{
			Part:           p,
			Confidence:     0.98,
			ConfidenceNote: "Owned catalog exact match",
			FitmentDriver:  driverName(rule.Driver),
			BrandResolved:  p.BrandName,
		})
	}
	return results, nil
}

// KeywordGatedStrategy runs TecDoc keyword full-text search but gates each
// result by category so a query like "oil filter" never returns wrong-category
// parts. Confidence is capped at 0.65 — the tecdoc_keyword sentinel — so
// higher-confidence matches from other strategies always outrank it.
//
// See BUG-1 in BUGS.md: without the category gate, tecdoc_keyword returns
// junk like "strut mounting" for "oil filter" queries.
type KeywordGatedStrategy struct{ search *SmartSearch }

func (st *KeywordGatedStrategy) Name() string            { return "keyword_gated" }
func (st *KeywordGatedStrategy) ConfidenceBase() float64 { return 0.65 }
func (st *KeywordGatedStrategy) Priority() float64       { return 0.50 }
func (st *KeywordGatedStrategy) Search(ctx context.Context, req StrategyRequest) ([]SmartResult, error) {
	if req.Query == "" || st.search.tecdoc == nil {
		return nil, nil
	}
	// SKIP keyword search for OEM-shaped queries (BUG: real OEMs like "82460-2T010"
	// match irrelevant articles by digit-substring — "82460" → "pressure hose" etc.
	// Keyword search is only sensible for free-text queries like "oil filter").
	// Detect: if the query looks like an OEM number OR the whole query is digits+dashes,
	// bail out and return nothing.
	if looksLikeOEMNumber(req.Query) || isMostlyDigits(req.Query) {
		return nil, nil
	}
	tdRefs, err := st.search.tecdoc.SearchByKeyword(req.Query, req.Limit)
	if err != nil {
		return nil, err
	}
	// Same category-token gate used inside searchByText — reject articles whose
	// classified driver doesn't match the query's classified driver.
	queryRule := ClassifyCategory(req.Query)
	results := make([]SmartResult, 0, len(tdRefs))
	for _, ref := range tdRefs {
		if ref.LegacyArticleId <= 0 {
			continue
		}
		artRule := ClassifyCategory(ref.Description)
		if queryRule.Driver != FitUniversal && artRule.Driver != queryRule.Driver {
			// Wrong category — reject silently
			continue
		}
		results = append(results, SmartResult{
			Part: model.Part{
				LegacyArticleId: ref.LegacyArticleId,
				ArticleNumber:   ref.ArticleNumber,
				Description:     ref.Description,
				BrandName:       ref.BrandName,
			},
			Confidence:     0.65,
			ConfidenceNote: "Keyword search (category-gated)",
			FitmentDriver:  driverName(artRule.Driver),
			BrandResolved:  ref.BrandName,
		})
	}
	return results, nil
}

// isMostlyDigits returns true when 70%+ of the input characters are digits.
// Used as a guard on keyword search to prevent OEM-shaped queries (which are
// mostly digits) from being interpreted as text keywords.
func isMostlyDigits(s string) bool {
	if s == "" {
		return false
	}
	digits := 0
	total := 0
	for _, r := range s {
		if r == ' ' || r == '-' || r == '/' || r == '.' {
			continue
		}
		total++
		if r >= '0' && r <= '9' {
			digits++
		}
	}
	if total == 0 {
		return false
	}
	return float64(digits)/float64(total) >= 0.7
}

// LegacyCascadeStrategy wraps the pre-strategy searchDispatch cascade
// (Postgres oem_search_index -> TecDoc SearchByOEM -> suffix strip -> prefix
// match -> PartsOuq online lookup -> dealer_lookup scrape -> supersession).
//
// This is the strategy that found "82460-2T010 = FRONT POWER WINDOW MOTOR
// ASSEMBLY" via dealer_lookup while every mono-strategy missed it. Including
// it in searchCombined restores parity with the no-mode ("legacy") cascade
// so Smart Search never returns strictly less than the default path.
//
// The dispatch itself has online steps (partsouq/dealer scrape) that can be
// slow, so combined-mode's 3s ctx already bounds it. When those steps time
// out, the cheap Postgres+TecDoc paths at the top of the cascade still return
// promptly. Priority 0.95 (just under exact_oem) so its results outrank
// keyword_gated and cross-brand paths.
type LegacyCascadeStrategy struct{ search *SmartSearch }

func (st *LegacyCascadeStrategy) Name() string            { return "legacy" }
func (st *LegacyCascadeStrategy) ConfidenceBase() float64 { return 0.70 }
func (st *LegacyCascadeStrategy) Priority() float64       { return 0.95 }
func (st *LegacyCascadeStrategy) Search(ctx context.Context, req StrategyRequest) ([]SmartResult, error) {
	if req.Query == "" && req.LinkageTargetId == 0 {
		return nil, nil
	}
	resp, err := st.search.searchDispatch(req.Query, req.LinkageTargetId, req.VehicleCC, req.FuelType, req.Category, 1, req.Limit)
	if err != nil || resp == nil {
		return nil, err
	}
	for i := range resp.Results {
		if resp.Results[i].SourceStrategy == "" {
			resp.Results[i].SourceStrategy = "legacy"
		}
	}
	return resp.Results, nil
}

// PrefixInferenceStrategy synthesizes a description for the OEM by
// decomposing it into (5-digit part-family prefix, 2-char chassis code,
// 3-char variant suffix) and joining each against the Postgres lookup
// tables seeded by migration 000011 + auto-enriched by
// scripts/derive_hk_maps/main.go.
//
// The strategy always runs offline and completes in ~5 ms. Its output is
// gated to a max confidence of 0.90 — a live source (TecDoc, dealer_lookup)
// always outranks pure inference. When a live source AGREES with the
// inference, its confidence gets boosted via corroboration (Phase 2).
//
// Priority 0.55 — below every live-data strategy but above keyword_gated
// (0.50), so combined-mode returns inferred results only when nothing
// verified exists.
type PrefixInferenceStrategy struct{ search *SmartSearch }

func (st *PrefixInferenceStrategy) Name() string            { return "prefix_inference" }
func (st *PrefixInferenceStrategy) ConfidenceBase() float64 { return 0.85 }
func (st *PrefixInferenceStrategy) Priority() float64       { return 0.55 }
func (st *PrefixInferenceStrategy) Search(ctx context.Context, req StrategyRequest) ([]SmartResult, error) {
	if st.search == nil || st.search.prefixInference == nil {
		return nil, nil
	}
	// Guard: only run on OEM-shaped queries. Free text is not decoded.
	q := req.OEM
	if q == "" {
		q = req.Query
	}
	if q == "" || !looksLikeOEMNumber(q) {
		return nil, nil
	}
	result := st.search.prefixInference.Synthesize(ctx, q)
	if result == nil {
		return nil, nil
	}
	return []SmartResult{*result}, nil
}

// CacheStrategy hits the Phase-2 persistent Postgres oem_resolution_cache
// (populated by StoreResultAsync after any successful strategy return).
// Runs FIRST in the combined-mode fan-out at priority 0.99 — one Postgres
// PK lookup, ~10-100 ms.
//
// Design: the cache never expires. A hit is authoritative unless the row
// has been downgraded past the trust threshold (3 unverified negative
// user flags) — in which case Lookup returns nil and the fan-out re-runs
// live sources to refresh.
//
// The strategy is safe to include unconditionally: when no OEMCache is
// attached (e.g. Postgres unreachable) it silently returns nil,nil and
// the fan-out continues with the other strategies.
type CacheStrategy struct{ search *SmartSearch }

func (st *CacheStrategy) Name() string            { return "cache" }
func (st *CacheStrategy) ConfidenceBase() float64 { return 0.90 }
func (st *CacheStrategy) Priority() float64       { return 0.99 }
func (st *CacheStrategy) Search(ctx context.Context, req StrategyRequest) ([]SmartResult, error) {
	if st.search == nil || st.search.oemCache == nil {
		return nil, nil
	}
	q := req.OEM
	if q == "" {
		q = req.Query
	}
	if q == "" {
		return nil, nil
	}
	result, err := st.search.oemCache.Lookup(ctx, q)
	if err != nil || result == nil {
		return nil, err
	}
	return []SmartResult{*result}, nil
}
